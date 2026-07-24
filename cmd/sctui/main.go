package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
	"soundcloud-tui/internal/ui/app"
	"soundcloud-tui/internal/ui/components/player"
)

const testAudioPlaybackStartTimeout = audio.PlaybackStartTimeout

// validateFlagStyle enforces the CLI convention: multi-letter options use a
// double dash (--search); a single dash is reserved for single-letter flags
// (-s). A single-dashed known long option (e.g. -search) is rejected with a
// hint. Args after a literal "--" are positional and not checked.
func validateFlagStyle(args []string, longNames map[string]bool) error {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
			continue
		}
		name := strings.TrimPrefix(a, "-")
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if len(name) > 1 && longNames[name] {
			return fmt.Errorf("use a double dash for multi-letter options: --%s (not -%s)", name, name)
		}
	}
	return nil
}

func main() {
	var (
		searchFlag    string
		trackFlag     string
		playFlag      string
		testAudioFlag string
		testTuiFlag   string
		helpFlag      bool
		whoamiFlag    bool
		playlistsFlag bool
	)
	// Long options use a double dash (--search); single-letter aliases use a
	// single dash (-s) where a sensible letter is free.
	flag.StringVar(&searchFlag, "search", "", "Search for tracks")
	flag.StringVar(&searchFlag, "s", "", "alias for --search")
	flag.StringVar(&trackFlag, "track", "", "Get info for a specific track URL")
	flag.StringVar(&trackFlag, "t", "", "alias for --track")
	flag.StringVar(&playFlag, "play", "", "Play a specific track URL directly")
	flag.StringVar(&playFlag, "p", "", "alias for --play")
	flag.StringVar(&testAudioFlag, "test-audio", "", "Test audio playback without TUI")
	flag.StringVar(&testTuiFlag, "test-tui", "", "Test TUI message flow without interactive mode")
	flag.BoolVar(&whoamiFlag, "whoami", false, "Show the signed-in SoundCloud user")
	flag.BoolVar(&playlistsFlag, "playlists", false, "List your personal playlists")
	flag.BoolVar(&helpFlag, "help", false, "Show help")
	flag.BoolVar(&helpFlag, "h", false, "alias for --help")

	longNames := map[string]bool{
		"search": true, "track": true, "play": true,
		"test-audio": true, "test-tui": true, "help": true,
		"whoami": true, "playlists": true,
	}
	if err := validateFlagStyle(os.Args[1:], longNames); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(2)
	}
	flag.Parse()

	if helpFlag {
		showHelp()
		return
	}

	// Show disclaimer on first run
	showDisclaimer()

	client, err := soundcloud.NewClient()
	if err != nil {
		log.Fatalf("Failed to create SoundCloud client: %v", err)
	}
	printAuthNotice(client)

	if whoamiFlag {
		if err := showWhoami(client); err != nil {
			log.Fatalf("whoami failed: %v", err)
		}
		return
	}

	if playlistsFlag {
		if err := showPlaylists(client); err != nil {
			log.Fatalf("Failed to list playlists: %v", err)
		}
		return
	}

	if searchFlag != "" {
		if err := searchTracks(client, searchFlag); err != nil {
			log.Fatalf("Search failed: %v", err)
		}
		return
	}

	if trackFlag != "" {
		if err := getTrackInfo(client, trackFlag); err != nil {
			log.Fatalf("Failed to get track info: %v", err)
		}
		return
	}

	if playFlag != "" {
		if err := playTrackFromURL(client, playFlag); err != nil {
			log.Fatalf("Failed to play track: %v", err)
		}
		return
	}

	if testAudioFlag != "" {
		if err := testAudioPlayback(client, testAudioFlag); err != nil {
			log.Fatalf("Failed to test audio: %v", err)
		}
		return
	}

	if testTuiFlag != "" {
		if err := testTuiPlayback(client, testTuiFlag); err != nil {
			log.Fatalf("Failed to test TUI: %v", err)
		}
		return
	}

	// Start TUI application
	application := app.NewApp()
	program := tea.NewProgram(application, tea.WithAltScreen(), tea.WithMouseAllMotion())

	if _, err := program.Run(); err != nil {
		log.Fatalf("Failed to start TUI: %v", err)
	}
}

