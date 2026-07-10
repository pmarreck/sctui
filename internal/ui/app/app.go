package app

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
	"soundcloud-tui/internal/ui/components/player"
	"soundcloud-tui/internal/ui/components/search"
	"soundcloud-tui/internal/ui/styles"
)

// SoundCloudClient is the app's SoundCloud port: search/playback methods plus
// logged-in library access for personal playlists and favorites.
type SoundCloudClient interface {
	soundcloud.ClientInterface
	IsAuthenticated() bool
	AuthSource() string
	Library() ([]soundcloud.Playlist, error)
	PlaylistTracks(playlistID int64) ([]soundcloud.Track, error)
	FavoriteTracks() ([]soundcloud.Track, error)
}

// ViewType represents the different views in the application
type ViewType int

const (
	ViewSearch ViewType = iota
	ViewPlayer
	ViewPlaylists
	ViewFavorites
)

// String returns the string representation of ViewType
func (v ViewType) String() string {
	switch v {
	case ViewSearch:
		return "search"
	case ViewPlayer:
		return "player"
	case ViewPlaylists:
		return "playlists"
	case ViewFavorites:
		return "favorites"
	default:
		return "unknown"
	}
}

type loadState int

const (
	loadNotStarted loadState = iota
	loadLoading
	loadLoaded
	loadError
)

type playlistMode int

const (
	playlistModeList playlistMode = iota
	playlistModeTracks
)

type playlistsLoadedMsg struct {
	playlists []soundcloud.Playlist
	err       error
}

type playlistTracksLoadedMsg struct {
	playlist soundcloud.Playlist
	tracks   []soundcloud.Track
	err      error
}

type favoritesLoadedMsg struct {
	tracks []soundcloud.Track
	err    error
}

type libraryMouseTarget int

const (
	mouseTargetNone libraryMouseTarget = iota
	mouseTargetPlaylist
	mouseTargetPlaylistTrack
	mouseTargetFavoriteTrack
)

const mouseDoubleClickWindow = 400 * time.Millisecond

type mouseClick struct {
	target libraryMouseTarget
	index  int
	at     time.Time
}

// App represents the main application model
type App struct {
	// Window size
	width  int
	height int

	// Current view
	currentView ViewType
	quitting    bool

	// Components
	searchComponent *search.SearchComponent
	playerComponent *player.PlayerComponent

	// Dependencies
	soundCloudClient SoundCloudClient
	audioPlayer      audio.Player
	streamExtractor  audio.StreamExtractor
	authNotice       string

	// Library tab state
	playlistsState         loadState
	playlistsMode          playlistMode
	playlists              []soundcloud.Playlist
	playlistSelectedIndex  int
	selectedPlaylist       *soundcloud.Playlist
	playlistTracksState    loadState
	playlistTracks         []soundcloud.Track
	playlistTrackIndex     int
	playlistError          error
	playlistTrackLoadError error

	// Favorites tab state
	favoritesState        loadState
	favoriteTracks        []soundcloud.Track
	favoriteSelectedIndex int
	favoritesError        error

	// Collection playback survives leaving its source view, allowing next/previous
	// and auto-advance to follow the selected playlist or favorites ordering.
	playbackCollection      []soundcloud.Track
	playbackCollectionIndex int

	now       func() time.Time
	lastClick mouseClick
}

// NewApp creates a new application instance
func NewApp() *App {
	// Initialize SoundCloud client
	client, err := soundcloud.NewClient()
	var appClient SoundCloudClient
	if err == nil && client != nil {
		appClient = client
	}

	// Initialize audio player with buffered streaming for better responsiveness
	audioPlayer := audio.NewBufferedBeepPlayer()

	// Initialize real stream extractor with the SoundCloud client
	streamExtractor := audio.NewRealSoundCloudStreamExtractor(client)

	return NewAppWithDependencies(appClient, audioPlayer, streamExtractor)
}

