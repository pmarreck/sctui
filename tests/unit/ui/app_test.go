package ui_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
	"soundcloud-tui/internal/ui/app"
	"soundcloud-tui/internal/ui/components/player"
	"soundcloud-tui/internal/ui/styles"
)

func TestApp_NewApp(t *testing.T) {
	application := app.NewApp()

	require.NotNil(t, application)
	assert.Equal(t, app.ViewSearch, application.GetCurrentView())
	assert.False(t, application.IsQuitting())
}

func TestApp_TerminalTitleShowsPlaybackState(t *testing.T) {
	newApplication := func() (*app.App, *MockAudioPlayer) {
		audioPlayer := &MockAudioPlayer{}
		application := app.NewAppWithDependencies(
			&MockSoundCloudClient{},
			audioPlayer,
			&MockStreamExtractor{},
		)
		application.Init()
		return application, audioPlayer
	}

	t.Run("playing", func(t *testing.T) {
		application, audioPlayer := newApplication()
		audioPlayer.state = audio.StatePlaying

		updated, titleCmd := application.Update(player.PlaybackStartedMsg{})
		application = updated.(*app.App)
		require.NotNil(t, titleCmd)
		assert.Equal(t, "🔊 SoundCloud TUI", fmt.Sprint(titleCmd()))
	})

	for _, state := range []audio.PlayerState{audio.StatePaused, audio.StateStopped} {
		t.Run(state.String(), func(t *testing.T) {
			application, audioPlayer := newApplication()
			audioPlayer.state = audio.StatePlaying
			updated, playingTitleCmd := application.Update(player.PlaybackStartedMsg{})
			application = updated.(*app.App)
			require.NotNil(t, playingTitleCmd)

			audioPlayer.state = state
			updated, titleCmd := application.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			application = updated.(*app.App)
			require.NotNil(t, titleCmd)
			assert.Equal(t, "SoundCloud TUI", fmt.Sprint(titleCmd()))
		})
	}
}

func TestApp_SearchInputSpaceDoesNotTogglePlayback(t *testing.T) {
	audioPlayer := &MockAudioPlayer{state: audio.StatePlaying}
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{},
		audioPlayer,
		&MockStreamExtractor{},
	)

	updated, _ := application.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("lofi hip")})
	application = updated.(*app.App)
	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeySpace})
	application = updated.(*app.App)
	require.Nil(t, cmd)
	updated, _ = application.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hop")})
	application = updated.(*app.App)

	assert.Contains(t, application.View(), "lofi hip hop")
	assert.Equal(t, audio.StatePlaying, audioPlayer.GetState())
}

func TestApp_SpaceStillTogglesPlaybackOutsideSearchInput(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *app.App) *app.App
	}{
		{
			name: "player view",
			setup: func(_ *testing.T, application *app.App) *app.App {
				application.SetCurrentView(app.ViewPlayer)
				return application
			},
		},
		{
			name: "search results",
			setup: func(t *testing.T, application *app.App) *app.App {
				updated, _ := application.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("query")})
				application = updated.(*app.App)
				updated, searchCmd := application.Update(tea.KeyMsg{Type: tea.KeyEnter})
				application = updated.(*app.App)
				require.NotNil(t, searchCmd)
				updated, _ = application.Update(searchCmd())
				return updated.(*app.App)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			application := app.NewAppWithDependencies(
				&MockSoundCloudClient{},
				&MockAudioPlayer{state: audio.StatePlaying},
				&MockStreamExtractor{},
			)
			application = tt.setup(t, application)

			_, cmd := application.Update(tea.KeyMsg{Type: tea.KeySpace})
			require.NotNil(t, cmd)
		})
	}
}

