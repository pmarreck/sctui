package ui_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
	"soundcloud-tui/internal/ui/components/player"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// barRowFromRender locates the █ progress bar in a rendered player View and
// returns its (row, startCol, cellWidth) measured in display cells. This is the
// independent oracle for the geometry function: it reads reality, not a formula.
func barRowFromRender(t *testing.T, view string) (row, startCol, width int) {
	t.Helper()
	for i, ln := range strings.Split(view, "\n") {
		plain := stripANSI(ln)
		idx := strings.IndexRune(plain, '█')
		if idx < 0 {
			continue
		}
		runeRun := 0
		for _, r := range plain[idx:] {
			if r != '█' {
				break
			}
			runeRun++
		}
		return i, lipgloss.Width(plain[:idx]), lipgloss.Width(strings.Repeat("█", runeRun))
	}
	t.Fatalf("no progress bar (█) found in rendered view:\n%s", view)
	return 0, 0, 0
}

func playingPlayerForRender(title string, width int) *player.PlayerComponent {
	mock := &MockAudioPlayer{state: audio.StatePlaying, position: 45 * time.Second, duration: 180 * time.Second, volume: 0.75}
	pc := player.NewPlayerComponent(mock, nil)
	pc.SetCurrentTrack(&soundcloud.Track{ID: 1, Title: title, User: soundcloud.User{Username: "Test Artist"}, Duration: 180000})
	pc.SetState(player.StatePlaying)
	pc.SetSize(width, 24)
	updated, _ := pc.Update(player.ProgressUpdateMsg{Position: 45 * time.Second, Duration: 180 * time.Second})
	return updated.(*player.PlayerComponent)
}

// TestProgressBarViewport_MatchesRender asserts the geometry function agrees
// with the actually-rendered bar across widths and a wrapped title (which
// pushes the bar down a row). If renderPlayingView's layout drifts, this fails.
func TestProgressBarViewport_MatchesRender(t *testing.T) {
	cases := []struct {
		name  string
		title string
		width int
	}{
		{"short title w60", "Short", 60},
		{"short title w80", "Short", 80},
		{"short title w120", "Short", 120},
		// A title far wider than (width-8) wraps to a second line, moving the bar.
		{"wrapped title w60", strings.Repeat("VeryLongTitleWord ", 6), 60},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pc := playingPlayerForRender(tc.title, tc.width)
			view := pc.View()
			wantRow, wantStart, wantWidth := barRowFromRender(t, view)

			row, startX, width, ok := pc.ProgressBarViewport()
			require.True(t, ok, "expected a seekable bar in playing state")
			assert.Equal(t, wantRow, row, "bar row")
			assert.Equal(t, wantStart, startX, "bar start column")
			assert.Equal(t, wantWidth, width, "bar cell width")
			assert.Equal(t, tc.width-12, width, "bar width should be p.width-12")
		})
	}
}

// No bar is reported outside the playing/paused views, so a stray click there
// can't be mistaken for a seek.
func TestProgressBarViewport_HiddenWhenNotPlaying(t *testing.T) {
	pc := player.NewPlayerComponent(&MockAudioPlayer{}, nil)
	pc.SetSize(80, 24)
	for _, st := range []player.State{player.StateIdle, player.StateLoading, player.StateError, player.StateCompleted} {
		pc.SetState(st)
		_, _, _, ok := pc.ProgressBarViewport()
		assert.False(t, ok, "state %v should report no bar", st)
	}
}

// Compact layouts (very short terminals) lay the bar out differently, so
// click-to-seek is disabled there for now.
func TestProgressBarViewport_HiddenWhenCompact(t *testing.T) {
	pc := playingPlayerForRender("Short", 80)
	pc.SetSize(80, 6) // height <= 8 => compact
	_, _, _, ok := pc.ProgressBarViewport()
	assert.False(t, ok, "compact layout should report no bar")
}
