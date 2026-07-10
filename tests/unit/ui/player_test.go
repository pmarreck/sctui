package ui_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
	"soundcloud-tui/internal/ui/components/player"
)

func runFirstImmediateCommand(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		if len(batch) == 0 {
			return nil
		}
		return runFirstImmediateCommand(batch[0])
	}
	return []tea.Msg{msg}
}

func messageTypeNames(msgs []tea.Msg) []string {
	names := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		names = append(names, fmt.Sprintf("%T", msg))
	}
	return names
}

func TestPlayerComponent_NewPlayerComponent(t *testing.T) {
	component := player.NewPlayerComponent(nil, nil)

	require.NotNil(t, component)
	assert.Nil(t, component.GetCurrentTrack())
	assert.Equal(t, player.StateIdle, component.GetState())
	assert.Equal(t, float64(1.0), component.GetVolume())
	assert.Equal(t, time.Duration(0), component.GetPosition())
}

func TestPlayerComponent_PlayTrack(t *testing.T) {
	mockPlayer := &MockAudioPlayer{}
	mockExtractor := &MockStreamExtractor{
		ExtractFunc: func(ctx context.Context, trackID int64) (*audio.StreamInfo, error) {
			return &audio.StreamInfo{
				URL:      "https://example.com/stream.mp3",
				Format:   "mp3",
				Quality:  "sq",
				Duration: 240000,
			}, nil
		},
	}

	component := player.NewPlayerComponent(mockPlayer, mockExtractor)

	track := &soundcloud.Track{
		ID:       123456789,
		Title:    "Test Track",
		User:     soundcloud.User{Username: "Test Artist"},
		Duration: 240000,
	}

	// Send play track message
	playMsg := player.PlayTrackMsg{Track: track}
	updatedComponent, cmd := component.Update(playMsg)
	component = updatedComponent.(*player.PlayerComponent)

	assert.Equal(t, player.StateLoading, component.GetState())
	assert.Equal(t, track, component.GetCurrentTrack())
	assert.NotNil(t, cmd) // Should return stream extraction command
}

func TestPlayerComponent_PlayTrackStopsCurrentPlaybackBeforeExtractingNextStream(t *testing.T) {
	var events []string
	mockPlayer := &MockAudioPlayer{
		state: audio.StatePlaying,
		OnStop: func() {
			events = append(events, "stop")
		},
	}
	mockExtractor := &MockStreamExtractor{
		ExtractTrackFunc: func(ctx context.Context, req audio.TrackStreamRequest) (*audio.StreamInfo, error) {
			events = append(events, "extract")
			return &audio.StreamInfo{
				URL:      "https://example.com/next.m3u8",
				Format:   "hls",
				Quality:  "hls",
				Duration: 180000,
			}, nil
		},
	}
	component := player.NewPlayerComponent(mockPlayer, mockExtractor)
	component.SetState(player.StatePlaying)
	component.SetCurrentTrack(&soundcloud.Track{ID: 1, Title: "Already Playing"})

	nextTrack := &soundcloud.Track{
		ID:                  2,
		Title:               "Next Private Track",
		PermalinkURL:        "https://soundcloud.com/peter/next-private",
		PlaylistID:          777,
		PlaylistSecretToken: "playlist-secret",
		SecretToken:         "track-secret",
	}

	updatedComponent, cmd := component.Update(player.PlayTrackMsg{Track: nextTrack})
	component = updatedComponent.(*player.PlayerComponent)
	require.NotNil(t, cmd)

	msgs := runFirstImmediateCommand(cmd)

	assert.Equal(t, player.StateLoading, component.GetState())
	assert.Equal(t, []string{"stop", "extract"}, events)
	assert.Equal(t, 1, mockPlayer.stopCalls)
	require.Len(t, mockExtractor.TrackRequests, 1)
	assert.Equal(t, audio.TrackStreamRequest{
		TrackID:             2,
		PermalinkURL:        "https://soundcloud.com/peter/next-private",
		PlaylistID:          777,
		PlaylistSecretToken: "playlist-secret",
		SecretToken:         "track-secret",
	}, mockExtractor.TrackRequests[0])
	assert.Contains(t, messageTypeNames(msgs), "player.StreamInfoMsg")
}