// NewAppWithDependencies creates an app with explicit ports for deterministic
// tests and future alternate frontends, while NewApp keeps production defaults.
func NewAppWithDependencies(client SoundCloudClient, audioPlayer audio.Player, streamExtractor audio.StreamExtractor) *App {
	return NewAppWithDependenciesAndClock(client, audioPlayer, streamExtractor, time.Now)
}

// NewAppWithDependenciesAndClock injects the double-click clock so mouse input
// behavior stays deterministic under test without sleeping.
func NewAppWithDependenciesAndClock(client SoundCloudClient, audioPlayer audio.Player, streamExtractor audio.StreamExtractor, now func() time.Time) *App {
	if now == nil {
		now = time.Now
	}
	// Initialize components
	searchComponent := search.NewSearchComponent(client)
	playerComponent := player.NewPlayerComponent(audioPlayer, streamExtractor)

	application := &App{
		width:               80,
		height:              24,
		currentView:         ViewSearch,
		quitting:            false,
		searchComponent:     searchComponent,
		playerComponent:     playerComponent,
		soundCloudClient:    client,
		audioPlayer:         audioPlayer,
		streamExtractor:     streamExtractor,
		authNotice:          renderAuthNotice(client),
		playlistsState:      loadNotStarted,
		playlistsMode:       playlistModeList,
		playlistTracksState: loadNotStarted,
		favoritesState:      loadNotStarted,
		now:                 now,
	}
	application.setComponentSizes()
	return application
}

func renderAuthNotice(client SoundCloudClient) string {
	if client == nil || !client.IsAuthenticated() {
		return "🔒 anonymous"
	}
	if client.AuthSource() == "" {
		return "🔓 Signed in"
	}
	return fmt.Sprintf("🔓 Signed in via %s", client.AuthSource())
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.searchComponent.Init(),
		a.playerComponent.Init(),
	)
}