func searchTracks(client *soundcloud.Client, query string) error {
	fmt.Printf("🔍 Searching for: %s\n\n", query)

	tracks, err := client.Search(query)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(tracks) == 0 {
		fmt.Println("No tracks found.")
		return nil
	}

	fmt.Printf("Found %d tracks:\n\n", len(tracks))
	for i, track := range tracks[:min(10, len(tracks))] {
		duration := formatDuration(track.Duration)
		fmt.Printf("%2d. %s\n", i+1, track.Title)
		fmt.Printf("    by %s\n", track.User.FullName())
		fmt.Printf("    Duration: %s | URL: %s\n\n", duration, track.PermalinkURL)
	}

	return nil
}

func getTrackInfo(client *soundcloud.Client, url string) error {
	fmt.Printf("🎵 Getting track info for: %s\n\n", url)

	track, err := client.GetTrackInfo(url)
	if err != nil {
		return fmt.Errorf("failed to get track info: %w", err)
	}

	duration := formatDuration(track.Duration)
	fmt.Printf("Title: %s\n", track.Title)
	fmt.Printf("Artist: %s\n", track.User.FullName())
	fmt.Printf("Duration: %s\n", duration)
	if track.Description != "" {
		fmt.Printf("Description: %s\n", track.Description)
	}
	fmt.Printf("URL: %s\n", track.PermalinkURL)

	return nil
}