func TestApp_Navigation(t *testing.T) {
	tests := []struct {
		name         string
		currentView  app.ViewType
		keyPressed   string
		expectedView app.ViewType
		shouldChange bool
	}{
		{
			name:         "tab from search to player",
			currentView:  app.ViewSearch,
			keyPressed:   "tab",
			expectedView: app.ViewPlayer,
			shouldChange: true,
		},
		{
			name:         "tab from player to playlists",
			currentView:  app.ViewPlayer,
			keyPressed:   "tab",
			expectedView: app.ViewPlaylists,
			shouldChange: true,
		},
		{
			name:         "tab from playlists to favorites",
			currentView:  app.ViewPlaylists,
			keyPressed:   "tab",
			expectedView: app.ViewFavorites,
			shouldChange: true,
		},
		{
			name:         "tab from favorites to search",
			currentView:  app.ViewFavorites,
			keyPressed:   "tab",
			expectedView: app.ViewSearch,
			shouldChange: true,
		},
		{
			name:         "shift+tab from search to favorites",
			currentView:  app.ViewSearch,
			keyPressed:   "shift+tab",
			expectedView: app.ViewFavorites,
			shouldChange: true,
		},
		{
			name:         "invalid key doesn't change view",
			currentView:  app.ViewSearch,
			keyPressed:   "x",
			expectedView: app.ViewSearch,
			shouldChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			application := app.NewApp()
			application.SetCurrentView(tt.currentView)

			initialView := application.GetCurrentView()
			assert.Equal(t, tt.currentView, initialView)

			// Simulate key press
			var msg tea.Msg
			switch tt.keyPressed {
			case "tab":
				msg = tea.KeyMsg{Type: tea.KeyTab}
			case "shift+tab":
				msg = tea.KeyMsg{Type: tea.KeyShiftTab}
			default:
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.keyPressed)}
			}

			updatedApp, _ := application.Update(msg)
			newApp := updatedApp.(*app.App)

			if tt.shouldChange {
				assert.Equal(t, tt.expectedView, newApp.GetCurrentView())
				assert.NotEqual(t, initialView, newApp.GetCurrentView())
			} else {
				assert.Equal(t, initialView, newApp.GetCurrentView())
			}
		})
	}
}

func TestApp_QuitHandling(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyCtrlC, tea.KeyCtrlQ} {
		application := app.NewApp()
		assert.Contains(t, application.View(), "Ctrl+C/Q: Quit")
		updatedApp, cmd := application.Update(tea.KeyMsg{Type: key})

		newApp := updatedApp.(*app.App)
		assert.True(t, newApp.IsQuitting(), key.String())
		assert.NotNil(t, cmd, key.String())
	}
}

func TestApp_WindowSizeHandling(t *testing.T) {
	application := app.NewApp()

	// Test window size message
	sizeMsg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedApp, _ := application.Update(sizeMsg)

	newApp := updatedApp.(*app.App)
	width, height := newApp.GetSize()
	assert.Equal(t, 120, width)
	assert.Equal(t, 40, height)
}

func TestApp_ViewTypes(t *testing.T) {
	tests := []struct {
		viewType app.ViewType
		expected string
	}{
		{app.ViewSearch, "search"},
		{app.ViewPlayer, "player"},
		{app.ViewPlaylists, "playlists"},
		{app.ViewFavorites, "favorites"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.viewType.String())
		})
	}
}

func TestApp_BubbleTeaIntegration(t *testing.T) {
	application := app.NewApp()

	// Test that app implements tea.Model interface
	var _ tea.Model = application

	// Test Init returns expected command
	cmd := application.Init()
	assert.NotNil(t, cmd) // Should return some initialization command

	// Test View returns non-empty string
	view := application.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "SoundCloud TUI") // Should contain app title
}

