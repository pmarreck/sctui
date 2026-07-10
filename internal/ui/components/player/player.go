package player

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
	"soundcloud-tui/internal/ui/styles"
)

const (
	StreamExtractionTimeout = audio.StreamExtractionTimeout
	PlaybackStartTimeout    = audio.PlaybackStartTimeout
	LoadingTimeout          = audio.LoadingTimeout
)

// State represents the current state of the player component
type State int

const (
	StateIdle State = iota
	StateLoading
	StatePlaying
	StatePaused
	StateCompleted
	StateError
)

// String returns the string representation of State
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateLoading:
		return "loading"
	case StatePlaying:
		return "playing"
	case StatePaused:
		return "paused"
	case StateCompleted:
		return "completed"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// PlayTrackMsg represents a message to play a track
type PlayTrackMsg struct {
	Track *soundcloud.Track
}

// StreamInfoMsg represents stream info message
type StreamInfoMsg struct {
	StreamInfo *audio.StreamInfo
	Error      error
	playbackID uint64
}

// ProgressUpdateMsg represents progress update message
type ProgressUpdateMsg struct {
	Position   time.Duration
	Duration   time.Duration
	playbackID uint64
}

// PlaybackStartedMsg indicates that playback has successfully started
type PlaybackStartedMsg struct {
	Track *soundcloud.Track
}

// PlaybackFailedMsg indicates that playback failed to start
type PlaybackFailedMsg struct {
	Track *soundcloud.Track
	Error error
}

// PlaybackCompletedMsg reports natural end-of-track playback to the app so it
// can advance the source collection without coupling queue policy to audio I/O.
type PlaybackCompletedMsg struct {
	Track *soundcloud.Track
}

// PlayerComponent represents the player view component
type PlayerComponent struct {
	// Size
	width  int
	height int

	// State
	state                 State
	currentTrack          *soundcloud.Track
	position              time.Duration
	duration              time.Duration
	expectedDuration      time.Duration // Duration from SoundCloud metadata
	volume                float64
	error                 error
	prematureStopDetected bool // Flag to track if we've already detected a premature stop
	playbackID            atomic.Uint64
	collectionNavigation  bool

	// Dependencies
	audioPlayer     audio.Player
	streamExtractor audio.StreamExtractor
}

// NewPlayerComponent creates a new player component
func NewPlayerComponent(audioPlayer audio.Player, streamExtractor audio.StreamExtractor) *PlayerComponent {
	return &PlayerComponent{
		width:           80,
		height:          20,
		state:           StateIdle,
		currentTrack:    nil,
		position:        0,
		duration:        0,
		volume:          1.0,
		error:           nil,
		audioPlayer:     audioPlayer,
		streamExtractor: streamExtractor,
	}
}

// Init initializes the player component
func (p *PlayerComponent) Init() tea.Cmd {
	return p.tickProgress()
}

// Update handles messages and updates the player component
func (p *PlayerComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return p.handleKeyMsg(msg)

	case PlayTrackMsg:
		return p.handlePlayTrack(msg)

	case StreamInfoMsg:
		if !p.isCurrentPlayback(msg.playbackID) {
			return p, nil
		}
		return p.handleStreamInfo(msg)

	case ProgressUpdateMsg:
		if !p.isCurrentPlayback(msg.playbackID) {
			return p, nil
		}
		previousState := p.state
		p.position = msg.Position
		p.duration = msg.Duration

		// If we were loading and got progress, transition to playing
		if p.state == StateLoading {
			p.state = StatePlaying
			// Send playback started message
			return p, tea.Batch(
				p.tickProgress(),
				func() tea.Msg {
					return PlaybackStartedMsg{
						Track: p.currentTrack,
					}
				},
			)
		}

		// Sync state with audio player if available
		if p.audioPlayer != nil {
			p.syncStateWithAudioPlayer()
		}
		if previousState != StateCompleted && p.state == StateCompleted {
			return p, func() tea.Msg {
				return PlaybackCompletedMsg{Track: p.currentTrack}
			}
		}
		return p, p.tickProgress()

	case LoadingTimeoutMsg:
		if !p.isCurrentPlayback(msg.playbackID) {
			return p, nil
		}
		// Handle loading timeout
		if p.state == StateLoading {
			p.state = StateError
			p.error = fmt.Errorf("loading timeout - unable to start playback")
		}
		return p, nil

	case PlaybackErrorMsg:
		if !p.isCurrentPlayback(msg.playbackID) {
			return p, nil
		}
		// Handle playback errors
		p.state = StateError
		p.error = msg.Error
		return p, func() tea.Msg {
			return PlaybackFailedMsg{
				Track: p.currentTrack,
				Error: msg.Error,
			}
		}

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return p, nil

	case error:
		// Handle error messages from commands
		return p.handleError(msg)

	default:
		// No special handling needed - let ProgressUpdateMsg handle updates
	}

	return p, nil
}