func TestPlayerComponent_IgnoresStaleStreamResolutionAfterANewerTrackIsSelected(t *testing.T) {
	mockPlayer := &MockAudioPlayer{}
	mockExtractor := &MockStreamExtractor{
		ExtractFunc: func(ctx context.Context, trackID int64) (*audio.StreamInfo, error) {
			return &audio.StreamInfo{URL: fmt.Sprintf("https://example.com/%d.m3u8", trackID), Format: "hls"}, nil
		},
	}
	component := player.NewPlayerComponent(mockPlayer, mockExtractor)
	first := &soundcloud.Track{ID: 1, Title: "First"}
	second := &soundcloud.Track{ID: 2, Title: "Second"}

	updated, firstCmd := component.Update(player.PlayTrackMsg{Track: first})
	component = updated.(*player.PlayerComponent)
	staleMessages := runFirstImmediateCommand(firstCmd)
	require.Len(t, staleMessages, 1)

	updated, _ = component.Update(player.PlayTrackMsg{Track: second})
	component = updated.(*player.PlayerComponent)
	updated, staleCmd := component.Update(staleMessages[0])
	component = updated.(*player.PlayerComponent)

	assert.Equal(t, second, component.GetCurrentTrack())
	assert.Equal(t, player.StateLoading, component.GetState())
	assert.Nil(t, staleCmd)
	assert.Equal(t, 0, mockPlayer.playStreamCalls)
}

func TestPlayerComponent_ReportsTrackCompletion(t *testing.T) {
	mockPlayer := &MockAudioPlayer{state: audio.StateStopped}
	component := player.NewPlayerComponent(mockPlayer, nil)
	track := &soundcloud.Track{ID: 1, Title: "Complete", Duration: 60_000}
	component.SetCurrentTrack(track)
	component.SetState(player.StatePlaying)

	updated, cmd := component.Update(player.ProgressUpdateMsg{
		Position: 60 * time.Second,
		Duration: 60 * time.Second,
	})
	component = updated.(*player.PlayerComponent)
	require.NotNil(t, cmd)

	message := cmd()
	completed, ok := message.(player.PlaybackCompletedMsg)
	require.True(t, ok)
	assert.Equal(t, track, completed.Track)
	assert.Equal(t, player.StateCompleted, component.GetState())
}

func TestPlayerComponent_TimeoutsUseSharedAudioDefaults(t *testing.T) {
	assert.Equal(t, audio.StreamExtractionTimeout, player.StreamExtractionTimeout)
	assert.Equal(t, audio.PlaybackStartTimeout, player.PlaybackStartTimeout)
	assert.GreaterOrEqual(t, player.LoadingTimeout, audio.StreamExtractionTimeout+audio.PlaybackStartTimeout)
}

func TestPlayerComponent_PlaybackControls(t *testing.T) {
	mockPlayer := &MockAudioPlayer{
		state: audio.StatePlaying,
	}
	component := player.NewPlayerComponent(mockPlayer, nil)

	// Set up playing state
	track := &soundcloud.Track{ID: 123, Title: "Test Track", User: soundcloud.User{Username: "Test Artist"}}
	component.SetCurrentTrack(track)
	component.SetState(player.StatePlaying)

	tests := []struct {
		name        string
		key         tea.Key
		expectedCmd bool
		description string
	}{
		{
			name:        "spacebar toggles play/pause",
			key:         tea.Key{Type: tea.KeySpace},
			expectedCmd: true,
			description: "should pause when playing",
		},
		{
			name:        "left arrow seeks backward",
			key:         tea.Key{Type: tea.KeyLeft},
			expectedCmd: true,
			description: "should seek backward",
		},
		{
			name:        "right arrow seeks forward",
			key:         tea.Key{Type: tea.KeyRight},
			expectedCmd: true,
			description: "should seek forward",
		},
		{
			name:        "plus increases volume",
			key:         tea.Key{Type: tea.KeyRunes, Runes: []rune{'+'}},
			expectedCmd: true,
			description: "should increase volume",
		},
		{
			name:        "minus decreases volume",
			key:         tea.Key{Type: tea.KeyRunes, Runes: []rune{'-'}},
			expectedCmd: true,
			description: "should decrease volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyMsg := tea.KeyMsg(tt.key)
			updatedComponent, cmd := component.Update(keyMsg)

			if tt.expectedCmd {
				assert.NotNil(t, cmd, tt.description)
			} else {
				assert.Nil(t, cmd, tt.description)
			}

			component = updatedComponent.(*player.PlayerComponent)
		})
	}
}

func TestPlayerComponent_VolumeControl(t *testing.T) {
	// Emulate the real player's default full volume.
	mockPlayer := &MockAudioPlayer{volume: 1.0}
	component := player.NewPlayerComponent(mockPlayer, nil)

	initialVolume := component.GetVolume()
	assert.Equal(t, float64(1.0), initialVolume)

	// Test volume increase
	plusMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}}
	updatedComponent, cmd := component.Update(plusMsg)
	component = updatedComponent.(*player.PlayerComponent)

	assert.NotNil(t, cmd)
	// Note: Volume change would be handled by the command result

	// Test volume decrease
	minusMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}}
	updatedComponent, cmd = component.Update(minusMsg)
	component = updatedComponent.(*player.PlayerComponent)

	assert.NotNil(t, cmd)
}

