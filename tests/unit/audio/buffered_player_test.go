package audio_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soundcloud-tui/internal/audio"
)

func TestBufferedStreamPlayer_NewPlayer(t *testing.T) {
	player := audio.NewBufferedStreamPlayer()

	require.NotNil(t, player)
	assert.Equal(t, audio.StateStopped, player.GetState())
	assert.Equal(t, float64(1.0), player.GetVolume())
	assert.Equal(t, time.Duration(0), player.GetPosition())
}

func TestBufferedStreamPlayer_ErrorHandling(t *testing.T) {
	// Inject a failing HTTP client + fast-fail policy so bad URLs error
	// quickly without touching the network.
	player := audio.NewBufferedStreamPlayer(
		audio.WithBufferedHTTPClient(newFailingHTTPClient()),
		audio.WithBufferedRetry(1, 0),
		audio.WithPreloadTimeout(150*time.Millisecond),
	)
	defer player.Close()

	tests := []struct {
		name        string
		streamURL   string
		expectError bool
	}{
		{
			name:        "empty URL returns error",
			streamURL:   "",
			expectError: true,
		},
		{
			name:        "invalid URL returns error",
			streamURL:   "invalid-url",
			expectError: true,
		},
		{
			name:        "non-existent URL returns error after retries",
			streamURL:   "https://example.com/nonexistent.mp3",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := player.Play(ctx, tt.streamURL)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBufferedStreamPlayer_StateManagement(t *testing.T) {
	player := audio.NewBufferedStreamPlayer()
	defer player.Close()

	// Initial state should be stopped
	assert.Equal(t, audio.StateStopped, player.GetState())

	// Test pause when stopped (should return error)
	err := player.Pause()
	assert.Error(t, err)

	// Test resume when stopped (should return error)
	err = player.Resume()
	assert.Error(t, err)
}

func TestBufferedStreamPlayer_VolumeControl(t *testing.T) {
	player := audio.NewBufferedStreamPlayer()
	defer player.Close()

	// Test initial volume
	assert.Equal(t, float64(1.0), player.GetVolume())

	// Test setting valid volume
	err := player.SetVolume(0.5)
	assert.NoError(t, err)
	assert.Equal(t, float64(0.5), player.GetVolume())

	// Test setting invalid volume (too high)
	err = player.SetVolume(1.5)
	assert.Error(t, err)

	// Test setting invalid volume (negative)
	err = player.SetVolume(-0.1)
	assert.Error(t, err)

	// Volume should remain unchanged after invalid attempts
	assert.Equal(t, float64(0.5), player.GetVolume())
}

func TestBufferedStreamPlayer_ContextCancellation(t *testing.T) {
	player := audio.NewBufferedStreamPlayer(
		audio.WithBufferedHTTPClient(newFailingHTTPClient()),
	)
	defer player.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should timeout/cancel quickly (no real network involved).
	err := player.Play(ctx, "https://example.com/long-audio.mp3")
	assert.Error(t, err)
}

func TestBufferedStreamPlayer_PlayShortCompletedDownload(t *testing.T) {
	shortWAV := makeTestWAV(8000, 8000)
	player := audio.NewBufferedStreamPlayer(
		audio.WithBufferedHTTPClient(newWAVResponder(shortWAV)),
		audio.WithBufferedAudioSink(fakeSink{}),
		audio.WithPreloadTimeout(300*time.Millisecond),
	)
	defer player.Close()

	err := player.Play(context.Background(), "https://example.com/short.wav")
	require.NoError(t, err)
	assert.Equal(t, audio.StatePlaying, player.GetState())
}

func TestBufferedStreamPlayer_PlayStreamHLSUsesInjectedDecoder(t *testing.T) {
	decoder := &fakeHLSDecoder{
		pcm: repeatedStereoPCM16LEFrames(20, [2]int16{16384, -16384}),
	}
	player := audio.NewBufferedStreamPlayer(
		audio.WithBufferedAudioSink(fakeSink{}),
		audio.WithBufferedHTTPClient(newFailingHTTPClient()),
		audio.WithBufferedHLSDecoder(decoder),
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
	assert.Equal(t, seekPosition, player.GetPosition())
}

func TestBufferedStreamPlayer_SeekOperations(t *testing.T) {
	player := audio.NewBufferedStreamPlayer()
	defer player.Close()

	// Test seek when no stream loaded
	err := player.Seek(time.Second)
	assert.Error(t, err)

	// Test seek with negative position
	err = player.Seek(-time.Second)
	assert.Error(t, err)
}

func TestBufferedStreamPlayer_CallbacksAndCleanup(t *testing.T) {
	player := audio.NewBufferedStreamPlayer()

	// Callbacks push onto a buffered channel so the test can wait on them
	// deterministically instead of sleeping.
	stateCh := make(chan audio.PlayerState, 8)
	player.SetStateChangeCallback(func(state audio.PlayerState) {
		stateCh <- state
	})
	// The error callback only fires from the async download goroutine after
	// retries + backoff (which needs the network), so we only assert that
	// registering it is safe here — its firing isn't deterministically
	// observable without a live stream.
	player.SetErrorCallback(func(error) {})

	// An empty URL fails synchronously — no network, no goroutines.
	err := player.Play(context.Background(), "")
	assert.Error(t, err)

	// Stop() deterministically fires the state-change callback without an
	// audio device or network. Wait on the channel, not a fixed sleep.
	require.NoError(t, player.Stop())
	select {
	case state := <-stateCh:
		assert.Equal(t, audio.StateStopped, state)
	case <-time.After(2 * time.Second):
		t.Fatal("state-change callback did not fire after Stop()")
	}

	// Close should stop cleanly without panicking.
	assert.NoError(t, player.Close())
}