// Update handles messages and updates the application state
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global key handling
		switch msg.Type {
		case tea.KeyCtrlC:
			a.quitting = true
			return a, tea.Quit

		case tea.KeyTab:
			a.nextView()
			return a, a.activateCurrentView()

		case tea.KeyShiftTab:
			a.previousView()
			return a, a.activateCurrentView()

		case tea.KeySpace:
			// Always pass space key to player component for play/pause
			updatedPlayer, playerCmd := a.playerComponent.Update(msg)
			a.playerComponent = updatedPlayer.(*player.PlayerComponent)
			if playerCmd != nil {
				cmds = append(cmds, playerCmd)
			}
			return a, tea.Batch(cmds...)

		case tea.KeyLeft, tea.KeyRight:
			if a.currentView == ViewPlaylists && a.playlistsMode == playlistModeList {
				if cmd := a.handlePlaylistsKey(msg); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return a, tea.Batch(cmds...)
			}
			if a.hasPlaybackCollection() {
				delta := 1
				if msg.Type == tea.KeyLeft {
					delta = -1
				}
				if cmd := a.skipCollectionTrack(delta); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return a, tea.Batch(cmds...)
			}
			if a.currentView == ViewPlaylists {
				if cmd := a.handlePlaylistsKey(msg); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return a, tea.Batch(cmds...)
			}

			// Always pass seek keys to player component
			updatedPlayer, playerCmd := a.playerComponent.Update(msg)
			a.playerComponent = updatedPlayer.(*player.PlayerComponent)
			if playerCmd != nil {
				cmds = append(cmds, playerCmd)
			}
			return a, tea.Batch(cmds...)

		case tea.KeyRunes:
			// Handle volume controls globally
			if len(msg.Runes) > 0 {
				switch string(msg.Runes) {
				case "+", "=", "-":
					// Always pass volume keys to player component
					updatedPlayer, playerCmd := a.playerComponent.Update(msg)
					a.playerComponent = updatedPlayer.(*player.PlayerComponent)
					if playerCmd != nil {
						cmds = append(cmds, playerCmd)
					}
					return a, tea.Batch(cmds...)
				}
			}
		}

		// Pass key messages to current view
		switch a.currentView {
		case ViewSearch:
			updatedSearch, cmd := a.searchComponent.Update(msg)
			a.searchComponent = updatedSearch.(*search.SearchComponent)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

			// Handle track selection from search
			if selectedTrack := a.searchComponent.GetSelectedTrack(); selectedTrack != nil {
				a.clearPlaybackCollection()
				// Don't clear selection immediately - wait for playback result
				playCmd := player.PlayTrackMsg{Track: selectedTrack}
				updatedPlayer, playerCmd := a.playerComponent.Update(playCmd)
				a.playerComponent = updatedPlayer.(*player.PlayerComponent)
				if playerCmd != nil {
					cmds = append(cmds, playerCmd)
				}
			}

		case ViewPlayer:
			updatedPlayer, cmd := a.playerComponent.Update(msg)
			a.playerComponent = updatedPlayer.(*player.PlayerComponent)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case ViewPlaylists:
			if cmd := a.handlePlaylistsKey(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		case ViewFavorites:
			if cmd := a.handleFavoritesKey(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case tea.MouseMsg:
		return a.handleMouse(msg)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.setComponentSizes()

	case playlistsLoadedMsg:
		if msg.err != nil {
			a.playlistsState = loadError
			a.playlistError = msg.err
			return a, nil
		}
		a.playlistsState = loadLoaded
		a.playlists = msg.playlists
		a.playlistSelectedIndex = clampIndex(a.playlistSelectedIndex, len(a.playlists))
		a.playlistError = nil
		return a, nil

	case playlistTracksLoadedMsg:
		if msg.err != nil {
			a.playlistTracksState = loadError
			a.playlistTrackLoadError = msg.err
			return a, nil
		}
		a.playlistTracksState = loadLoaded
		a.playlistTracks = msg.tracks
		a.playlistTrackIndex = 0
		a.playlistTrackLoadError = nil
		playlist := msg.playlist
		a.selectedPlaylist = &playlist
		a.playlistsMode = playlistModeTracks
		return a, nil

	case favoritesLoadedMsg:
		if msg.err != nil {
			a.favoritesState = loadError
			a.favoritesError = msg.err
			return a, nil
		}
		a.favoritesState = loadLoaded
		a.favoriteTracks = msg.tracks
		a.favoriteSelectedIndex = clampIndex(a.favoriteSelectedIndex, len(a.favoriteTracks))
		a.favoritesError = nil
		return a, nil

	case player.PlaybackStartedMsg:
		// Playback started successfully - reset search state
		a.searchComponent.ClearSelection()
		a.searchComponent.ResetToResults()
		// Switch to player view to show playback
		a.currentView = ViewPlayer
		return a, nil

	case player.PlaybackFailedMsg:
		// Playback failed - reset search state and show error
		a.searchComponent.ClearSelection()
		a.searchComponent.ResetToResults()
		if cmd := a.advanceAfterCollectionTrack(msg.Track); cmd != nil {
			return a, cmd
		}
		return a, nil

	case player.PlaybackCompletedMsg:
		return a, a.advanceAfterCollectionTrack(msg.Track)

	default:
		// Pass other messages to components
		updatedSearch, searchCmd := a.searchComponent.Update(msg)
		a.searchComponent = updatedSearch.(*search.SearchComponent)
		if searchCmd != nil {
			cmds = append(cmds, searchCmd)
		}

		updatedPlayer, playerCmd := a.playerComponent.Update(msg)
		a.playerComponent = updatedPlayer.(*player.PlayerComponent)
		if playerCmd != nil {
			cmds = append(cmds, playerCmd)
		}
	}

	return a, tea.Batch(cmds...)
}

// View renders the application
func (a *App) View() string {
	if a.quitting {
		return "Goodbye!\n"
	}

	// Build the view
	var view string

	// Header
	header := a.renderHeader()

	// Main content based on current view
	var content string
	switch a.currentView {
	case ViewSearch:
		content = a.searchComponent.View()
	case ViewPlayer:
		content = a.playerComponent.View()
	case ViewPlaylists:
		content = a.renderPlaylistsView()
	case ViewFavorites:
		content = a.renderFavoritesView()
	}

	// Footer
	footer := a.renderFooter()

	// Combine all parts
	view = lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		footer,
	)

	return view
}