func formatDuration(ms int64) string {
	seconds := ms / 1000
	minutes := seconds / 60
	seconds = seconds % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// printAuthNotice reports (to stderr) whether a logged-in browser session was
// found, so the user knows if they're browsing as themselves or anonymously.
func printAuthNotice(c *soundcloud.Client) {
	if c.IsAuthenticated() {
		fmt.Fprintf(os.Stderr, "🔓 Signed in via %s\n\n", c.AuthSource())
	} else {
		fmt.Fprintf(os.Stderr, "🔒 No browser session found — browsing SoundCloud anonymously.\n\n")
	}
}

func showWhoami(c *soundcloud.Client) error {
	me, err := c.Me()
	if err != nil {
		return err
	}
	fmt.Printf("Signed in as:      %s (%s)\n", me.Username, me.FullName)
	fmt.Printf("User ID:           %d\n", me.ID)
	fmt.Printf("Followers:         %d\n", me.FollowersCount)
	fmt.Printf("Private playlists: %d\n", me.PrivatePlaylistsCount)
	return nil
}

func showPlaylists(c *soundcloud.Client) error {
	pls, err := c.Library()
	if err != nil {
		return err
	}
	fmt.Printf("Your library — %d playlists:\n\n", len(pls))
	for _, p := range pls {
		vis := ""
		if p.IsPrivate() {
			vis = " 🔒"
		}
		fmt.Printf("  [%-6s]%s %s — %d tracks\n", p.Kind, vis, p.Title, p.TrackCount)
	}
	return nil
}

func showDisclaimer() {
	// The disclaimer is metadata/warning output → stderr, so structured stdout
	// (e.g. --playlists) stays pipeable.
	w := os.Stderr
	fmt.Fprintln(w, "⚠️  IMPORTANT DISCLAIMER ⚠️")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "This application uses SoundCloud's undocumented internal API")
	fmt.Fprintln(w, "through a reverse-engineered Go library. This may violate")
	fmt.Fprintln(w, "SoundCloud's Terms of Service.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "By using this software, you acknowledge:")
	fmt.Fprintln(w, "• This is for educational/personal use only")
	fmt.Fprintln(w, "• You assume full responsibility for ToS compliance")
	fmt.Fprintln(w, "• The functionality may break if SoundCloud changes their API")
	fmt.Fprintln(w, "• Consider supporting artists through official channels")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Use at your own discretion and risk.")
	fmt.Fprintln(w, "═══════════════════════════════════════════════════════════")
	fmt.Fprintln(w)
}

// validateSoundCloudURL validates and normalizes a SoundCloud URL
func validateSoundCloudURL(url string) error {
	// Remove whitespace and common prefixes
	url = strings.TrimSpace(url)

	// Ensure it's a valid SoundCloud URL
	soundcloudPattern := regexp.MustCompile(`^https?://(www\.|m\.)?soundcloud\.com/[^/]+/[^/?]+`)
	if !soundcloudPattern.MatchString(url) {
		return fmt.Errorf("invalid SoundCloud URL format. Expected: https://soundcloud.com/artist/track")
	}

	return nil
}

// playTrackFromURL plays a track directly from a SoundCloud URL
func playTrackFromURL(client *soundcloud.Client, url string) error {
	fmt.Printf("🎵 Loading track from: %s\n\n", url)

	// Validate URL format
	if err := validateSoundCloudURL(url); err != nil {
		return err
	}

	// Get track information
	track, err := client.GetTrackInfo(url)
	if err != nil {
		return fmt.Errorf("failed to get track info: %w", err)
	}

	fmt.Printf("Now playing: %s by %s\n", track.Title, track.User.FullName())
	fmt.Printf("Duration: %s\n\n", formatDuration(track.Duration))

	// Create audio components with enhanced buffered streaming
	audioPlayer := audio.NewBufferedBeepPlayer()
	defer audioPlayer.Close()

	streamExtractor := audio.NewRealSoundCloudStreamExtractor(client)

	// Create player-only TUI
	playerComponent := player.NewPlayerComponent(audioPlayer, streamExtractor)

	// Create simple app that only shows the player
	playApp := &DirectPlayApp{
		player: playerComponent,
		track:  track,
	}

	// Start the player TUI
	fmt.Printf("Starting TUI player interface...\n")
	program := tea.NewProgram(playApp, tea.WithAltScreen())
	_, err = program.Run()

	return err
}

// DirectPlayApp is a minimal TUI app for direct URL playback
type DirectPlayApp struct {
	player *player.PlayerComponent
	track  *soundcloud.Track
	width  int
	height int
}

func (a *DirectPlayApp) Init() tea.Cmd {
	// Start playing the track immediately
	return tea.Batch(
		a.player.Init(),
		func() tea.Msg {
			return player.PlayTrackMsg{Track: a.track}
		},
	)
}

func (a *DirectPlayApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle quit keys
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			return a, tea.Quit
		}

		// Pass all other keys to player
		updatedPlayer, cmd := a.player.Update(msg)
		a.player = updatedPlayer.(*player.PlayerComponent)
		return a, cmd

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.player.SetSize(msg.Width, msg.Height-2) // Reserve space for title
		return a, nil

	case player.PlaybackStartedMsg:
		// Playback started successfully - just continue
		return a, nil

	case player.PlaybackFailedMsg:
		// Playback failed - show error and quit
		fmt.Printf("\n❌ Playback failed: %v\n", msg.Error)
		return a, tea.Quit

	default:
		// Pass all other messages to player
		updatedPlayer, cmd := a.player.Update(msg)
		a.player = updatedPlayer.(*player.PlayerComponent)
		return a, cmd
	}
}

func (a *DirectPlayApp) View() string {
	// Simple header
	header := fmt.Sprintf("SoundCloud TUI - Direct Play Mode (Press 'q' or Ctrl+C to quit)")

	// Player view
	playerView := a.player.View()

	return fmt.Sprintf("%s\n%s", header, playerView)
}