// handleKeyMsg handles key messages
func (p *PlayerComponent) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if p.audioPlayer == nil {
		return p, nil
	}

	switch msg.Type {
	case tea.KeySpace:
		return p.togglePlayPause()

	case tea.KeyLeft:
		return p.seekBackward()

	case tea.KeyRight:
		return p.seekForward()

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "+", "=":
			return p.increaseVolume()
		case "-":
			return p.decreaseVolume()
		}
	}

	return p, nil
}

// LoadingTimeoutMsg represents a loading timeout for one playback request.
type LoadingTimeoutMsg struct {
	playbackID uint64
}

// handlePlayTrack handles play track message
func (p *PlayerComponent) handlePlayTrack(msg PlayTrackMsg) (tea.Model, tea.Cmd) {
	playbackID := p.playbackID.Add(1)
	p.currentTrack = msg.Track
	p.state = StateLoading
	p.error = nil
	p.prematureStopDetected = false // Reset flag for new track
	p.position = 0
	p.duration = 0
	p.expectedDuration = 0

	if p.streamExtractor == nil {
		p.state = StateError
		p.error = fmt.Errorf("no stream extractor available")
		return p, nil
	}

	// Start loading with timeout
	return p, tea.Batch(
		p.extractStreamURL(msg.Track, playbackID),
		p.loadingTimeoutCmd(playbackID),
	)
}

// handleStreamInfo handles stream info message
func (p *PlayerComponent) handleStreamInfo(msg StreamInfoMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		p.state = StateError
		p.error = msg.Error
		// Send playback failed message
		return p, func() tea.Msg {
			return PlaybackFailedMsg{
				Track: p.currentTrack,
				Error: msg.Error,
			}
		}
	}

	// Store expected duration from SoundCloud metadata
	if msg.StreamInfo != nil && msg.StreamInfo.Duration > 0 {
		p.expectedDuration = time.Duration(msg.StreamInfo.Duration) * time.Millisecond
	}

	// Stay in loading state until playback actually starts
	return p, p.playStream(msg.StreamInfo, msg.playbackID)
}