// renderHeader renders the application header
func (a *App) renderHeader() string {
	title := styles.TitleStyle.Render("SoundCloud TUI")
	auth := styles.StatusStyle.Render(a.authNotice)

	// Navigation tabs
	tabs := []string{}
	for i, viewName := range []string{"Search", "Player", "Playlists", "Favorites"} {
		if ViewType(i) == a.currentView {
			tabs = append(tabs, styles.ActiveTabStyle.Render(viewName))
		} else {
			tabs = append(tabs, styles.InactiveTabStyle.Render(viewName))
		}
	}

	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	// Combine title and tabs
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		auth,
		tabBar,
	)

	return styles.HeaderStyle.Render(header)
}

// renderFooter renders the application footer
func (a *App) renderFooter() string {
	helpText := "Tab: Next View • Shift+Tab: Previous View • Ctrl+C: Quit"

	// Add global audio controls (work from any view)
	if a.playerComponent.GetCurrentTrack() != nil {
		arrowControls := "←→: Seek"
		if a.hasPlaybackCollection() {
			arrowControls = "←→: Previous/Next Track"
		}
		helpText += " • Space: Play/Pause • " + arrowControls + " • +/-: Volume"
	}

	// Add view-specific help
	switch a.currentView {
	case ViewSearch:
		helpText += " • Enter: Search • ↑↓: Navigate • Enter: Select"
	case ViewPlayer:
		// Player-specific controls already shown above
		helpText += ""
	case ViewPlaylists:
		helpText += " • ↑↓: Navigate • →/Enter: Open/Play • ←/Esc: Back"
	case ViewFavorites:
		helpText += " • ↑↓: Navigate • Enter: Play"
	}

	return styles.FooterStyle.Render(helpText)
}

func (a *App) activateCurrentView() tea.Cmd {
	switch a.currentView {
	case ViewPlaylists:
		if a.playlistsState == loadNotStarted {
			return a.loadPlaylistsCmd()
		}
	case ViewFavorites:
		if a.favoritesState == loadNotStarted {
			return a.loadFavoritesCmd()
		}
	}
	return nil
}

func (a *App) loadPlaylistsCmd() tea.Cmd {
	a.playlistsState = loadLoading
	return func() tea.Msg {
		if a.soundCloudClient == nil {
			return playlistsLoadedMsg{err: fmt.Errorf("no SoundCloud client available")}
		}
		playlists, err := a.soundCloudClient.Library()
		return playlistsLoadedMsg{playlists: playlists, err: err}
	}
}

func (a *App) loadPlaylistTracksCmd(playlist soundcloud.Playlist) tea.Cmd {
	a.playlistTracksState = loadLoading
	a.playlistTrackLoadError = nil
	return func() tea.Msg {
		if a.soundCloudClient == nil {
			return playlistTracksLoadedMsg{playlist: playlist, err: fmt.Errorf("no SoundCloud client available")}
		}
		tracks, err := a.soundCloudClient.PlaylistTracks(playlist.ID)
		return playlistTracksLoadedMsg{playlist: playlist, tracks: tracks, err: err}
	}
}

func (a *App) loadFavoritesCmd() tea.Cmd {
	a.favoritesState = loadLoading
	return func() tea.Msg {
		if a.soundCloudClient == nil {
			return favoritesLoadedMsg{err: fmt.Errorf("no SoundCloud client available")}
		}
		tracks, err := a.soundCloudClient.FavoriteTracks()
		return favoritesLoadedMsg{tracks: tracks, err: err}
	}
}

