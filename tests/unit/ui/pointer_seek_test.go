package ui_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
	"soundcloud-tui/internal/ui/components/player"
)

// playingPlayer builds a PlayerComponent parked in the Playing state with a
// known cached position/duration, ready to exercise seek behavior.
func playingPlayer(t *testing.T, duration, position time.Duration) (*player.PlayerComponent, *MockAudioPlayer) {
	t.Helper()
	mock := &MockAudioPlayer{
		state:    audio.StatePlaying,
		position: position,
		duration: duration,
		volume:   1.0,
	}
	pc := player.NewPlayerComponent(mock, nil)
	pc.SetCurrentTrack(&soundcloud.Track{ID: 1, Title: "Test", Duration: int64(duration / time.Millisecond)})
	pc.SetState(player.StatePlaying)
	updated, _ := pc.Update(player.ProgressUpdateMsg{Position: position, Duration: duration})
	return updated.(*player.PlayerComponent), mock
}

// SeekToFraction maps a proportional position (0.0–1.0) onto the track and
// seeks there — the model behind progress-bar click-to-seek.
func TestPlayerComponent_SeekToFraction(t *testing.T) {
	pc, mock := playingPlayer(t, 200*time.Second, 0)

	model, cmd := pc.SeekToFraction(0.5)
	pc = model.(*player.PlayerComponent)

	// Position updates immediately (mirrors the ←/→ seek behavior).
	assert.Equal(t, 100*time.Second, pc.GetPosition())

	// Running the returned command performs the real Seek.
	runFirstImmediateCommand(cmd)
	require.Len(t, mock.seekPositions, 1)
	assert.Equal(t, 100*time.Second, mock.seekPositions[0])
}

// A fraction outside [0,1] clamps to the track bounds rather than erroring.
func TestPlayerComponent_SeekToFraction_Clamps(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fraction float64
		want     time.Duration
	}{
		{"below zero clamps to start", -0.3, 0},
		{"above one clamps to end", 1.4, 200 * time.Second},
		{"exact end", 1.0, 200 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc, mock := playingPlayer(t, 200*time.Second, 50*time.Second)
			model, cmd := pc.SeekToFraction(tc.fraction)
			pc = model.(*player.PlayerComponent)
			assert.Equal(t, tc.want, pc.GetPosition())
			runFirstImmediateCommand(cmd)
			require.Len(t, mock.seekPositions, 1)
			assert.Equal(t, tc.want, mock.seekPositions[0])
		})
	}
}

// With no known duration, a click must NOT seek and must NOT reset playback.
func TestPlayerComponent_SeekToFraction_NoDurationIsNoOp(t *testing.T) {
	mock := &MockAudioPlayer{
		state:    audio.StatePlaying,
		position: 42 * time.Second,
		duration: 0, // unknown
		volume:   1.0,
	}
	pc := player.NewPlayerComponent(mock, nil)
	pc.SetCurrentTrack(&soundcloud.Track{ID: 1, Title: "Live"})
	pc.SetState(player.StatePlaying)

	model, cmd := pc.SeekToFraction(0.5)
	pc = model.(*player.PlayerComponent)

	runFirstImmediateCommand(cmd)
	assert.Empty(t, mock.seekPositions, "no seek attempted without a known duration")
	assert.Equal(t, 42*time.Second, mock.GetPosition(), "playback position untouched")
}

// A nil audio player is safe (idle app, no track loaded).
func TestPlayerComponent_SeekToFraction_NilPlayerIsSafe(t *testing.T) {
	pc := player.NewPlayerComponent(nil, nil)
	model, cmd := pc.SeekToFraction(0.5)
	require.NotNil(t, model)
	assert.Nil(t, cmd)
}