func TestPlayerComponent_SeekControls(t *testing.T) {
	mockPlayer := &MockAudioPlayer{
		state:    audio.StatePlaying,
		duration: 240 * time.Second,
		position: 60 * time.Second,
	}
	component := player.NewPlayerComponent(mockPlayer, nil)

	// Test seek backward
	leftMsg := tea.KeyMsg{Type: tea.KeyLeft}
	updatedComponent, cmd := component.Update(leftMsg)
	component = updatedComponent.(*player.PlayerComponent)

	assert.NotNil(t, cmd) // Should return seek command

	// Test seek forward
	rightMsg := tea.KeyMsg{Type: tea.KeyRight}
	updatedComponent, cmd = component.Update(rightMsg)
	component = updatedComponent.(*player.PlayerComponent)

	assert.NotNil(t, cmd) // Should return seek command
}

func TestPlayerComponent_RepeatedSeekForwardUsesTheLatestRequestedPosition(t *testing.T) {
	mockPlayer := &MockAudioPlayer{
		state:    audio.StatePlaying,
		duration: 60 * time.Second,
	}
	component := player.NewPlayerComponent(mockPlayer, nil)
	component.SetState(player.StatePlaying)

	updated, _ := component.Update(player.ProgressUpdateMsg{
		Position: 10 * time.Second,
		Duration: 60 * time.Second,
	})
	component = updated.(*player.PlayerComponent)

	updated, firstSeek := component.Update(tea.KeyMsg{Type: tea.KeyRight})
	component = updated.(*player.PlayerComponent)
	updated, secondSeek := component.Update(tea.KeyMsg{Type: tea.KeyRight})
	component = updated.(*player.PlayerComponent)
	require.NotNil(t, firstSeek)
	require.NotNil(t, secondSeek)

	_ = firstSeek()
	_ = secondSeek()
	assert.Equal(t, []time.Duration{20 * time.Second, 30 * time.Second}, mockPlayer.seekPositions)
	assert.Equal(t, 30*time.Second, component.GetPosition())
}

func TestPlayerComponent_SeekForwardUsesAudioDurationBeforeFirstProgressUpdate(t *testing.T) {
	mockPlayer := &MockAudioPlayer{
		state:    audio.StatePlaying,
		duration: 60 * time.Second,
	}
	component := player.NewPlayerComponent(mockPlayer, nil)
	component.SetState(player.StatePlaying)

	updated, cmd := component.Update(tea.KeyMsg{Type: tea.KeyRight})
	component = updated.(*player.PlayerComponent)
	require.NotNil(t, cmd)
	_ = cmd()

	assert.Equal(t, []time.Duration{10 * time.Second}, mockPlayer.seekPositions)
	assert.Equal(t, 10*time.Second, component.GetPosition())
}

func TestPlayerComponent_SeekForwardDoesNotRestartWhenDurationIsUnknown(t *testing.T) {
	mockPlayer := &MockAudioPlayer{state: audio.StatePlaying}
	component := player.NewPlayerComponent(mockPlayer, nil)
	component.SetState(player.StatePlaying)

	updated, cmd := component.Update(tea.KeyMsg{Type: tea.KeyRight})
	component = updated.(*player.PlayerComponent)
	require.NotNil(t, cmd)
	_, ok := cmd().(player.ProgressUpdateMsg)
	assert.True(t, ok)
	assert.Empty(t, mockPlayer.seekPositions)
	assert.Equal(t, time.Duration(0), component.GetPosition())
}

func TestPlayerComponent_StateTransitions(t *testing.T) {
	mockPlayer := &MockAudioPlayer{}
	component := player.NewPlayerComponent(mockPlayer, nil)

	// Initial state
	assert.Equal(t, player.StateIdle, component.GetState())

	// Loading state
	component.SetState(player.StateLoading)
	assert.Equal(t, player.StateLoading, component.GetState())

	// Playing state
	component.SetState(player.StatePlaying)
	assert.Equal(t, player.StatePlaying, component.GetState())

	// Paused state
	component.SetState(player.StatePaused)
	assert.Equal(t, player.StatePaused, component.GetState())

	// Error state
	component.SetState(player.StateError)
	assert.Equal(t, player.StateError, component.GetState())
}