// togglePlayPause toggles between play and pause
func (p *PlayerComponent) togglePlayPause() (tea.Model, tea.Cmd) {
	if p.audioPlayer == nil {
		return p, nil
	}

	switch p.audioPlayer.GetState() {
	case audio.StatePlaying:
		return p, func() tea.Msg {
			err := p.audioPlayer.Pause()
			if err != nil {
				return fmt.Errorf("failed to pause: %w", err)
			}
			return ProgressUpdateMsg{
				Position: p.audioPlayer.GetPosition(),
				Duration: p.audioPlayer.GetDuration(),
			}
		}
	case audio.StatePaused:
		// Resume playback without restarting the stream. Reflect the resume in
		// the UI immediately for responsive feedback; an error from Resume()
		// below transitions to the error state.
		p.state = StatePlaying
		return p, func() tea.Msg {
			err := p.audioPlayer.Resume()
			if err != nil {
				return fmt.Errorf("failed to resume: %w", err)
			}
			return ProgressUpdateMsg{
				Position: p.audioPlayer.GetPosition(),
				Duration: p.audioPlayer.GetDuration(),
			}
		}
	case audio.StateStopped:
		// Check if we've actually completed the track or if it's a temporary stop
		if p.currentTrack != nil {
			expectedDuration := time.Duration(p.currentTrack.Duration) * time.Millisecond
			// Only restart from beginning if we're near the end or if duration is unknown
			if p.position >= expectedDuration-2*time.Second || expectedDuration == 0 {
				// Track completed - replay from beginning using normal flow with timeout
				p.state = StateLoading
				p.error = nil
				p.prematureStopDetected = false
				return p, tea.Batch(
					p.extractStreamURL(p.currentTrack, p.playbackID.Load()),
					p.loadingTimeoutCmd(p.playbackID.Load()),
				)
			} else {
				// Premature stop - restart using normal flow (simpler and more reliable)
				p.state = StateLoading
				p.error = nil
				p.prematureStopDetected = false
				return p, tea.Batch(
					p.extractStreamURL(p.currentTrack, p.playbackID.Load()),
					p.loadingTimeoutCmd(p.playbackID.Load()),
				)
			}
		}
		return p, nil
	default:
		return p, nil
	}
}

// seekBackward seeks backward by 10 seconds
func (p *PlayerComponent) seekBackward() (tea.Model, tea.Cmd) {
	if p.audioPlayer == nil {
		return p, nil
	}

	newPos := p.position - 10*time.Second
	if newPos < 0 {
		newPos = 0
	}
	// Update immediately so rapid repeated keypresses calculate from the most
	// recently requested position instead of waiting for async seek completion.
	p.position = newPos

	return p, func() tea.Msg {
		err := p.audioPlayer.Seek(newPos)
		if err != nil {
			return fmt.Errorf("failed to seek: %w", err)
		}
		return ProgressUpdateMsg{
			Position: p.audioPlayer.GetPosition(),
			Duration: p.audioPlayer.GetDuration(),
		}
	}
}

// seekForward seeks forward by 10 seconds
func (p *PlayerComponent) seekForward() (tea.Model, tea.Cmd) {
	if p.audioPlayer == nil {
		return p, nil
	}

	duration := p.duration
	if duration <= 0 {
		duration = p.audioPlayer.GetDuration()
		p.duration = duration
	}
	if duration <= 0 {
		return p, func() tea.Msg {
			return ProgressUpdateMsg{
				Position: p.audioPlayer.GetPosition(),
				Duration: p.audioPlayer.GetDuration(),
			}
		}
	}

	newPos := p.position + 10*time.Second
	if newPos > duration {
		newPos = duration
	}
	// Update immediately so rapid repeated keypresses calculate from the most
	// recently requested position instead of waiting for async seek completion.
	p.position = newPos

	return p, func() tea.Msg {
		err := p.audioPlayer.Seek(newPos)
		if err != nil {
			return fmt.Errorf("failed to seek: %w", err)
		}
		return ProgressUpdateMsg{
			Position: p.audioPlayer.GetPosition(),
			Duration: p.audioPlayer.GetDuration(),
		}
	}
}

// increaseVolume increases volume by 10%
func (p *PlayerComponent) increaseVolume() (tea.Model, tea.Cmd) {
	if p.audioPlayer == nil {
		// Even without audio player, update local volume for UI feedback
		p.volume = p.volume + 0.1
		if p.volume > 1.0 {
			p.volume = 1.0
		}
		return p, nil
	}

	newVolume := p.volume + 0.1
	if newVolume > 1.0 {
		newVolume = 1.0
	}

	return p, func() tea.Msg {
		err := p.audioPlayer.SetVolume(newVolume)
		if err != nil {
			return fmt.Errorf("failed to set volume: %w", err)
		}
		// Update local volume tracking
		p.volume = p.audioPlayer.GetVolume()
		return ProgressUpdateMsg{
			Position: p.audioPlayer.GetPosition(),
			Duration: p.audioPlayer.GetDuration(),
		}
	}
}