func (a *App) handlePlaylistsKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyLeft:
		if a.playlistsMode == playlistModeTracks {
			a.returnToPlaylistList()
		}
	case tea.KeyRight:
		if a.playlistsMode == playlistModeList {
			return a.openSelectedPlaylist()
		}
	case tea.KeyUp:
		if a.playlistsMode == playlistModeTracks {
			a.playlistTrackIndex = moveIndex(a.playlistTrackIndex, len(a.playlistTracks), -1)
		} else {
			a.playlistSelectedIndex = moveIndex(a.playlistSelectedIndex, len(a.playlists), -1)
		}
	case tea.KeyDown:
		if a.playlistsMode == playlistModeTracks {
			a.playlistTrackIndex = moveIndex(a.playlistTrackIndex, len(a.playlistTracks), 1)
		} else {
			a.playlistSelectedIndex = moveIndex(a.playlistSelectedIndex, len(a.playlists), 1)
		}
	case tea.KeyEsc:
		if a.playlistsMode == playlistModeTracks {
			a.returnToPlaylistList()
		}
	case tea.KeyEnter:
		if a.playlistsMode == playlistModeTracks {
			if len(a.playlistTracks) == 0 {
				return nil
			}
			return a.playCollectionTrack(a.playlistTracks, a.playlistTrackIndex)
		}
		if len(a.playlists) == 0 {
			return nil
		}
		return a.openSelectedPlaylist()
	}
	return nil
}

func (a *App) returnToPlaylistList() {
	a.playlistsMode = playlistModeList
	a.playlistTracks = nil
	a.selectedPlaylist = nil
	a.playlistTracksState = loadNotStarted
	a.playlistTrackLoadError = nil
}

func (a *App) openSelectedPlaylist() tea.Cmd {
	if len(a.playlists) == 0 {
		return nil
	}
	playlist := a.playlists[a.playlistSelectedIndex]
	if playlist.ID == 0 {
		a.playlistTrackLoadError = fmt.Errorf("this playlist cannot be opened from the TUI yet")
		a.playlistTracksState = loadError
		return nil
	}
	return a.loadPlaylistTracksCmd(playlist)
}

func (a *App) handleFavoritesKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyUp:
		a.favoriteSelectedIndex = moveIndex(a.favoriteSelectedIndex, len(a.favoriteTracks), -1)
	case tea.KeyDown:
		a.favoriteSelectedIndex = moveIndex(a.favoriteSelectedIndex, len(a.favoriteTracks), 1)
	case tea.KeyEnter:
		if len(a.favoriteTracks) == 0 {
			return nil
		}
		return a.playCollectionTrack(a.favoriteTracks, a.favoriteSelectedIndex)
	}
	return nil
}

func (a *App) playTrack(track *soundcloud.Track) tea.Cmd {
	updatedPlayer, cmd := a.playerComponent.Update(player.PlayTrackMsg{Track: track})
	a.playerComponent = updatedPlayer.(*player.PlayerComponent)
	return cmd
}

// playCollectionTrack snapshots a playlist or Favorites ordering before
// playback so navigation away from the library cannot lose auto-advance state.
func (a *App) playCollectionTrack(tracks []soundcloud.Track, index int) tea.Cmd {
	if len(tracks) == 0 {
		return nil
	}
	a.playbackCollection = append([]soundcloud.Track(nil), tracks...)
	a.playbackCollectionIndex = clampIndex(index, len(a.playbackCollection))
	a.playerComponent.SetCollectionNavigation(true)
	return a.playCurrentCollectionTrack()
}

func (a *App) hasPlaybackCollection() bool {
	return len(a.playbackCollection) > 0 &&
		a.playbackCollectionIndex >= 0 &&
		a.playbackCollectionIndex < len(a.playbackCollection)
}

func (a *App) clearPlaybackCollection() {
	a.playbackCollection = nil
	a.playbackCollectionIndex = 0
	a.playerComponent.SetCollectionNavigation(false)
}