// testAudioPlayback tests audio playback without TUI interface
func testAudioPlayback(client *soundcloud.Client, url string) error {
	fmt.Printf("🔧 Testing audio playback without TUI for: %s\n\n", url)

	// Validate URL format
	if err := validateSoundCloudURL(url); err != nil {
		return err
	}

	// Get track information
	track, err := client.GetTrackInfo(url)
	if err != nil {
		return fmt.Errorf("failed to get track info: %w", err)
	}

	fmt.Printf("Track: %s by %s\n", track.Title, track.User.FullName())
	fmt.Printf("Duration: %s\n\n", formatDuration(track.Duration))

	// Create audio components with enhanced buffered streaming
	audioPlayer := audio.NewBufferedBeepPlayer()
	defer audioPlayer.Close()

	streamExtractor := audio.NewRealSoundCloudStreamExtractor(client)

	// Extract stream URL
	fmt.Printf("Extracting stream URL...\n")
	streamInfo, err := streamExtractor.ExtractStreamURL(context.Background(), track.ID)
	if err != nil {
		return fmt.Errorf("failed to extract stream URL: %w", err)
	}

	fmt.Printf("Stream URL obtained: %s\n", streamInfo.URL[:50]+"...")

	// Start playback
	fmt.Printf("Starting audio playback...\n")
	ctx, cancel := context.WithTimeout(context.Background(), testAudioPlaybackStartTimeout)
	defer cancel()

	err = audioPlayer.PlayStream(ctx, streamInfo)
	if err != nil {
		return fmt.Errorf("failed to start playback: %w", err)
	}

	fmt.Printf("✅ Playback started successfully!\n")
	fmt.Printf("Playing for 10 seconds to test stability...\n\n")

	// Monitor playback for 10 seconds
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)

		state := audioPlayer.GetState()
		position := audioPlayer.GetPosition()
		duration := audioPlayer.GetDuration()

		fmt.Printf("Second %d: State=%s, Position=%v, Duration=%v\n",
			i+1, state.String(), position.Truncate(time.Millisecond), duration.Truncate(time.Millisecond))

		if state == audio.StateStopped {
			fmt.Printf("❌ Playback stopped unexpectedly at second %d!\n", i+1)
			break
		}
	}

	// Stop playback
	fmt.Printf("\nStopping playback...\n")
	audioPlayer.Stop()

	return nil
}