// decreaseVolume decreases volume by 10%
func (p *PlayerComponent) decreaseVolume() (tea.Model, tea.Cmd) {
	if p.audioPlayer == nil {
		// Even without audio player, update local volume for UI feedback
		p.volume = p.volume - 0.1
		if p.volume < 0.0 {
			p.volume = 0.0
		}
		return p, nil
	}

	newVolume := p.volume - 0.1
	if newVolume < 0.0 {
		newVolume = 0.0
	}

	return p, func() tea.Msg {
		err := p.audioPlayer.SetVolume(newVolume)
		if err != nil {
			return fmt.Errorf("failed to set volume: %w", err)
		}
		// Update local volume tracking
		p.volume = p.audioPlayer.GetVolume()
		return ProgressUpdateMsg{
			Position: p.audioPlayer.GetPosition(),
			Duration: p.audioPlayer.GetDuration(),
		}
	}
}

// extractStreamURL stops any active audio, then extracts the stream URL for the
// requested track with private playlist context when the extractor supports it.
func (p *PlayerComponent) extractStreamURL(track *soundcloud.Track, playbackID uint64) tea.Cmd {
	return func() tea.Msg {
		if !p.isCurrentPlayback(playbackID) {
			return nil
		}
		if track == nil {
			return StreamInfoMsg{Error: fmt.Errorf("no track selected"), playbackID: playbackID}
		}

		if p.audioPlayer != nil {
			switch p.audioPlayer.GetState() {
			case audio.StatePlaying, audio.StatePaused:
				if err := p.audioPlayer.Stop(); err != nil {
					return StreamInfoMsg{Error: fmt.Errorf("failed to stop current playback: %w", err), playbackID: playbackID}
				}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), StreamExtractionTimeout)
		defer cancel()

		req := audio.TrackStreamRequest{
			TrackID:             track.ID,
			PermalinkURL:        track.PermalinkURL,
			PlaylistID:          track.PlaylistID,
			PlaylistSecretToken: track.PlaylistSecretToken,
			SecretToken:         track.SecretToken,
		}
		var streamInfo *audio.StreamInfo
		var err error
		if extractor, ok := p.streamExtractor.(audio.TrackContextStreamExtractor); ok {
			streamInfo, err = extractor.ExtractTrackStreamURL(ctx, req)
		} else {
			streamInfo, err = p.streamExtractor.ExtractStreamURL(ctx, track.ID)
		}
		if !p.isCurrentPlayback(playbackID) {
			return nil
		}
		return StreamInfoMsg{
			StreamInfo: streamInfo,
			Error:      err,
			playbackID: playbackID,
		}
	}
}

// PlaybackErrorMsg represents a playback error
type PlaybackErrorMsg struct {
	Error      error
	playbackID uint64
}

// playStream starts playing a stream
func (p *PlayerComponent) playStream(streamInfo *audio.StreamInfo, playbackID uint64) tea.Cmd {
	return func() tea.Msg {
		if !p.isCurrentPlayback(playbackID) {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), PlaybackStartTimeout)
		defer cancel()

		err := p.audioPlayer.PlayStream(ctx, streamInfo)
		if err != nil {
			return PlaybackErrorMsg{
				Error:      fmt.Errorf("failed to play stream: %w", err),
				playbackID: playbackID,
			}
		}

		return ProgressUpdateMsg{
			Position:   p.audioPlayer.GetPosition(),
			Duration:   p.audioPlayer.GetDuration(),
			playbackID: playbackID,
		}
	}
}

func (p *PlayerComponent) isCurrentPlayback(playbackID uint64) bool {
	return playbackID == 0 || playbackID == p.playbackID.Load()
}

// tickProgress returns a command that sends progress updates
func (p *PlayerComponent) tickProgress() tea.Cmd {
	// Use shorter interval for smoother progress updates
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		if p.audioPlayer != nil && (p.state == StatePlaying || p.state == StatePaused) {
			return ProgressUpdateMsg{
				Position: p.audioPlayer.GetPosition(),
				Duration: p.audioPlayer.GetDuration(),
			}
		}
		return nil
	})
}