func (a *App) skipCollectionTrack(delta int) tea.Cmd {
	if !a.hasPlaybackCollection() {
		return nil
	}
	next := a.playbackCollectionIndex + delta
	if next < 0 || next >= len(a.playbackCollection) {
		return nil
	}
	a.playbackCollectionIndex = next
	return a.playCurrentCollectionTrack()
}

func (a *App) advanceAfterCollectionTrack(track *soundcloud.Track) tea.Cmd {
	if !a.hasPlaybackCollection() || track == nil ||
		a.playbackCollection[a.playbackCollectionIndex].ID != track.ID {
		return nil
	}
	return a.skipCollectionTrack(1)
}

func (a *App) playCurrentCollectionTrack() tea.Cmd {
	if !a.hasPlaybackCollection() {
		return nil
	}
	track := a.playbackCollection[a.playbackCollectionIndex]
	return a.playTrack(&track)
}

func (a *App) renderPlaylistsView() string {
	if a.playlistsState == loadLoading {
		return styles.SearchBoxStyle.Render(styles.LoadingStatusStyle.Render("Loading playlists..."))
	}
	if a.playlistsState == loadError {
		return styles.SearchBoxStyle.Render(styles.ErrorStatusStyle.Render(a.playlistError.Error()))
	}
	if a.playlistsMode == playlistModeTracks {
		return a.renderPlaylistTracksView()
	}
	if len(a.playlists) == 0 {
		return styles.SearchResultsStyle.Render(styles.StatusStyle.Render("No playlists found."))
	}

	items := make([]string, 0, len(a.playlists))
	visibleStart, visibleEnd := visibleWindow(len(a.playlists), a.playlistSelectedIndex, a.libraryVisibleItems())
	for i := visibleStart; i < visibleEnd; i++ {
		playlist := a.playlists[i]
		visibility := ""
		if playlist.IsPrivate() {
			visibility = " 🔒"
		}
		item := fmt.Sprintf("%-48s %s%s (%d tracks)",
			styles.TruncateText(playlist.Title, 48),
			playlist.Kind,
			visibility,
			playlist.TrackCount,
		)
		if i == a.playlistSelectedIndex {
			items = append(items, styles.SelectedListItemStyle.Render("▶ "+item))
		} else {
			items = append(items, styles.ListItemStyle.Render("  "+item))
		}
	}

	content := a.renderLibraryContent(
		styles.TrackTitleStyle.Render("Personal Playlists "+rangeIndicator(visibleStart, visibleEnd, len(a.playlists))),
		items,
	)
	return a.libraryResultsStyle().Render(content)
}

func (a *App) renderPlaylistTracksView() string {
	title := "Playlist Tracks"
	if a.selectedPlaylist != nil {
		title = a.selectedPlaylist.Title
	}
	if a.playlistTracksState == loadLoading {
		return styles.SearchBoxStyle.Render(styles.LoadingStatusStyle.Render("Loading tracks..."))
	}
	if a.playlistTracksState == loadError {
		return styles.SearchBoxStyle.Render(styles.ErrorStatusStyle.Render(a.playlistTrackLoadError.Error()))
	}
	if len(a.playlistTracks) == 0 {
		return styles.SearchResultsStyle.Render(styles.StatusStyle.Render("No tracks found in " + title + "."))
	}

	items := make([]string, 0, len(a.playlistTracks))
	visibleStart, visibleEnd := visibleWindow(len(a.playlistTracks), a.playlistTrackIndex, a.libraryVisibleItems())
	for i := visibleStart; i < visibleEnd; i++ {
		track := a.playlistTracks[i]
		item := fmt.Sprintf("%-50s %s (%s)",
			styles.TruncateText(track.Title, 50),
			track.Artist(),
			track.DurationString(),
		)
		if i == a.playlistTrackIndex {
			items = append(items, styles.SelectedListItemStyle.Render("▶ "+item))
		} else {
			items = append(items, styles.ListItemStyle.Render("  "+item))
		}
	}

	content := a.renderLibraryContent(
		styles.TrackTitleStyle.Render(title+" "+rangeIndicator(visibleStart, visibleEnd, len(a.playlistTracks))),
		items,
	)
	return a.libraryResultsStyle().Render(content)
}

