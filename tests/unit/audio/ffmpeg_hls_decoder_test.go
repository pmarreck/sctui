package audio_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/gopxl/beep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soundcloud-tui/internal/audio"
)

type recordingFFmpegRunner struct {
	name   string
	args   []string
	stdout []byte
	stderr string
	err    error
}

func (r *recordingFFmpegRunner) Run(ctx context.Context, name string, args ...string) ([]byte, string, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	return r.stdout, r.stderr, r.err
}

func TestFFmpegHLSDecoder_DecodesAACHLSToSeekablePCM(t *testing.T) {
	pcm := stereoPCM16LEFrames(
		[2]int16{0, 16384},
		[2]int16{-16384, 32767},
	)
	runner := &recordingFFmpegRunner{stdout: pcm}
	decoder := audio.NewFFmpegHLSDecoder(audio.WithFFmpegRunner(runner))

	streamer, format, err := decoder.Decode(context.Background(), "https://cf-media.sndcdn.com/playlist.m3u8?Policy=secret")
	require.NoError(t, err)
	defer streamer.Close()

	assert.Equal(t, "ffmpeg", runner.name)
	assert.Equal(t, []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", "https://cf-media.sndcdn.com/playlist.m3u8?Policy=secret",
		"-f", "s16le",
		"-ac", "2",
		"-ar", "44100",
		"pipe:1",
	}, runner.args)
	assert.Equal(t, beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 2}, format)
	assert.Equal(t, 2, streamer.Len())

	require.NoError(t, streamer.Seek(1))
	samples := make([][2]float64, 1)
	n, ok := streamer.Stream(samples)
	require.Equal(t, 1, n)
	require.True(t, ok)
	assert.InDelta(t, -0.5, samples[0][0], 0.00001)
	assert.InDelta(t, float64(32767)/32768.0, samples[0][1], 0.00001)
}

func TestFFmpegHLSDecoder_RedactsURLsFromErrors(t *testing.T) {
	runner := &recordingFFmpegRunner{
		stderr: "https://cf-media.sndcdn.com/playlist.m3u8?Policy=secret&Signature=token failed",
		err:    fmt.Errorf("exit status 1"),
	}
	decoder := audio.NewFFmpegHLSDecoder(audio.WithFFmpegRunner(runner))

	streamer, format, err := decoder.Decode(context.Background(), "https://cf-media.sndcdn.com/playlist.m3u8?Policy=secret")

	assert.Error(t, err)
	assert.Nil(t, streamer)
	assert.Equal(t, beep.Format{}, format)
	assert.Contains(t, err.Error(), "ffmpeg HLS decode failed")
	assert.Contains(t, err.Error(), "[url redacted]")
	assert.NotContains(t, err.Error(), "Policy=secret")
	assert.NotContains(t, err.Error(), "Signature=token")
}

func stereoPCM16LEFrames(frames ...[2]int16) []byte {
	out := make([]byte, 0, len(frames)*4)
	for _, frame := range frames {
		out = appendInt16LE(out, frame[0])
		out = appendInt16LE(out, frame[1])
	}
	return out
}

func repeatedStereoPCM16LEFrames(count int, frame [2]int16) []byte {
	frames := make([][2]int16, count)
	for i := range frames {
		frames[i] = frame
	}
	return stereoPCM16LEFrames(frames...)
}

func appendInt16LE(out []byte, v int16) []byte {
	u := uint16(v)
	return append(out, byte(u), byte(u>>8))
}