func TestApp_HeaderShowsAuthNotice(t *testing.T) {
	t.Run("signed in", func(t *testing.T) {
		application := app.NewAppWithDependencies(
			&MockSoundCloudClient{Authenticated: true, AuthSourceValue: "Firefox (default)"},
			&MockAudioPlayer{},
			&MockStreamExtractor{},
		)

		view := application.View()
		assert.Contains(t, view, "SoundCloud TUI")
		assert.Contains(t, view, "Signed in via Firefox (default)")
	})

	t.Run("anonymous", func(t *testing.T) {
		application := app.NewAppWithDependencies(
			&MockSoundCloudClient{Authenticated: false},
			&MockAudioPlayer{},
			&MockStreamExtractor{},
		)

		view := application.View()
		assert.Contains(t, view, "SoundCloud TUI")
		assert.Contains(t, view, "anonymous")
	})
}

func TestApp_StateManagement(t *testing.T) {
	application := app.NewApp()

	// Test initial state
	assert.Equal(t, app.ViewSearch, application.GetCurrentView())
	assert.False(t, application.IsQuitting())

	// Test state changes are persistent
	application.SetCurrentView(app.ViewPlayer)
	assert.Equal(t, app.ViewPlayer, application.GetCurrentView())

	// Test multiple view changes
	views := []app.ViewType{app.ViewPlaylists, app.ViewFavorites, app.ViewSearch, app.ViewPlayer}
	for _, view := range views {
		application.SetCurrentView(view)
		assert.Equal(t, view, application.GetCurrentView())
	}
}

func TestApp_PlaylistsTabLoadsAndPlaysTrack(t *testing.T) {
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{
			Authenticated:   true,
			AuthSourceValue: "Firefox (default)",
			LibraryFunc: func() ([]soundcloud.Playlist, error) {
				return []soundcloud.Playlist{
					{ID: 10, Title: "Samson likes", TrackCount: 2, Sharing: "private", Kind: "owned"},
				}, nil
			},
			PlaylistFunc: func(playlistID int64) ([]soundcloud.Track, error) {
				assert.Equal(t, int64(10), playlistID)
				return []soundcloud.Track{
					{ID: 123, Title: "Playlist Track", User: soundcloud.User{Username: "Playlist Artist"}},
				}, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	require.Nil(t, cmd)
	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, app.ViewPlaylists, application.GetCurrentView())

	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "Samson likes")

	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyEnter})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "Playlist Track")

	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyEnter})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "Playlist Track", application.GetCurrentTrack().Title)
}

func TestApp_PlaylistsTabRightAndLeftDrillIntoPlaylist(t *testing.T) {
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{
			Authenticated:   true,
			AuthSourceValue: "Firefox (default)",
			LibraryFunc: func() ([]soundcloud.Playlist, error) {
				return []soundcloud.Playlist{
					{ID: 10, Title: "Samson likes", TrackCount: 2, Sharing: "private", Kind: "owned"},
				}, nil
			},
			PlaylistFunc: func(playlistID int64) ([]soundcloud.Track, error) {
				assert.Equal(t, int64(10), playlistID)
				return []soundcloud.Track{
					{ID: 123, Title: "Playlist Track", User: soundcloud.User{Username: "Playlist Artist"}},
				}, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	updated, _ := application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "Samson likes")

	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyRight})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "Playlist Track")

	updated, _ = application.Update(tea.KeyMsg{Type: tea.KeyLeft})
	application = updated.(*app.App)
	view := application.View()
	assert.Contains(t, view, "Samson likes")
	assert.NotContains(t, view, "Playlist Track")
}