func (a *App) renderFavoritesView() string {
	if a.favoritesState == loadLoading {
		return styles.SearchBoxStyle.Render(styles.LoadingStatusStyle.Render("Loading favorites..."))
	}
	if a.favoritesState == loadError {
		return styles.SearchBoxStyle.Render(styles.ErrorStatusStyle.Render(a.favoritesError.Error()))
	}
	if len(a.favoriteTracks) == 0 {
		return styles.SearchResultsStyle.Render(styles.StatusStyle.Render("No favorite tracks found."))
	}

	items := make([]string, 0, len(a.favoriteTracks))
	visibleStart, visibleEnd := visibleWindow(len(a.favoriteTracks), a.favoriteSelectedIndex, a.libraryVisibleItems())
	for i := visibleStart; i < visibleEnd; i++ {
		track := a.favoriteTracks[i]
		item := fmt.Sprintf("%-50s %s (%s)",
			styles.TruncateText(track.Title, 50),
			track.Artist(),
			track.DurationString(),
		)
		if i == a.favoriteSelectedIndex {
			items = append(items, styles.SelectedListItemStyle.Render("▶ "+item))
		} else {
			items = append(items, styles.ListItemStyle.Render("  "+item))
		}
	}

	content := a.renderLibraryContent(
		styles.TrackTitleStyle.Render("Favorites "+rangeIndicator(visibleStart, visibleEnd, len(a.favoriteTracks))),
		items,
	)
	return a.libraryResultsStyle().Render(content)
}