// syncStateWithAudioPlayer synchronizes the UI state with the audio player state
func (p *PlayerComponent) syncStateWithAudioPlayer() {
	if p.audioPlayer == nil {
		return
	}

	audioState := p.audioPlayer.GetState()
	switch audioState {
	case audio.StatePlaying:
		if p.state != StatePlaying && p.state != StateLoading {
			p.state = StatePlaying
			p.prematureStopDetected = false // Reset flag when playback starts
		}
	case audio.StatePaused:
		if p.state == StatePlaying {
			p.state = StatePaused
		}
	case audio.StateStopped:
		if p.state == StatePlaying || p.state == StatePaused {
			// Only mark as completed if we're near the end of the track
			// Otherwise it might be a temporary stop due to buffering/network issues
			if p.currentTrack != nil {
				// Check if position is near the end (within last 2 seconds)
				expectedDuration := time.Duration(p.currentTrack.Duration) * time.Millisecond
				if p.position >= expectedDuration-2*time.Second || expectedDuration == 0 {
					p.state = StateCompleted
				} else {
					// Premature stop detected - only log once
					if !p.prematureStopDetected {
						// Premature stop detected
						p.prematureStopDetected = true
					}
					// Don't change state immediately - wait for user input
				}
			} else {
				// No track means we should be idle
				p.state = StateIdle
			}
		}
	}

	// Update volume to stay in sync
	p.volume = p.audioPlayer.GetVolume()
}

// handleError handles error messages and transitions to error state
func (p *PlayerComponent) handleError(err error) (tea.Model, tea.Cmd) {
	p.state = StateError
	p.error = err
	// Send playback failed message if we have a current track
	if p.currentTrack != nil {
		return p, func() tea.Msg {
			return PlaybackFailedMsg{
				Track: p.currentTrack,
				Error: err,
			}
		}
	}
	return p, nil
}

// View renders the player component
func (p *PlayerComponent) View() string {
	switch p.state {
	case StateIdle:
		return p.renderIdleView()
	case StateLoading:
		return p.renderLoadingView()
	case StatePlaying, StatePaused:
		return p.renderPlayingView()
	case StateCompleted:
		return p.renderCompletedView()
	case StateError:
		return p.renderErrorView()
	default:
		return "Unknown player state"
	}
}

// renderIdleView renders the idle view
func (p *PlayerComponent) renderIdleView() string {
	if p.usesCompactLayout() {
		return styles.StatusStyle.Render("No track loaded")
	}
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		styles.StatusStyle.Render("🎵 No track loaded"),
		"",
		styles.HelpStyle.Render("Select a track from the search to start playing"),
	)

	return styles.PlayerStyle.Width(p.width - 4).Height(p.height - 4).Render(
		lipgloss.Place(p.width-8, p.height-8, lipgloss.Center, lipgloss.Center, content),
	)
}

// renderLoadingView renders the loading view
func (p *PlayerComponent) renderLoadingView() string {
	if p.currentTrack == nil {
		return p.renderIdleView()
	}
	if p.usesCompactLayout() {
		return p.renderCompactTrackView("Loading")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		styles.TrackTitleStyle.Render(p.currentTrack.Title),
		styles.TrackArtistStyle.Render(p.currentTrack.Artist()),
		"",
		styles.LoadingStatusStyle.Render("🔄 Loading..."),
	)

	return styles.PlayerStyle.Width(p.width - 4).Height(p.height - 4).Render(
		lipgloss.Place(p.width-8, p.height-8, lipgloss.Center, lipgloss.Center, content),
	)
}