// testTuiPlayback simulates TUI message flow to test for differences vs direct audio
func testTuiPlayback(client *soundcloud.Client, url string) error {
	fmt.Printf("🔧 Testing TUI message flow for: %s\n\n", url)

	// Validate URL format
	if err := validateSoundCloudURL(url); err != nil {
		return err
	}

	// Get track information
	track, err := client.GetTrackInfo(url)
	if err != nil {
		return fmt.Errorf("failed to get track info: %w", err)
	}

	fmt.Printf("Track: %s by %s\n", track.Title, track.User.FullName())
	fmt.Printf("Duration: %s\n\n", formatDuration(track.Duration))

	// Create audio components (same as TUI)
	audioPlayer := audio.NewBeepPlayer()
	defer audioPlayer.Close()

	streamExtractor := audio.NewRealSoundCloudStreamExtractor(client)
	playerComponent := player.NewPlayerComponent(audioPlayer, streamExtractor)

	// Simulate TUI initialization
	fmt.Printf("Simulating TUI message flow...\n")

	// Step 1: Init player component
	initCmd := playerComponent.Init()
	if initCmd != nil {
		fmt.Printf("Player component initialized\n")
	}

	// Step 2: Send PlayTrackMsg (like TUI does)
	fmt.Printf("Sending PlayTrackMsg...\n")
	playMsg := player.PlayTrackMsg{Track: track}
	updatedPlayer, cmd := playerComponent.Update(playMsg)
	playerComponent = updatedPlayer.(*player.PlayerComponent)

	// Step 3: Execute the command (stream extraction)
	if cmd != nil {
		fmt.Printf("Executing stream extraction command...\n")
		msg := cmd()

		// Step 4: Handle StreamInfoMsg
		if streamMsg, ok := msg.(player.StreamInfoMsg); ok {
			if streamMsg.Error != nil {
				return fmt.Errorf("stream extraction failed: %w", streamMsg.Error)
			}

			fmt.Printf("Stream URL extracted, sending StreamInfoMsg...\n")
			updatedPlayer, playCmd := playerComponent.Update(streamMsg)
			playerComponent = updatedPlayer.(*player.PlayerComponent)

			// Step 5: Execute play command (it's a batch)
			if playCmd != nil {
				fmt.Printf("Executing play command batch...\n")
				playResult := playCmd()
				fmt.Printf("Play command result type: %T\n", playResult)

				// Handle BatchMsg - execute all commands in the batch
				if batchMsg, ok := playResult.(tea.BatchMsg); ok {
					fmt.Printf("Handling batch with %d commands\n", len(batchMsg))
					for i, cmd := range batchMsg {
						fmt.Printf("Executing batch command %d...\n", i+1)
						cmdResult := cmd()
						fmt.Printf("Batch command %d result type: %T\n", i+1, cmdResult)

						// Update player with each result
						updatedPlayer, _ := playerComponent.Update(cmdResult)
						playerComponent = updatedPlayer.(*player.PlayerComponent)
					}
				} else {
					// Handle single command result
					updatedPlayer, progressCmd := playerComponent.Update(playResult)
					playerComponent = updatedPlayer.(*player.PlayerComponent)

					// Execute progress command if available
					if progressCmd != nil {
						fmt.Printf("Executing initial progress command...\n")
						progressResult := progressCmd()
						updatedPlayer, _ = playerComponent.Update(progressResult)
						playerComponent = updatedPlayer.(*player.PlayerComponent)
					}
				}
			}
		}
	}

	fmt.Printf("✅ TUI simulation started!\n")
	fmt.Printf("Waiting 1 second for playback to initialize...\n")
	time.Sleep(1 * time.Second)
	fmt.Printf("Monitoring for 10 seconds to compare with test-audio...\n\n")

	// Monitor playback for 10 seconds (same as test-audio)
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)

		state := audioPlayer.GetState()
		position := audioPlayer.GetPosition()
		duration := audioPlayer.GetDuration()

		fmt.Printf("Second %d: State=%s, Position=%v, Duration=%v\n",
			i+1, state.String(), position.Truncate(time.Millisecond), duration.Truncate(time.Millisecond))

		if state == audio.StateStopped {
			fmt.Printf("❌ Playback stopped unexpectedly at second %d! (TUI simulation)\n", i+1)
			break
		}
	}

	// Stop playback
	fmt.Printf("\nStopping playback...\n")
	audioPlayer.Stop()

	return nil
}

func showHelp() {
	fmt.Printf(`SoundCloud TUI - Terminal User Interface for SoundCloud

Usage:
  %s [flags]

Flags (long options take a double dash; single-letter aliases take one):
  --search, -s "query"   Search for tracks by keyword
  --track,  -t "url"     Get information for a specific track URL
  --play,   -p "url"     Play a specific track URL directly
  --whoami               Show the signed-in SoundCloud user (if a browser session exists)
  --playlists            List your personal playlists (owned, liked, followed)
  --test-audio "url"     Test audio playback without TUI (debug mode)
  --test-tui "url"       Test TUI message flow without interactive mode
  --help,   -h           Show this help message

Auth: if you're logged into SoundCloud in Firefox, the app detects your
session automatically and browses as you (personal/private playlists). No
login step; otherwise it browses anonymously.

Examples:
  %s --search "lofi hip hop"
  %s --track "https://soundcloud.com/artist/track"
  %s -p "https://soundcloud.com/artist/track"
  %s --test-audio "https://soundcloud.com/artist/track"
  %s --test-tui "https://soundcloud.com/artist/track"
  %s                     # Start interactive TUI

Note: This application uses SoundCloud's undocumented API.
See disclaimer above for important legal considerations.
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}