func TestPlayerComponent_StreamInfoHandling(t *testing.T) {
	mockPlayer := &MockAudioPlayer{}
	component := player.NewPlayerComponent(mockPlayer, nil)

	streamInfo := &audio.StreamInfo{
		URL:      "https://example.com/stream.mp3",
		Format:   "mp3",
		Quality:  "sq",
		Duration: 240000,
	}

	// Stream info arrives while the component is loading. Per the buffered-
	// streaming design it stays in loading until playback actually starts, and
	// returns a command to begin playback.
	component.SetState(player.StateLoading)
	streamMsg := player.StreamInfoMsg{StreamInfo: streamInfo, Error: nil}
	updatedComponent, cmd := component.Update(streamMsg)
	component = updatedComponent.(*player.PlayerComponent)

	assert.Equal(t, player.StateLoading, component.GetState())
	assert.NotNil(t, cmd) // Should return play command
}

func TestPlayerComponent_PlayCommandPreservesStreamInfoFormat(t *testing.T) {
	mockPlayer := &MockAudioPlayer{}
	component := player.NewPlayerComponent(mockPlayer, nil)

	streamInfo := &audio.StreamInfo{
		URL:      "https://example.com/playlist.m3u8",
		Format:   "hls",
		Quality:  "hls",
		Duration: 240000,
	}

	component.SetState(player.StateLoading)
	updatedComponent, cmd := component.Update(player.StreamInfoMsg{StreamInfo: streamInfo})
	component = updatedComponent.(*player.PlayerComponent)
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(player.ProgressUpdateMsg)
	require.True(t, ok)
	assert.Equal(t, player.StateLoading, component.GetState())
	assert.Equal(t, 1, mockPlayer.playStreamCalls)
	assert.Equal(t, 0, mockPlayer.playCalls)
	require.NotNil(t, mockPlayer.lastStreamInfo)
	assert.Equal(t, "hls", mockPlayer.lastStreamInfo.Format)
	assert.Equal(t, "https://example.com/playlist.m3u8", mockPlayer.lastStreamInfo.URL)
}

func TestPlayerComponent_ErrorHandling(t *testing.T) {
	mockPlayer := &MockAudioPlayer{}
	component := player.NewPlayerComponent(mockPlayer, nil)

	// Send error message
	errorMsg := player.StreamInfoMsg{
		StreamInfo: nil,
		Error:      assert.AnError,
	}

	updatedComponent, _ := component.Update(errorMsg)
	component = updatedComponent.(*player.PlayerComponent)

	assert.Equal(t, player.StateError, component.GetState())
	assert.NotNil(t, component.GetError())
}

func TestPlayerComponent_ProgressUpdates(t *testing.T) {
	mockPlayer := &MockAudioPlayer{
		state:    audio.StatePlaying,
		duration: 240 * time.Second,
		position: 30 * time.Second,
	}
	component := player.NewPlayerComponent(mockPlayer, nil)

	// Send progress update message
	progressMsg := player.ProgressUpdateMsg{
		Position: 45 * time.Second,
		Duration: 240 * time.Second,
	}

	updatedComponent, _ := component.Update(progressMsg)
	component = updatedComponent.(*player.PlayerComponent)

	// The position should be updated based on the message
	assert.Equal(t, 45*time.Second, component.GetPosition())
	assert.Equal(t, 240*time.Second, component.GetDuration())
}

func TestPlayerComponent_ViewRendering(t *testing.T) {
	mockPlayer := &MockAudioPlayer{}
	component := player.NewPlayerComponent(mockPlayer, nil)

	// Test idle view
	view := component.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "No track") // Should show no track message

	// Set current track
	track := &soundcloud.Track{
		ID:       123,
		Title:    "Test Track",
		User:     soundcloud.User{Username: "Test Artist"},
		Duration: 240000,
	}
	component.SetCurrentTrack(track)
	component.SetState(player.StatePlaying)
	// Set the mock player to playing state
	mockPlayer.state = audio.StatePlaying

	view = component.View()
	assert.Contains(t, view, "Test Track")
	assert.Contains(t, view, "Test Artist")
	assert.Contains(t, view, "Playing") // Should show playing state
}

func TestPlayerComponent_BubbleTeaIntegration(t *testing.T) {
	mockPlayer := &MockAudioPlayer{}
	component := player.NewPlayerComponent(mockPlayer, nil)

	// Test that component implements tea.Model interface
	var _ tea.Model = component

	// Test Init returns expected command
	cmd := component.Init()
	assert.NotNil(t, cmd) // Should return progress ticker command

	// Test Update handles various message types
	_, cmd = component.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Nil(t, cmd) // Window size should not generate commands

	// Test View returns non-empty string
	view := component.View()
	assert.NotEmpty(t, view)
}
