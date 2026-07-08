package audio_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gopxl/beep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soundcloud-tui/internal/audio"
)

type fakeHLSDecoder struct {
	calls      int
	url        string
	pcm        []byte
	err        error
	sampleRate beep.SampleRate
}

func (d *fakeHLSDecoder) Decode(ctx context.Context, streamURL string) (beep.StreamSeekCloser, beep.Format, error) {
	d.calls++
	d.url = streamURL
	if d.err != nil {
		return nil, beep.Format{}, d.err
	}
	sampleRate := d.sampleRate
	if sampleRate == 0 {
		sampleRate = 44100
	}
	streamer, format, err := audio.NewPCMStreamSeekCloser(d.pcm, sampleRate)
	if err != nil {
		return nil, beep.Format{}, err
	}
	return streamer, format, nil
}

// newTestPlayer builds a BeepPlayer wired to a headless audio sink and an HTTP
// client that serves a valid in-memory WAV, so playback can be exercised
// deterministically without a sound card or network.
func newTestPlayer() *audio.BeepPlayer {
	return audio.NewBeepPlayer(
		audio.WithAudioSink(fakeSink{}),
		audio.WithHTTPClient(newWAVResponder(testWAV)),
	)
}

func TestBeepPlayer_NewPlayer(t *testing.T) {
	player := audio.NewBeepPlayer()

	require.NotNil(t, player)
	assert.Equal(t, audio.StateStopped, player.GetState())
	assert.Equal(t, float64(1.0), player.GetVolume()) // Default volume
	assert.Equal(t, time.Duration(0), player.GetPosition())
	assert.Equal(t, time.Duration(0), player.GetDuration())
}

func TestBeepPlayer_Play(t *testing.T) {
	tests := []struct {
		name      string
		streamURL string
		wantErr   bool
	}{
		{
			name:      "valid stream URL starts playback",
			streamURL: "https://cf-media.sndcdn.com/test.mp3",
			wantErr:   false,
		},
		{
			name:      "empty stream URL returns error",
			streamURL: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := newTestPlayer()
			ctx := context.Background()

			err := player.Play(ctx, tt.streamURL)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, audio.StateStopped, player.GetState())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, audio.StatePlaying, player.GetState())
			assert.Greater(t, player.GetDuration(), time.Duration(0))
		})
	}
}

func TestBeepPlayer_PlayStreamHLSUsesInjectedDecoder(t *testing.T) {
	decoder := &fakeHLSDecoder{
		pcm: repeatedStereoPCM16LEFrames(20, [2]int16{16384, -16384}),
	}
	player := audio.NewBeepPlayer(
		audio.WithAudioSink(fakeSink{}),
		audio.WithHTTPClient(newFailingHTTPClient()),
		audio.WithHLSDecoder(decoder),
	)
	defer player.Close()

	err := player.PlayStream(context.Background(), &audio.StreamInfo{
		URL:    "https://cf-media.sndcdn.com/playlist.m3u8?Policy=secret",
		Format: "hls",
	})

	require.NoError(t, err)
	assert.Equal(t, 1, decoder.calls)
	assert.Equal(t, "https://cf-media.sndcdn.com/playlist.m3u8?Policy=secret", decoder.url)
	assert.Equal(t, audio.StatePlaying, player.GetState())
	assert.Greater(t, player.GetDuration(), time.Duration(0))
	seekPosition := 10*time.Second/44100 + time.Nanosecond
	require.NoError(t, player.Seek(seekPosition))
	assert.Equal(t, beep.SampleRate(44100).D(10), player.GetPosition())
}

