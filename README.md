# SoundCloud TUI

[![Mechatron Prime CI](https://img.shields.io/endpoint?url=https%3A%2F%2Fthelio-nixos.tail66c90.ts.net%2Fbadges%2Fsctui.json&style=for-the-badge)](https://thelio-nixos.tail66c90.ts.net/mechatron-prime/)

A Terminal User Interface for SoundCloud written in Go, featuring real audio playback and interactive controls.

Search, play, and control SoundCloud tracks directly from your terminal.

## What This Fork Adds

This fork turns the original anonymous search-and-play TUI into a more capable
personal SoundCloud client with reproducible builds and resilient playback.

- **Personal-account library**: silently reuses a logged-in Firefox SoundCloud
  session, exposes identity and playlist CLI commands, and adds in-TUI
  Playlists and Favorites tabs. Playlist hydration handles large collections.
- **Library workflow**: keyboard and mouse selection, double-click open/play,
  wheel navigation, fixed TUI chrome for long lists, and `F5` refreshes without
  leaving the current view.
- **Modern stream playback**: direct transcoding resolution, signed private
  streams, ffmpeg AAC/HLS decoding into seekable PCM, and a buffered fallback
  for progressive media. DRM-only streams fail with a useful explanation.
- **Collection behavior**: Shift+Left/Right track navigation, automatic
  end-of-track advance, and visible retry/skip feedback when a collection item
  cannot play.
- **Interaction refinements**: searches accept spaces, Ctrl+Q quits, playback
  changes the terminal title, regular arrows retain 10-second seeking, and
  CLI long flags use GNU-style double dashes with single-letter aliases.
- **Reliability fixes**: interrupted playback is stopped before resolving the
  next track; short completed downloads preload correctly; seeking no longer
  self-deadlocks or resets playback position.
- **Reproducible delivery**: Nix supplies the build, test, ffmpeg, sqlite3,
  and audio dependencies; Mechatron Prime builds the exact pushed commit.

## ⚠️ Important Disclaimer

This application uses SoundCloud's undocumented internal API through a reverse-engineered Go library. This may violate SoundCloud's Terms of Service.

**By using this software, you acknowledge:**
- This is for educational/personal use only
- You assume full responsibility for ToS compliance  
- The functionality may break if SoundCloud changes their API
- Consider supporting artists through official channels

**Use at your own discretion and risk.**

## Features

✅ **Fully Implemented:**
- **Interactive TUI** with Bubble Tea framework
- **Real audio playback** using Beep plus ffmpeg-backed HLS decoding
- **Search and browse** SoundCloud tracks
- **Player controls** (play/pause, seek, volume)
- **Silent Firefox session login**: detects an existing SoundCloud browser session without a separate login flow
- **Personal library**: loads owned, liked, followed, and private playlists plus favorite tracks
- **Playlist navigation**: enter a playlist with Right/Enter or a double-click; return with Left/Esc
- **F5 library refresh**: refetches playlists and Favorites and refreshes the opened playlist's tracks when loaded
- **Collection playback**: Shift+Left/Right moves through a playlist or Favorites; completion advances automatically
- **Visible collection recovery**: highlights skipped tracks' successors and shows why an unavailable track was skipped
- **Mouse support**: click tabs and items, double-click to open/play, and use the wheel to move library selections
- **Terminal playback title**: the terminal title becomes `🔊 SoundCloud TUI` while audio is playing
- **Robust stream selection**: resolves current SoundCloud transcodings, private signed streams, and reports DRM-only media clearly
- **Progress tracking** with smooth progress bars
- **Global hotkeys** (Space, ←→, +/-) work from any view
- **Track completion** handling with replay functionality
- **CLI mode** for search, track info, playback, identity, and playlist listing
- **Reproducible Nix builds and tests** with Mechatron Prime exact-commit CI

🎵 **TUI Navigation:**
- **Tab/Shift+Tab**: Switch between Search/Player/Playlists/Favorites views
- **Search View**: Enter to search, ↑↓ to navigate, Enter to select
- **Player View**: Space (play/pause), ←→ (seek 10s), Shift+←→ (previous/next track for playlists and Favorites), +/- (volume)
- **Playlists View**: ↑↓ to navigate, →/Enter to open a playlist or play a track, ←/Esc to go back
- **Favorites View**: ↑↓ to navigate liked tracks, Enter to play
- **F5**: Refresh personal playlists, Favorites, and an opened playlist's tracks
- **Global Controls**: Audio controls work from any view

🚧 **Coming Soon:**
- Explicit queue management
- Enhanced metadata display

## Installation

### Prerequisites
- [Nix](https://nixos.org/download/) with flakes enabled
- ffmpeg (required for current SoundCloud AAC/HLS streams; Nix builds wrap it automatically)
- Audio system (ALSA/PulseAudio on Linux or Core Audio on macOS)

The packaged Nix build currently declares Unix platforms. Native Windows
packaging is not part of this fork's supported release path yet.

### Build from Source

```bash
# Clone the repository
git clone https://github.com/pmarreck/sctui.git
cd sctui

# Build the reproducible, wrapped binary
./build

# Run the complete hermetic test suite
./test
```

`./build` writes the runnable binary to `bin/sctui`. It includes `ffmpeg` and
`sqlite3` in its runtime PATH for HLS decoding and browser-session discovery.

## Usage

### Interactive TUI Mode (Default)
```bash
./bin/sctui
```

Launches the full interactive Terminal UI with audio playback capabilities.

### CLI Mode Examples
```bash
# Search for tracks
./bin/sctui --search "lofi hip hop"

# Get track information
./bin/sctui --track "https://soundcloud.com/artist/track"

# Play a track directly
./bin/sctui --play "https://soundcloud.com/artist/track"

# Inspect the detected SoundCloud session and list your playlists
./bin/sctui --whoami
./bin/sctui --playlists

# Show help
./bin/sctui --help
```

### TUI Controls
- **Tab/Shift+Tab**: Navigate between views
- **Search View**: 
  - Type to search, Enter to execute
  - ↑↓ to navigate results, Enter to play
- **Playlists View**:
  - ↑↓ to navigate playlists or playlist tracks
  - →/Enter to open a playlist or play the selected track
  - ←/Esc to return from playlist tracks to the playlist list
- **Favorites View**:
  - ↑↓ to navigate liked tracks, Enter to play
- **F5**: Refresh playlists, Favorites, and the currently opened playlist
- **Global Audio Controls** (work from any view):
  - **Space**: Play/Pause
  - **←→**: Seek backward/forward (10 seconds)
  - **Shift+←→**: Move to the previous/next track while playing a playlist or Favorites collection
  - **+/-**: Volume up/down
- **Mouse**:
  - Click a tab to select it
  - Click a playlist or track to select it
  - Double-click a playlist to open it, or a track to play it
  - Scroll to move the active playlist or track selection
- **Ctrl+C/Q**: Quit application

## Development

### Build and Test Commands

```bash
./build          # Reproducible Nix package build; writes bin/sctui
./test           # Full hermetic Nix test suite, matching Mechatron CI
nix run          # Build and launch the TUI without staging bin/sctui
nix develop      # Enter a Go, ffmpeg, sqlite3, and audio development shell
```

### Project Structure

```
cmd/
├── sctui/          # Main TUI application entry point
└── test/           # Test utilities
internal/
├── audio/          # Audio playback and streaming (Beep integration)
├── soundcloud/     # SoundCloud API client wrapper
├── ui/             # TUI components (Bubble Tea)
│   ├── app/        # Main application model
│   ├── components/ # Player, Search, UI components
│   └── styles/     # Centralized styling
├── api/            # Legacy OAuth code (unused)
└── config/         # Configuration management
tests/
└── unit/           # Audio and TUI unit tests
.mechatron-prime/   # Exact-commit Nix targets for Mechatron Prime CI
notes/              # Planning and documentation
bin/                # Build artifacts (gitignored)
```

### Testing

We follow Test-Driven Development (TDD) principles:

```bash
# Run the same full test derivation used by CI
./test

# Work inside the project development shell when invoking Go directly
nix develop -c go test -cover ./...
```

## Technical Architecture

### Audio Implementation
- **Beep Library**: High-performance audio playback with seekable streamers
- **AAC/HLS Decoding**: Current SoundCloud HLS streams decode through ffmpeg into in-memory stereo PCM
- **Progressive Fallback**: Older MP3/WAV URLs still use the direct HTTP decoder path
- **Buffered Seeking**: Fully decoded HLS PCM permits stable seeks and collection track changes
- **DRM Detection**: Major-label SoundCloud+ tracks that only expose FairPlay/Widevine/PlayReady HLS are reported as unsupported instead of surfacing a misleading media 404
- **Real-time Position Tracking**: 250ms update intervals for smooth progress
- **Thread-safe Player**: Concurrent-safe with proper mutex locking

### TUI Framework
- **Bubble Tea**: Modern terminal UI framework with message passing
- **Component Architecture**: Modular player, search, and navigation components
- **Global State Management**: Centralized app state with component communication
- **Responsive Design**: Adapts to terminal size changes
- **Library Views**: Bounded list rendering keeps headers and controls visible for long playlists and Favorites

### SoundCloud Integration  
- **Reverse-engineered API**: Uses `github.com/zackradisic/soundcloud-api`
- **Browser Session Detection**: Reads an existing Firefox SoundCloud session when available, otherwise browses anonymously
- **Real Stream URLs**: Prefers HLS CDN URLs, with progressive fallback
- **CloudFront Authentication**: Handles signed URL parameters

## Roadmap

**Phase 1: Core TUI** ✅ 
- Interactive TUI with Bubble Tea
- Search and navigation
- Player controls and state management

**Phase 2: Real Audio** ✅
- Beep library integration
- HTTP audio streaming  
- Position/duration tracking
- Volume and seeking controls

**Phase 3: Enhanced Experience** 🚧
- Personal playlists and Favorites ✅
- Collection playback and automatic recovery ✅
- Explicit queue management
- Advanced metadata display
- Further playback diagnostics

## Contributing

This is an educational project demonstrating TUI development and audio programming in Go. Contributions welcome for:

- **Bug fixes and improvements**: Help make the player more robust
- **Test coverage**: Expand unit and integration test coverage  
- **Documentation**: Improve guides and API documentation
- **Performance optimizations**: Audio streaming and UI responsiveness improvements
- **New features**: Explicit queue management, enhanced metadata, and a tested
  Windows packaging path

### Development Guidelines
- Follow TDD principles - write tests first
- Use `./build` and `./test` for the reproducible build and full test suite
- Update CLAUDE.md for any new commands or workflows
- Ensure changes work on the supported Unix package platforms; add a tested
  release path before claiming another platform
- Include appropriate error handling and user feedback

### Getting Started
1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Write tests for your changes
4. Implement your feature
5. Ensure all tests pass: `./test`
6. Submit a pull request

Please ensure all changes include appropriate tests and documentation.

## Troubleshooting

### Audio Issues
- **Linux**: Ensure ALSA or PulseAudio is installed and running
  ```bash
  # Check audio system
  aplay -l  # List audio devices
  pulseaudio --check  # Check PulseAudio status
  ```
- **macOS**: Should work out of the box with Core Audio
- **Windows**: Native packaging is not currently supported; use a Unix host or
  contribute a tested Windows release path.

### Build Issues
- **Nix daemon unavailable**: Start `nix-daemon.service` and rerun `./build` or `./test`
- **First build downloads dependencies**: Nix fetches its pinned inputs and Go module tree before the build can run offline
- **Direct Go development**: Use `nix develop` so ffmpeg, sqlite3, and audio dependencies are available

### Runtime Issues
- **TUI not displaying**: Ensure terminal supports 256 colors
- **Track not playing**: Check internet connection and SoundCloud availability
- **SoundCloud+ catalog track not playing**: If SoundCloud only advertises DRM-encrypted `cbc-encrypted-hls` / `ctr-encrypted-hls` streams, sctui will report that explicitly. Use `SCTUI_LIVE_DEBUG=1 SCTUI_LIVE_DEBUG_TRACK_URL="<url>" nix develop -c go test ./internal/soundcloud -run TestLiveDebugTrackTranscodings -v` to inspect redacted live transcoding metadata.
- **Controls not responding**: Try different terminal emulator or update to latest version

For more help, check the [troubleshooting guide](notes/troubleshooting.md) or open an issue.

## Legal

This project is for educational purposes only. Users are responsible for compliance with SoundCloud's Terms of Service. The developers assume no liability for misuse of this software.

## License

[License TBD]
