package audio_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/wav"
)

// fakeSink is a headless audio.AudioSink used in tests. It accepts every call
// and touches no real audio device, so player logic can be exercised in CI.
type fakeSink struct{}

func (fakeSink) Init(sampleRate beep.SampleRate, bufferSize int) error { return nil }
func (fakeSink) Play(streamers ...beep.Streamer)                       {}
func (fakeSink) Lock()                                                 {}
func (fakeSink) Unlock()                                               {}

// roundTripFunc adapts a plain function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// readSeekNopCloser is a seekable, closeable in-memory body. The wav decoder
// type-asserts its reader to io.Seeker for Seek support, which io.NopCloser
// would hide.
type readSeekNopCloser struct{ *bytes.Reader }

func (readSeekNopCloser) Close() error { return nil }

// newWAVResponder returns an *http.Client whose transport answers every request
// with a valid, seekable WAV payload and 200 OK — no network involved.
func newWAVResponder(wavBytes []byte) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": {"audio/wav"}},
				Body:       readSeekNopCloser{bytes.NewReader(wavBytes)},
				Request:    r,
			}, nil
		}),
	}
}

// newStatusResponder returns an *http.Client whose transport replies with a
// per-host status: 200 for hosts containing wantHost, 404 otherwise. Used to
// make ValidateStreamURL's reachability check deterministic.
func newStatusResponder(wantHost string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			code := http.StatusNotFound
			status := "404 Not Found"
			if r.URL != nil && contains(r.URL.Host, wantHost) {
				code, status = http.StatusOK, "200 OK"
			}
			return &http.Response{
				StatusCode: code,
				Status:     status,
				Header:     http.Header{},
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    r,
			}, nil
		}),
	}
}

// newFailingHTTPClient returns a client whose transport always errors, so a
// stream download fails deterministically without any real network.
func newFailingHTTPClient() *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("network disabled in test")
		}),
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || bytes.Contains([]byte(haystack), []byte(needle))
}

// silentStreamer yields `remaining` frames of stereo silence — used only to
// synthesize a valid, decodable WAV blob.
type silentStreamer struct{ remaining int }

func (s *silentStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	if s.remaining <= 0 {
		return 0, false
	}
	n = len(samples)
	if n > s.remaining {
		n = s.remaining
	}
	for i := 0; i < n; i++ {
		samples[i] = [2]float64{0, 0}
	}
	s.remaining -= n
	return n, true
}

func (s *silentStreamer) Err() error { return nil }

// memWriteSeeker is an in-memory io.WriteSeeker (wav.Encode seeks back to patch
// the RIFF/data sizes into the header).
type memWriteSeeker struct {
	data []byte
	pos  int
}

func (m *memWriteSeeker) Write(p []byte) (int, error) {
	end := m.pos + len(p)
	if end > len(m.data) {
		m.data = append(m.data, make([]byte, end-len(m.data))...)
	}
	copy(m.data[m.pos:end], p)
	m.pos = end
	return len(p), nil
}

func (m *memWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var np int64
	switch whence {
	case io.SeekStart:
		np = offset
	case io.SeekCurrent:
		np = int64(m.pos) + offset
	case io.SeekEnd:
		np = int64(len(m.data)) + offset
	default:
		return 0, fmt.Errorf("invalid whence %d", whence)
	}
	if np < 0 {
		return 0, fmt.Errorf("negative position %d", np)
	}
	m.pos = int(np)
	return np, nil
}

// makeTestWAV builds a valid in-memory WAV of `frames` frames at the given
// sample rate (stereo, 16-bit). Duration = frames / sampleRate.
func makeTestWAV(sampleRate beep.SampleRate, frames int) []byte {
	var ws memWriteSeeker
	format := beep.Format{SampleRate: sampleRate, NumChannels: 2, Precision: 2}
	if err := wav.Encode(&ws, &silentStreamer{remaining: frames}, format); err != nil {
		panic(fmt.Sprintf("makeTestWAV: %v", err))
	}
	return ws.data
}

// testWAV is a 60-second silent WAV at 8kHz — long enough to seek within, small
// enough to hold in memory. Built once at package init (deterministic).
var testWAV = makeTestWAV(8000, 8000*60)