func TestApp_F5RefreshesLibraryAndOpenedPlaylistTracks(t *testing.T) {
	libraryCalls := 0
	favoriteCalls := 0
	playlistTrackCalls := 0
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{
			LibraryFunc: func() ([]soundcloud.Playlist, error) {
				libraryCalls++
				return []soundcloud.Playlist{{ID: 10, Title: "Refresh me"}}, nil
			},
			FavoritesFunc: func() ([]soundcloud.Track, error) {
				favoriteCalls++
				return []soundcloud.Track{{ID: int64(favoriteCalls), Title: fmt.Sprintf("Favorite %d", favoriteCalls)}}, nil
			},
			PlaylistFunc: func(playlistID int64) ([]soundcloud.Track, error) {
				assert.Equal(t, int64(10), playlistID)
				playlistTrackCalls++
				return []soundcloud.Track{{ID: int64(playlistTrackCalls), Title: fmt.Sprintf("Playlist Track %d", playlistTrackCalls)}}, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	updated, _ := application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	updated, loadPlaylistsCmd := application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	require.NotNil(t, loadPlaylistsCmd)
	updated, _ = application.Update(loadPlaylistsCmd())
	application = updated.(*app.App)

	updated, loadTracksCmd := application.Update(tea.KeyMsg{Type: tea.KeyEnter})
	application = updated.(*app.App)
	require.NotNil(t, loadTracksCmd)
	updated, _ = application.Update(loadTracksCmd())
	application = updated.(*app.App)
	require.Equal(t, 1, libraryCalls)
	require.Equal(t, 1, playlistTrackCalls)

	updated, refreshCmd := application.Update(tea.KeyMsg{Type: tea.KeyF5})
	application = updated.(*app.App)
	require.NotNil(t, refreshCmd)
	batch, ok := refreshCmd().(tea.BatchMsg)
	require.True(t, ok)
	require.Len(t, batch, 3)
	for _, command := range batch {
		updated, _ = application.Update(command())
		application = updated.(*app.App)
	}

	assert.Equal(t, 2, libraryCalls)
	assert.Equal(t, 1, favoriteCalls)
	assert.Equal(t, 2, playlistTrackCalls)
	assert.Contains(t, application.View(), "Playlist Track 2")
}

func TestApp_FooterUsesCompactGlobalHelp(t *testing.T) {
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	view := application.View()
	assert.Contains(t, view, "[Shift-]Tab: [Previous/]Next View")
	assert.Contains(t, view, "F5: Refresh Library")
	assert.Contains(t, view, "Ctrl+C/Q: Quit")
	assert.NotContains(t, view, "Tab: Next View")
	assert.NotContains(t, view, "Ctrl+C/Ctrl+Q: Quit")
}

func TestApp_PlaylistsTabWindowsManyPlaylistsAroundSelection(t *testing.T) {
	playlists := make([]soundcloud.Playlist, 40)
	for i := range playlists {
		playlists[i] = soundcloud.Playlist{
			ID:         int64(i + 1),
			Title:      fmt.Sprintf("Playlist %03d", i+1),
			TrackCount: 1,
			Kind:       "owned",
		}
	}
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{
			Authenticated:   true,
			AuthSourceValue: "Firefox (default)",
			LibraryFunc: func() ([]soundcloud.Playlist, error) {
				return playlists, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)
	updated, _ := application.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	application = updated.(*app.App)
	updated, _ = application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)

	for i := 0; i < 29; i++ {
		updated, _ = application.Update(tea.KeyMsg{Type: tea.KeyDown})
		application = updated.(*app.App)
	}

	view := application.View()
	assert.Contains(t, view, "Playlist 030")
	assert.NotContains(t, view, "Playlist 001")
}

func TestApp_FavoritesTabLoadsAndPlaysTrack(t *testing.T) {
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{
			Authenticated:   true,
			AuthSourceValue: "Firefox (default)",
			FavoritesFunc: func() ([]soundcloud.Track, error) {
				return []soundcloud.Track{
					{ID: 456, Title: "Favorite Track", User: soundcloud.User{Username: "Favorite Artist"}},
				}, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)

	assert.Equal(t, app.ViewFavorites, application.GetCurrentView())
	assert.Contains(t, application.View(), "Favorite Track")

	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyEnter})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "Favorite Track", application.GetCurrentTrack().Title)
}

