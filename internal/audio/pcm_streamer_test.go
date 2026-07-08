package audio

import (
	"testing"

	"github.com/gopxl/beep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPCMStreamSeekCloser_StreamsAndSeeksStereoS16LE(t *testing.T) {
	pcm := []byte{
		0x00, 0x00, 0x00, 0x40, // left 0.0, right 0.5
		0x00, 0xc0, 0xff, 0x7f, // left -0.5, right just under 1.0
	}

	streamer, format, err := NewPCMStreamSeekCloser(pcm, beep.SampleRate(44100))
	require.NoError(t, err)
	defer streamer.Close()

	assert.Equal(t, beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 2}, format)
	assert.Equal(t, 2, streamer.Len())
	assert.Equal(t, 0, streamer.Position())

	samples := make([][2]float64, 1)
	n, ok := streamer.Stream(samples)
	require.Equal(t, 1, n)
	require.True(t, ok)
	assert.InDelta(t, 0.0, samples[0][0], 0.00001)
	assert.InDelta(t, 0.5, samples[0][1], 0.00001)
	assert.Equal(t, 1, streamer.Position())

	require.NoError(t, streamer.Seek(1))
	n, ok = streamer.Stream(samples)
	require.Equal(t, 1, n)
	require.True(t, ok)
	assert.InDelta(t, -0.5, samples[0][0], 0.00001)
	assert.InDelta(t, float64(32767)/32768.0, samples[0][1], 0.00001)
	assert.Equal(t, 2, streamer.Position())

	n, ok = streamer.Stream(samples)
	assert.Equal(t, 0, n)
	assert.False(t, ok)
}

func TestPCMStreamSeekCloser_RejectsMisalignedPCM(t *testing.T) {
	streamer, format, err := NewPCMStreamSeekCloser([]byte{0x00, 0x01, 0x02}, beep.SampleRate(44100))

	assert.Error(t, err)
	assert.Nil(t, streamer)
	assert.Equal(t, beep.Format{}, format)
	assert.Contains(t, err.Error(), "frame-aligned")
}
