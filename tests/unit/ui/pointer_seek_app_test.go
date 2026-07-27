package ui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
	"soundcloud-tui/internal/ui/app"
	"soundcloud-tui/internal/ui/components/player"
)

// playingApp returns an App on the Player view with a playing track of the
// given duration, sized to (width, height).
func playingApp(t *testing.T, width, height int, duration time.Duration) (*app.App, *MockAudioPlayer) {
	t.Helper()
	mockAudio := &MockAudioPlayer{state: audio.StatePlaying, duration: duration}
	application := app.NewAppWithDependencies(nil, mockAudio, nil)
	updated, _ := application.Update(tea.WindowSizeMsg{Width: width, Height: height})
	application = updated.(*app.App)
	application.SetCurrentView(app.ViewPlayer)

	pc := application.PlayerComponent()
	pc.SetCurrentTrack(&soundcloud.Track{ID: 7, Title: "Clicky", User: soundcloud.User{Username: "Artist"}, Duration: int64(duration / time.Millisecond)})
	pc.SetState(player.StatePlaying)
	pc.Update(player.ProgressUpdateMsg{Position: 0, Duration: duration})
	return application, mockAudio
}

func leftClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}
}

// A left-click at the horizontal midpoint of the rendered progress bar seeks to
// roughly the middle of the track.
func TestApp_ClickProgressBarSeeksToMidpoint(t *testing.T) {
	application, mockAudio := playingApp(t, 80, 24, 200*time.Second)

	// Locate the bar in the FULL app view (absolute screen coordinates).
	row, startCol, width := barRowFromRender(t, application.View())
	clickX := startCol + width/2

	updated, cmd := application.Update(leftClick(clickX, row))
	application = updated.(*app.App)
	runFirstImmediateCommand(cmd)

	require.Len(t, mockAudio.seekPositions, 1, "a click on the bar should seek exactly once")
	got := mockAudio.seekPositions[0]
	assert.InDelta(t, float64(100*time.Second), float64(got), float64(4*time.Second),
		"midpoint click should land near 50%%, got %s", got)
}

// Clicking the far-left cell seeks to the start; far-right seeks to the end.
func TestApp_ClickProgressBarEnds(t *testing.T) {
	application, mockAudio := playingApp(t, 80, 24, 200*time.Second)
	row, startCol, width := barRowFromRender(t, application.View())

	// Far left => ~0
	u1, c1 := application.Update(leftClick(startCol, row))
	application = u1.(*app.App)
	runFirstImmediateCommand(c1)
	require.Len(t, mockAudio.seekPositions, 1)
	assert.Equal(t, time.Duration(0), mockAudio.seekPositions[0], "leftmost click seeks to start")

	// Far right => ~end
	u2, c2 := application.Update(leftClick(startCol+width-1, row))
	application = u2.(*app.App)
	runFirstImmediateCommand(c2)
	require.Len(t, mockAudio.seekPositions, 2)
	assert.Equal(t, 200*time.Second, mockAudio.seekPositions[1], "rightmost click seeks to end")
}

// A click off the bar row (e.g. on the time-info line just below) does nothing.
func TestApp_ClickOffBarDoesNotSeek(t *testing.T) {
	application, mockAudio := playingApp(t, 80, 24, 200*time.Second)
	row, startCol, width := barRowFromRender(t, application.View())

	// One row below the bar, and also to the left of the bar entirely.
	for _, click := range []tea.MouseMsg{
		leftClick(startCol+width/2, row+1),
		leftClick(0, row),
	} {
		u, c := application.Update(click)
		application = u.(*app.App)
		runFirstImmediateCommand(c)
	}
	assert.Empty(t, mockAudio.seekPositions, "clicks off the bar must not seek")
}

// On a non-player view, a click at the same coordinates must not seek.
func TestApp_ClickBarCoordsOnOtherViewDoesNotSeek(t *testing.T) {
	application, mockAudio := playingApp(t, 80, 24, 200*time.Second)
	row, startCol, width := barRowFromRender(t, application.View())
	application.SetCurrentView(app.ViewSearch)

	u, c := application.Update(leftClick(startCol+width/2, row))
	application = u.(*app.App)
	runFirstImmediateCommand(c)
	assert.Empty(t, mockAudio.seekPositions, "seek only applies on the player view")
}