func TestApp_CollectionPlaybackUsesShiftArrowsAndSkipsUnavailableTracks(t *testing.T) {
	tracks := []soundcloud.Track{
		{ID: 1, Title: "First", User: soundcloud.User{Username: "Artist"}},
		{ID: 2, Title: "Unavailable", User: soundcloud.User{Username: "Artist"}},
		{ID: 3, Title: "Playable", User: soundcloud.User{Username: "Artist"}},
	}
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{
			Authenticated: true,
			FavoritesFunc: func() ([]soundcloud.Track, error) {
				return tracks, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)

	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyEnter})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "First", application.GetCurrentTrack().Title)
	assert.Contains(t, application.View(), "←→: Seek")
	assert.Contains(t, application.View(), "Shift+←→: Previous/Next Track")

	// Plain arrows retain their seek behavior even while a collection is active.
	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyRight})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "First", application.GetCurrentTrack().Title)

	// Shift+Right advances the collection before the first async request returns.
	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "Unavailable", application.GetCurrentTrack().Title)

	updated, cmd = application.Update(player.PlaybackFailedMsg{
		Track: application.GetCurrentTrack(),
		Error: fmt.Errorf("HTTP 404"),
	})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "Unavailable", application.GetCurrentTrack().Title)
	assert.Contains(t, application.View(), "▶ Playable")
	assert.Contains(t, application.View(), "Skipped Unavailable: HTTP 404")

	updated, cmd = application.Update(app.CollectionAdvanceMsg{TrackID: 3})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "Playable", application.GetCurrentTrack().Title)
}

func TestApp_CollectionPlaybackAdvancesAfterTrackCompletion(t *testing.T) {
	tracks := []soundcloud.Track{
		{ID: 1, Title: "First", User: soundcloud.User{Username: "Artist"}},
		{ID: 2, Title: "Second", User: soundcloud.User{Username: "Artist"}},
	}
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{
			Authenticated: true,
			PlaylistFunc: func(playlistID int64) ([]soundcloud.Track, error) {
				return tracks, nil
			},
			LibraryFunc: func() ([]soundcloud.Playlist, error) {
				return []soundcloud.Playlist{{ID: 10, Title: "Queue"}}, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	updated, _ := application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyEnter})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyEnter})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "First", application.GetCurrentTrack().Title)

	updated, cmd = application.Update(player.PlaybackCompletedMsg{Track: application.GetCurrentTrack()})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "Second", application.GetCurrentTrack().Title)
}

func TestApp_MouseClicksTabsAndDoubleClicksLibraryItems(t *testing.T) {
	now := time.Date(2026, time.July, 10, 9, 0, 0, 0, time.UTC)
	tracks := []soundcloud.Track{
		{ID: 1, Title: "First", User: soundcloud.User{Username: "Artist"}},
		{ID: 2, Title: "Second", User: soundcloud.User{Username: "Artist"}},
	}
	application := app.NewAppWithDependenciesAndClock(
		&MockSoundCloudClient{
			Authenticated: true,
			LibraryFunc: func() ([]soundcloud.Playlist, error) {
				return []soundcloud.Playlist{
					{ID: 10, Title: "First Playlist"},
					{ID: 20, Title: "Clicked Playlist"},
				}, nil
			},
			PlaylistFunc: func(playlistID int64) ([]soundcloud.Track, error) {
				return tracks, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
		func() time.Time { return now },
	)

	// The Playlists tab occupies the third tab at the rendered header's tab row.
	updated, cmd := application.Update(mousePress(24, 3))
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, app.ViewPlaylists, application.GetCurrentView())
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)

	// A single click selects the second visible playlist.
	updated, _ = application.Update(mousePress(4, 11))
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "▶ Clicked Playlist")

	// A second click opens that selected playlist.
	now = now.Add(100 * time.Millisecond)
	updated, cmd = application.Update(mousePress(4, 11))
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "Second")

	// A single click selects the second track, and a second click starts it.
	now = now.Add(100 * time.Millisecond)
	updated, _ = application.Update(mousePress(4, 11))
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "▶ Second")
	now = now.Add(100 * time.Millisecond)
	updated, cmd = application.Update(mousePress(4, 11))
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	assert.Equal(t, "Second", application.GetCurrentTrack().Title)
}