func clampIndex(index, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func moveIndex(index, length, delta int) int {
	return clampIndex(index+delta, length)
}

func (a *App) libraryVisibleItems() int {
	maxVisible := a.libraryContentHeight() - 6
	if a.usesCompactLibraryLayout() {
		maxVisible = a.libraryContentHeight() - 3
	}
	if maxVisible < 1 {
		return 1
	}
	return maxVisible
}

// libraryContentHeight reserves exact rendered header/footer heights so long
// collection views cannot scroll the application chrome off the terminal.
func (a *App) libraryContentHeight() int {
	height := a.height - lipgloss.Height(a.renderHeader()) - lipgloss.Height(a.renderFooter())
	if height < 1 {
		return 1
	}
	return height
}

func (a *App) setComponentSizes() {
	contentHeight := a.libraryContentHeight()
	a.searchComponent.SetSize(a.width, contentHeight)
	// PlayerComponent's bordered panel adds three rows around its requested
	// height, so pass one extra row before its internal four-row adjustment.
	a.playerComponent.SetSize(a.width, contentHeight+1)
}

func (a *App) usesCompactLibraryLayout() bool {
	return a.libraryContentHeight() < 7
}

func (a *App) libraryResultsStyle() lipgloss.Style {
	innerHeight := a.libraryContentHeight() - 2 // Rounded border top + bottom.
	if innerHeight < 1 {
		innerHeight = 1
	}
	style := styles.SearchResultsStyle.Height(innerHeight)
	if a.usesCompactLibraryLayout() {
		return style.Padding(0)
	}
	return style
}

func (a *App) renderLibraryContent(title string, items []string) string {
	parts := []string{title}
	if !a.usesCompactLibraryLayout() {
		parts = append(parts, "")
	}
	parts = append(parts, items...)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (a *App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return a, nil
	}
	if view, ok := a.tabAt(msg.X, msg.Y); ok {
		a.currentView = view
		return a, a.activateCurrentView()
	}

	switch a.currentView {
	case ViewPlaylists:
		if a.playlistsMode == playlistModeList {
			if index, ok := a.libraryItemAt(msg.Y, len(a.playlists), a.playlistSelectedIndex); ok {
				a.playlistSelectedIndex = index
				if a.isDoubleClick(mouseTargetPlaylist, index) {
					return a, a.openSelectedPlaylist()
				}
			}
			return a, nil
		}
		if index, ok := a.libraryItemAt(msg.Y, len(a.playlistTracks), a.playlistTrackIndex); ok {
			a.playlistTrackIndex = index
			if a.isDoubleClick(mouseTargetPlaylistTrack, index) {
				return a, a.playCollectionTrack(a.playlistTracks, index)
			}
		}
	case ViewFavorites:
		if index, ok := a.libraryItemAt(msg.Y, len(a.favoriteTracks), a.favoriteSelectedIndex); ok {
			a.favoriteSelectedIndex = index
			if a.isDoubleClick(mouseTargetFavoriteTrack, index) {
				return a, a.playCollectionTrack(a.favoriteTracks, index)
			}
		}
	}
	return a, nil
}

func (a *App) tabAt(x, y int) (ViewType, bool) {
	if y != lipgloss.Height(a.renderHeader())-3 {
		return ViewSearch, false
	}
	start := 0
	for i, name := range []string{"Search", "Player", "Playlists", "Favorites"} {
		width := lipgloss.Width(styles.InactiveTabStyle.Render(name))
		if x >= start && x < start+width {
			return ViewType(i), true
		}
		start += width
	}
	return ViewSearch, false
}

func (a *App) libraryItemAt(y, length, selected int) (int, bool) {
	start, end := visibleWindow(length, selected, a.libraryVisibleItems())
	firstItemY := lipgloss.Height(a.renderHeader()) + 4
	if a.usesCompactLibraryLayout() {
		firstItemY = lipgloss.Height(a.renderHeader()) + 2
	}
	index := start + y - firstItemY
	if index < start || index >= end {
		return 0, false
	}
	return index, true
}

func (a *App) isDoubleClick(target libraryMouseTarget, index int) bool {
	now := a.now()
	doubleClick := a.lastClick.target == target && a.lastClick.index == index &&
		now.Sub(a.lastClick.at) <= mouseDoubleClickWindow
	a.lastClick = mouseClick{target: target, index: index, at: now}
	if doubleClick {
		a.lastClick = mouseClick{}
	}
	return doubleClick
}

func visibleWindow(length, selected, maxVisible int) (int, int) {
	if length <= 0 {
		return 0, 0
	}
	if maxVisible <= 0 || maxVisible > length {
		maxVisible = length
	}
	selected = clampIndex(selected, length)
	start := selected - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > length {
		end = length
		start = end - maxVisible
	}
	return start, end
}

func rangeIndicator(start, end, total int) string {
	if total <= 0 {
		return "(0)"
	}
	if start == 0 && end == total {
		return fmt.Sprintf("(%d)", total)
	}
	return fmt.Sprintf("(%d-%d of %d)", start+1, end, total)
}

// nextView switches to the next view in the cycle
func (a *App) nextView() {
	switch a.currentView {
	case ViewSearch:
		a.currentView = ViewPlayer
	case ViewPlayer:
		a.currentView = ViewPlaylists
	case ViewPlaylists:
		a.currentView = ViewFavorites
	case ViewFavorites:
		a.currentView = ViewSearch
	}
}

// previousView switches to the previous view in the cycle
func (a *App) previousView() {
	switch a.currentView {
	case ViewSearch:
		a.currentView = ViewFavorites
	case ViewPlayer:
		a.currentView = ViewSearch
	case ViewPlaylists:
		a.currentView = ViewPlayer
	case ViewFavorites:
		a.currentView = ViewPlaylists
	}
}

// Getter methods for testing
func (a *App) GetCurrentView() ViewType {
	return a.currentView
}

func (a *App) SetCurrentView(view ViewType) {
	a.currentView = view
}

func (a *App) IsQuitting() bool {
	return a.quitting
}

func (a *App) GetSize() (int, int) {
	return a.width, a.height
}

func (a *App) GetCurrentTrack() *soundcloud.Track {
	return a.playerComponent.GetCurrentTrack()
}