// renderPlayingView renders the playing/paused view
func (p *PlayerComponent) renderPlayingView() string {
	if p.currentTrack == nil {
		return p.renderIdleView()
	}
	if p.usesCompactLayout() {
		status := "Playing"
		if p.state == StatePaused {
			status = "Paused"
		}
		return p.renderCompactTrackView(status)
	}

	// Track info with enhanced metadata display
	metadata := styles.RenderMetadataPanel(
		p.currentTrack.Title,
		p.currentTrack.Artist(),
		p.width-8, // Account for player panel padding
	)

	// Status
	var status string
	if p.audioPlayer != nil {
		switch p.audioPlayer.GetState() {
		case audio.StatePlaying:
			status = styles.PlayingStatusStyle.Render("▶ Playing")
		case audio.StatePaused:
			status = styles.PausedStatusStyle.Render("⏸ Paused")
		default:
			status = styles.StatusStyle.Render("⏹ Stopped")
		}
	} else {
		status = styles.StatusStyle.Render("⏹ Stopped")
	}

	// Progress bar
	var progressBar string
	var timeInfo string

	// Progress reflects the component's cached position/duration, which the
	// Bubble Tea update loop keeps in sync via ProgressUpdateMsg.
	displayDuration := p.duration
	if displayDuration <= 0 && p.expectedDuration > 0 {
		displayDuration = p.expectedDuration
	}

	if displayDuration > 0 {
		progress := float64(p.position) / float64(displayDuration)
		progressBar = styles.RenderProgressBar(p.width-12, progress)

		posStr := styles.FormatDurationFromTime(p.position)
		durStr := styles.FormatDurationFromTime(displayDuration)
		timeInfo = fmt.Sprintf("%s / %s", posStr, durStr)
	} else {
		progressBar = styles.RenderProgressBar(p.width-12, 0)
		timeInfo = styles.FormatDurationFromTime(0) + " / " + styles.FormatDurationFromTime(0)
	}

	// Volume reflects the live player volume when available.
	displayVolume := p.volume
	if p.audioPlayer != nil {
		displayVolume = p.audioPlayer.GetVolume()
	}
	volumePercent := int(displayVolume * 100)
	var volumeIcon string
	switch {
	case displayVolume == 0:
		volumeIcon = "🔇" // Muted
	case displayVolume < 0.5:
		volumeIcon = "🔉" // Low volume
	default:
		volumeIcon = "🔊" // High volume
	}
	volumeInfo := fmt.Sprintf("%s %d%%", volumeIcon, volumePercent)

	// Controls help
	arrowControls := "←→: Seek"
	if p.collectionNavigation {
		arrowControls += " • Shift+←→: Previous/Next Track"
	}
	controls := styles.HelpStyle.Render("Space: Play/Pause • " + arrowControls + " • +/-: Volume")

	// Combine everything
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		metadata,
		"",
		status,
		"",
		progressBar,
		styles.StatusStyle.Render(timeInfo),
		"",
		styles.StatusStyle.Render(volumeInfo),
		"",
		controls,
	)

	return styles.PlayerStyle.Width(p.width - 4).Render(content)
}

// renderCompletedView renders the completed view
func (p *PlayerComponent) renderCompletedView() string {
	if p.currentTrack == nil {
		return p.renderIdleView()
	}
	if p.usesCompactLayout() {
		return p.renderCompactTrackView("Completed")
	}

	// Track info with enhanced metadata display
	metadata := styles.RenderMetadataPanel(
		p.currentTrack.Title,
		p.currentTrack.Artist(),
		p.width-8, // Account for player panel padding
	)

	// Status
	status := styles.StatusStyle.Render("✅ Track Completed")

	// Progress bar (show as full)
	var progressBar string
	var timeInfo string

	// Use actual duration from audio player, fallback to expected duration from metadata
	displayDuration := p.duration
	if displayDuration <= 0 && p.expectedDuration > 0 {
		displayDuration = p.expectedDuration
	}

	if displayDuration > 0 {
		progressBar = styles.RenderProgressBar(p.width-12, 1.0) // 100% complete

		durStr := styles.FormatDurationFromTime(displayDuration)
		timeInfo = fmt.Sprintf("%s / %s", durStr, durStr)
	} else {
		progressBar = styles.RenderProgressBar(p.width-12, 1.0)
		timeInfo = "Completed"
	}

	// Volume info
	volumePercent := int(p.volume * 100)
	var volumeIcon string
	switch {
	case p.volume == 0:
		volumeIcon = "🔇" // Muted
	case p.volume < 0.5:
		volumeIcon = "🔉" // Low volume
	default:
		volumeIcon = "🔊" // High volume
	}
	volumeInfo := fmt.Sprintf("%s %d%%", volumeIcon, volumePercent)

	// Controls help
	controls := styles.HelpStyle.Render("Space: Replay • Search for another track")

	// Combine everything
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		metadata,
		"",
		status,
		"",
		progressBar,
		styles.StatusStyle.Render(timeInfo),
		"",
		styles.StatusStyle.Render(volumeInfo),
		"",
		controls,
	)

	return styles.PlayerStyle.Width(p.width - 4).Render(content)
}