func TestBeepPlayer_PlayStreamHLSSurfacesDecodeErrors(t *testing.T) {
	decoder := &fakeHLSDecoder{err: fmt.Errorf("decoder failed")}
	player := audio.NewBeepPlayer(
		audio.WithAudioSink(fakeSink{}),
		audio.WithHTTPClient(newFailingHTTPClient()),
		audio.WithHLSDecoder(decoder),
	)
	defer player.Close()

	err := player.PlayStream(context.Background(), &audio.StreamInfo{
		URL:    "https://cf-media.sndcdn.com/playlist.m3u8",
		Format: "hls",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decoder failed")
	assert.Equal(t, audio.StateStopped, player.GetState())
}

func TestBeepPlayer_PlayWithContextCancellation(t *testing.T) {
	player := newTestPlayer()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := player.Play(ctx, "https://example.com/test.mp3")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
	assert.Equal(t, audio.StateStopped, player.GetState())
}

func TestBeepPlayer_Pause(t *testing.T) {
	player := newTestPlayer()
	ctx := context.Background()

	// Cannot pause when stopped
	err := player.Pause()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot pause: player is stopped")

	// Start playing first
	err = player.Play(ctx, "https://example.com/test.mp3")
	require.NoError(t, err)
	assert.Equal(t, audio.StatePlaying, player.GetState())

	// Now pause should work
	err = player.Pause()
	require.NoError(t, err)
	assert.Equal(t, audio.StatePaused, player.GetState())

	// Cannot pause when already paused
	err = player.Pause()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot pause: player is paused")
}

func TestBeepPlayer_Stop(t *testing.T) {
	player := newTestPlayer()
	ctx := context.Background()

	// Start playing
	err := player.Play(ctx, "https://example.com/test.mp3")
	require.NoError(t, err)

	// Simulate some playback position (well within the 60s test track)
	err = player.Seek(30 * time.Second)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, player.GetPosition())

	// Stop should reset position
	err = player.Stop()
	require.NoError(t, err)
	assert.Equal(t, audio.StateStopped, player.GetState())
	assert.Equal(t, time.Duration(0), player.GetPosition())
}

func TestBeepPlayer_Volume(t *testing.T) {
	player := audio.NewBeepPlayer()

	// Test valid volume values
	testVolumes := []float64{0.0, 0.5, 1.0}
	for _, volume := range testVolumes {
		err := player.SetVolume(volume)
		require.NoError(t, err)
		assert.Equal(t, volume, player.GetVolume())
	}

	// Test invalid volume values
	invalidVolumes := []float64{-0.1, 1.1, 2.0}
	for _, volume := range invalidVolumes {
		err := player.SetVolume(volume)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "volume must be between 0.0 and 1.0")
	}
}

func TestBeepPlayer_Seek(t *testing.T) {
	player := newTestPlayer()
	ctx := context.Background()

	// Start playing to set duration
	err := player.Play(ctx, "https://example.com/test.mp3")
	require.NoError(t, err)

	duration := player.GetDuration()
	require.Greater(t, duration, time.Duration(0))

	// Test valid seek positions
	validPositions := []time.Duration{
		0,
		duration / 2,
		duration,
	}

	for _, position := range validPositions {
		err := player.Seek(position)
		require.NoError(t, err)
		assert.Equal(t, position, player.GetPosition())
	}

	// Test invalid seek positions
	err = player.Seek(-1 * time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "position cannot be negative")

	err = player.Seek(duration + time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds duration")
}

func TestBeepPlayer_StateTransitions(t *testing.T) {
	player := newTestPlayer()
	ctx := context.Background()

	// Initial state
	assert.Equal(t, audio.StateStopped, player.GetState())

	// Stopped -> Playing
	err := player.Play(ctx, "https://example.com/test.mp3")
	require.NoError(t, err)
	assert.Equal(t, audio.StatePlaying, player.GetState())

	// Playing -> Paused
	err = player.Pause()
	require.NoError(t, err)
	assert.Equal(t, audio.StatePaused, player.GetState())

	// Paused -> Playing (resume)
	err = player.Play(ctx, "https://example.com/test.mp3")
	require.NoError(t, err)
	assert.Equal(t, audio.StatePlaying, player.GetState())

	// Playing -> Stopped
	err = player.Stop()
	require.NoError(t, err)
	assert.Equal(t, audio.StateStopped, player.GetState())

	// Paused -> Stopped
	err = player.Play(ctx, "https://example.com/test.mp3")
	require.NoError(t, err)
	err = player.Pause()
	require.NoError(t, err)
	err = player.Stop()
	require.NoError(t, err)
	assert.Equal(t, audio.StateStopped, player.GetState())
}

func TestBeepPlayer_Close(t *testing.T) {
	player := newTestPlayer()
	ctx := context.Background()

	// Start playing
	err := player.Play(ctx, "https://example.com/test.mp3")
	require.NoError(t, err)
	assert.Equal(t, audio.StatePlaying, player.GetState())

	// Close should stop playback
	err = player.Close()
	require.NoError(t, err)
	assert.Equal(t, audio.StateStopped, player.GetState())
	assert.Equal(t, time.Duration(0), player.GetPosition())
}

func TestPlayerState_String(t *testing.T) {
	tests := []struct {
		state    audio.PlayerState
		expected string
	}{
		{audio.StateStopped, "stopped"},
		{audio.StatePlaying, "playing"},
		{audio.StatePaused, "paused"},
		{audio.PlayerState(999), "unknown"}, // Invalid state
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestBeepPlayer_InterfaceCompliance(t *testing.T) {
	// Ensure BeepPlayer implements Player interface
	var _ audio.Player = (*audio.BeepPlayer)(nil)

	// Test that we can use it as Player interface
	var player audio.Player = audio.NewBeepPlayer()

	assert.Equal(t, audio.StateStopped, player.GetState())
	assert.Equal(t, float64(1.0), player.GetVolume())
}
