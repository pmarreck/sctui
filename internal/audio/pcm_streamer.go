package audio

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"

	"github.com/gopxl/beep"
)

const pcmStereoS16LEFrameBytes = 4

// PCMStreamSeekCloser adapts decoded stereo s16le PCM bytes into Beep's
// seekable streamer interface, giving HLS playback local in-memory seeking.
type PCMStreamSeekCloser struct {
	mu       sync.RWMutex
	data     []byte
	position int
	closed   bool
}

// NewPCMStreamSeekCloser validates ffmpeg's stereo s16le output and returns a
// seekable Beep streamer plus the matching audio format.
func NewPCMStreamSeekCloser(pcm []byte, sampleRate beep.SampleRate) (beep.StreamSeekCloser, beep.Format, error) {
	if sampleRate <= 0 {
		return nil, beep.Format{}, fmt.Errorf("sample rate must be positive")
	}
	if len(pcm) == 0 {
		return nil, beep.Format{}, fmt.Errorf("PCM data cannot be empty")
	}
	if len(pcm)%pcmStereoS16LEFrameBytes != 0 {
		return nil, beep.Format{}, fmt.Errorf("PCM data must be frame-aligned stereo s16le (%d-byte frames)", pcmStereoS16LEFrameBytes)
	}

	copied := append([]byte(nil), pcm...)
	return &PCMStreamSeekCloser{data: copied}, beep.Format{
		SampleRate:  sampleRate,
		NumChannels: 2,
		Precision:   2,
	}, nil
}

func (s *PCMStreamSeekCloser) Stream(samples [][2]float64) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.position >= s.lenLocked() {
		return 0, false
	}

	n := len(samples)
	remaining := s.lenLocked() - s.position
	if n > remaining {
		n = remaining
	}

	for i := 0; i < n; i++ {
		offset := (s.position + i) * pcmStereoS16LEFrameBytes
		left := int16(binary.LittleEndian.Uint16(s.data[offset:]))
		right := int16(binary.LittleEndian.Uint16(s.data[offset+2:]))
		samples[i][0] = float64(left) / 32768.0
		samples[i][1] = float64(right) / 32768.0
	}
	s.position += n

	return n, true
}

func (s *PCMStreamSeekCloser) Err() error {
	return nil
}

func (s *PCMStreamSeekCloser) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lenLocked()
}

func (s *PCMStreamSeekCloser) Position() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.position
}

func (s *PCMStreamSeekCloser) Seek(p int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p < 0 || p > s.lenLocked() {
		return fmt.Errorf("seek position %d out of range 0..%d", p, s.lenLocked())
	}
	if s.closed {
		return fmt.Errorf("streamer is closed")
	}
	s.position = p
	return nil
}

func (s *PCMStreamSeekCloser) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *PCMStreamSeekCloser) lenLocked() int {
	return len(s.data) / pcmStereoS16LEFrameBytes
}

func isHLSStream(format, streamURL string) bool {
	return strings.EqualFold(format, "hls") || strings.Contains(strings.ToLower(streamURL), ".m3u8")
}