// renderErrorView renders the error view
func (p *PlayerComponent) renderErrorView() string {
	if p.usesCompactLayout() {
		if p.currentTrack == nil {
			return styles.ErrorStatusStyle.Render("Playback error")
		}
		return p.renderCompactTrackView("Playback error")
	}
	var trackInfo string
	if p.currentTrack != nil {
		trackInfo = fmt.Sprintf("Track: %s - %s", p.currentTrack.Title, p.currentTrack.Artist())
	} else {
		trackInfo = "Unknown track"
	}

	errMsg := "Unknown error"
	if p.error != nil {
		errMsg = p.error.Error()
	}

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		styles.ErrorStatusStyle.Render("❌ Playback Error"),
		"",
		styles.StatusStyle.Render(trackInfo),
		"",
		styles.ErrorStatusStyle.Render(errMsg),
		"",
		styles.HelpStyle.Render("Try selecting another track"),
	)

	return styles.PlayerStyle.Width(p.width - 4).Height(p.height - 4).Render(
		lipgloss.Place(p.width-8, p.height-8, lipgloss.Center, lipgloss.Center, content),
	)
}

func (p *PlayerComponent) usesCompactLayout() bool {
	// App sizes the component to the available content area plus one row.
	return p.height <= 8
}

func (p *PlayerComponent) renderCompactTrackView(status string) string {
	if p.currentTrack == nil {
		return styles.StatusStyle.Render(status)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		styles.StatusStyle.Render(status),
		styles.TrackTitleStyle.Render(p.currentTrack.Title+" - "+p.currentTrack.Artist()),
	)
}

// formatDuration formats a duration to MM:SS format
func formatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// Getter and setter methods for testing and integration
func (p *PlayerComponent) GetCurrentTrack() *soundcloud.Track {
	return p.currentTrack
}

func (p *PlayerComponent) SetCurrentTrack(track *soundcloud.Track) {
	p.currentTrack = track
}

func (p *PlayerComponent) GetState() State {
	return p.state
}

func (p *PlayerComponent) SetState(state State) {
	p.state = state
}

func (p *PlayerComponent) GetVolume() float64 {
	if p.audioPlayer != nil {
		p.volume = p.audioPlayer.GetVolume()
	}
	return p.volume
}

func (p *PlayerComponent) GetPosition() time.Duration {
	return p.position
}

func (p *PlayerComponent) GetDuration() time.Duration {
	return p.duration
}

func (p *PlayerComponent) GetError() error {
	return p.error
}

func (p *PlayerComponent) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetCollectionNavigation updates the controls hint; playlist/favorites track
// selection itself remains app-owned so the player stays a pure audio adapter.
func (p *PlayerComponent) SetCollectionNavigation(enabled bool) {
	p.collectionNavigation = enabled
}

// loadingTimeoutCmd returns a command that sends a timeout message after delay
func (p *PlayerComponent) loadingTimeoutCmd(playbackID uint64) tea.Cmd {
	return tea.Tick(LoadingTimeout, func(t time.Time) tea.Msg {
		return LoadingTimeoutMsg{playbackID: playbackID}
	})
}