func TestApp_MouseMotionHighlightsTabWithoutSelectingIt(t *testing.T) {
	colorProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(colorProfile) })

	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)
	initialView := application.View()

	updated, _ := application.Update(mouseMotion(14, 3))
	application = updated.(*app.App)
	assert.Equal(t, app.ViewSearch, application.GetCurrentView())
	assert.NotEqual(t, initialView, application.View())
	assert.Contains(t, application.View(), styles.HoveredTabStyle.Render("Player"))

	updated, _ = application.Update(mouseMotion(79, 7))
	application = updated.(*app.App)
	assert.Equal(t, app.ViewSearch, application.GetCurrentView())
	assert.Equal(t, initialView, application.View())
}

func TestApp_MouseWheelNavigatesFavoritesAndPlaylistTracks(t *testing.T) {
	tracks := []soundcloud.Track{
		{ID: 1, Title: "First", User: soundcloud.User{Username: "Artist"}},
		{ID: 2, Title: "Second", User: soundcloud.User{Username: "Artist"}},
	}
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{
			LibraryFunc: func() ([]soundcloud.Playlist, error) {
				return []soundcloud.Playlist{{ID: 10, Title: "Playlist"}}, nil
			},
			PlaylistFunc: func(playlistID int64) ([]soundcloud.Track, error) {
				return tracks, nil
			},
			FavoritesFunc: func() ([]soundcloud.Track, error) {
				return tracks, nil
			},
		},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	updated, _ = application.Update(wheelDown())
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "▶ Second")

	updated, _ = application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	updated, _ = application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	updated, cmd = application.Update(tea.KeyMsg{Type: tea.KeyEnter})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)
	updated, _ = application.Update(wheelDown())
	application = updated.(*app.App)
	assert.Contains(t, application.View(), "▶ Second")
	assert.Nil(t, application.GetCurrentTrack())
}

func TestApp_LongContentFitsBelowHeaderAndAboveFooter(t *testing.T) {
	tracks := make([]soundcloud.Track, 40)
	for i := range tracks {
		tracks[i] = soundcloud.Track{ID: int64(i + 1), Title: fmt.Sprintf("Favorite %d", i+1)}
	}
	application := app.NewAppWithDependencies(
		&MockSoundCloudClient{FavoritesFunc: func() ([]soundcloud.Track, error) { return tracks, nil }},
		&MockAudioPlayer{},
		&MockStreamExtractor{},
	)

	updated, _ := application.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	application = updated.(*app.App)
	assert.LessOrEqual(t, lipgloss.Height(application.View()), 16)
	assert.Contains(t, application.View(), "SoundCloud TUI")
	updated, cmd := application.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	application = updated.(*app.App)
	require.NotNil(t, cmd)
	updated, _ = application.Update(cmd())
	application = updated.(*app.App)

	assert.LessOrEqual(t, lipgloss.Height(application.View()), 16)
	assert.Contains(t, application.View(), "SoundCloud TUI")
	assert.Contains(t, application.View(), "Favorites")

	application.SetCurrentView(app.ViewPlayer)
	assert.LessOrEqual(t, lipgloss.Height(application.View()), 16)
	assert.Contains(t, application.View(), "SoundCloud TUI")
}

func mousePress(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func mouseMotion(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion}
}

func wheelDown() tea.MouseMsg {
	return tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
}

func TestApp_ErrorHandling(t *testing.T) {
	application := app.NewApp()

	// Test with nil message
	updatedApp, cmd := application.Update(nil)
	assert.NotNil(t, updatedApp)
	assert.Nil(t, cmd) // Should handle gracefully

	// Test with unknown message type
	unknownMsg := "unknown message"
	updatedApp, cmd = application.Update(unknownMsg)
	assert.NotNil(t, updatedApp)
	// Should not crash and return the app
}
