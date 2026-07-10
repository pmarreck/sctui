# Implementation Plan for Open-Source SoundCloud TUI Client in Go

## Current Work: Collection Playback and Reliable Skipping (2026-07-10 EDT)

Goal: make repeated forward skips reliable, continue through the active
playlist or Favorites collection after a track completes or cannot be played,
and make the library usable with mouse or long collections.

Done criteria:
- [x] Repeated forward skips advance through the active collection without
  reusing a stale playback position or stream request.
- [x] Playlist/Favorites playback retains an ordered collection context and
  advances automatically when a track completes.
- [x] Playback failures, including unavailable/DRM tracks, automatically try
  later tracks in that same collection until one starts or it is exhausted.
- [x] `./test` and `./build` pass before commit.
  Completed 2026-07-10 09:28 EDT.
- [x] Mouse clicks select tabs and tracks; double-clicks open playlists or play
  tracks through the same collection playback path as keyboard input.
- [x] Long playlist, Favorites, Search, and Player panels remain below the fixed header and
  above the footer at the current terminal height.

Next small behaviors:
- [x] Collection context: a selected playlist/favorite track remembers its
  source ordering after navigating away from the library view.
  Curiosity poke: can returning from a playlist list erase the context of a
  currently playing private playlist?
- [x] Skip behavior: repeated right-arrow presses target successive tracks and
  stale asynchronous stream results cannot overwrite the newest choice.
  Curiosity poke: can a slow first stream-resolution command begin playback
  after two faster skips?
- [x] Auto-advance behavior: completion and playback failures select the next
  collection member, preserving the final failure when no playable track remains.
  Curiosity poke: can an old completion message advance a newly selected track?
- [x] Mouse behavior: single clicks update selections and double-clicks perform
  their keyboard-equivalent action.
  Curiosity poke: can a double-click accidentally play the adjacent row after a
  bounded list window shifts?
- [x] Layout behavior: library/player/search panel height fits the terminal after accounting
  for header/footer chrome.
  Curiosity poke: can a very short terminal leave an unusable negative height?

## Current Work: SoundCloud+ DRM Stream Detection (2026-07-08 EDT)

Goal: stop reporting misleading transcoding 404s for major-label SoundCloud+
tracks that only expose DRM-protected HLS streams.

Done criteria:
- [x] Live debug metadata for a SoundCloud track is available behind
  `SCTUI_LIVE_DEBUG=1`, with signed/media URLs redacted.
  Completed 2026-07-08 17:53 EDT.
- [x] Plain HLS/progressive candidates that 404 are tried in order instead of
  failing on the first missing transcoding.
  Completed 2026-07-08 17:53 EDT.
- [x] If only DRM-encrypted cbc/ctr HLS variants are available, the player reports
  an explicit unsupported encrypted SoundCloud+ stream error.
  Completed 2026-07-08 17:53 EDT.
- [x] `./test` and `./build` pass before commit.
  Completed 2026-07-08 17:57 EDT.

Next small behaviors:
- [x] DRM catalog behavior: reproduce the New Lands shape with a fixture where
  DRM transcodings resolve but plain HLS/progressive return 404.
  Curiosity poke: can a subscribed track be playable in the browser but expose
  only FairPlay/Widevine/PlayReady streams to non-browser clients?
  Completed 2026-07-08 17:53 EDT.
- [x] Debug behavior: retain an env-gated live transcoding probe while redacting
  signed query strings and key attributes.
  Curiosity poke: can debug logs leak signed CDN URLs or DRM init data?
  Completed 2026-07-08 17:53 EDT.

## Previous Work: Interruptible Playback + Timeout Alignment (2026-07-08 EDT)

Goal: make selecting a second track interrupt current playback before SoundCloud
stream resolution, preserve private playlist context through playback, and remove
the mismatched TUI/CLI startup timeouts.

Done criteria:
- [x] Selecting a new track while audio is playing stops the active player before
  resolving the next stream URL.
- [x] Playlist/favorite tracks carry any SoundCloud secret context needed for
  private stream extraction.
- [x] TUI extraction/playback timeouts and CLI `--test-audio` startup timeout use
  shared constants, with the loading UI timeout long enough to cover both phases.
  Completed 2026-07-08 16:58 EDT.
- [x] Secret-token-only private tracks resolve via their secret permalink when no
  playlist context exists.
  Completed 2026-07-08 17:03 EDT.
- [x] `./test` and `./build` pass before commit.
  Completed 2026-07-08 17:04 EDT.

Next small behaviors:
- [x] Playback interruption behavior: add a regression proving `Stop` occurs
  before the next stream extraction command.
  Curiosity poke: does Bubble Tea `Batch` execution hide ordering assumptions?
  Completed 2026-07-08 16:58 EDT.
- [x] Private stream behavior: add a regression proving playlist ID/secret context
  reaches `GetTrackInfoWithOptions`.
  Curiosity poke: can private playlist tracks hydrate for display but fail later
  because playback refetches by naked track ID?
  Completed 2026-07-08 16:58 EDT.
- [x] Timeout behavior: add a regression proving TUI and CLI startup paths use the
  same audio timeout constants.
  Curiosity poke: can the loading UI timer fire before sequential extract+decode
  has had its full budget?
  Completed 2026-07-08 16:58 EDT.
- [x] Secret-token behavior: add a regression proving private favorites can use a
  secret permalink instead of a naked track ID.
  Curiosity poke: can a private track hydrate for display outside a playlist but
  fail during playback because only the ID was reused?
  Completed 2026-07-08 17:03 EDT.

## Previous Work: Playback Reuse + Large Playlist Fixes (2026-07-08 EDT)

Goal: make repeated track playback and very large personal playlists reliable.

Done criteria:
- [x] Playback stream extraction resolves the selected transcoding URL directly
  instead of re-resolving the permalink through the upstream helper on every
  play attempt.
  Completed 2026-07-08 12:38 EDT.
- [x] Playlist track hydration uses SoundCloud-sized 50-ID batches and includes
  playlist secret context when available.
  Completed 2026-07-08 12:38 EDT.
- [x] A >300-track playlist fixture fully populates in order.
  Completed 2026-07-08 12:38 EDT.
- [x] `./test` and `./build` pass before commit.
  Completed 2026-07-08 12:38 EDT.

Next small behaviors:
- [x] Playback extraction behavior: add a regression proving a direct
  transcoding resolver is used even when permalink-based `GetDownloadURL`
  would fail.
  Curiosity poke: can retrying playback fail because the helper refetches track
  metadata and gets a stale 404? Completed 2026-07-08 12:38 EDT.
- [x] Playlist hydration behavior: add a >300-track fixture that rejects
  batches larger than 50 IDs and requires playlist secret context.
  Curiosity poke: did our prior 100-ID batching still exceed SoundCloud's
  actual `/tracks?ids=` limit? Completed 2026-07-08 12:38 EDT.

## Previous Work: AAC/HLS Playback via ffmpeg (2026-07-07 EDT)

Goal: decode SoundCloud AAC/HLS streams reliably by shelling out to ffmpeg,
loading decoded PCM into memory so seeking/skipping is local once playback
starts.

Done criteria:
- [x] Real stream extraction prefers HLS when available and falls back to
  progressive for older tracks.
- [x] HLS playback decodes through an injectable ffmpeg adapter into an
  in-memory seekable PCM streamer.
- [x] Existing MP3/WAV URL playback remains compatible through `Play(ctx, url)`.
- [x] TUI and CLI preserve `StreamInfo.Format` instead of losing it at the URL
  boundary.
- [x] Nix package/dev shell expose `ffmpeg` at runtime.
- [x] `./test` and `./build` pass before commit.

Next small behaviors:
- [x] Extractor behavior: change existing progressive-preference tests to prove
  HLS is selected first.
  Curiosity poke: do we accidentally pick Opus HLS before AAC HLS when both are
  listed? Completed 2026-07-07 20:52 EDT.
- [x] Audio core behavior: add a pure PCM memory streamer test for stream,
  duration, and repeated seek.
  Curiosity poke: can frame alignment break when ffmpeg returns an odd byte
  count? Completed 2026-07-07 20:52 EDT.
- [x] Decoder behavior: add an ffmpeg command-runner test that verifies HLS URLs
  are decoded to stereo 44.1kHz s16le without invoking real ffmpeg.
  Curiosity poke: do context cancellation and subprocess stderr produce useful,
  token-safe errors? Completed 2026-07-07 20:52 EDT.
- [x] Playback wiring behavior: add a headless player test showing
  `PlayStream(... Format: "hls")` uses the decoder and supports seeking.
  Curiosity poke: does old `Play(ctx, url)` still route MP3/WAV through the
  HTTP decoder unchanged? Completed 2026-07-07 20:52 EDT.
- [x] Packaging behavior: add `ffmpeg` to Nix wrapper/dev shell and verify the
  binary can find it.
  Curiosity poke: is `ffmpeg` on PATH both in `nix run`/installed binary and
  `nix develop`? Completed 2026-07-07 20:52 EDT.

## Previous Work: Personal Playlist TUI Integration (2026-07-07 EDT)

Goal: let a signed-in SoundCloud browser session browse personal/private playlists
inside the interactive TUI and play selected tracks.

Done criteria:
- [x] `soundcloud.Client.PlaylistTracks(id)` returns fully hydrated tracks from a
  playlist fixture, including shallow `/playlists/{id}` entries hydrated through
  `/tracks?ids=...`. Completed 2026-07-07 19:09 EDT.
- [x] The interactive TUI header surfaces whether the app is signed in or browsing
  anonymously. Completed 2026-07-07 19:14 EDT.
- [x] The TUI has a playlist/library view: list playlists, drill into tracks, and
  send selected tracks to the existing player. Completed 2026-07-07 19:21 EDT.
- [x] The TUI has a favorites view for liked tracks and can play selected tracks.
  Completed 2026-07-07 19:21 EDT.
- [x] Favorites uses the live browser-OAuth-compatible endpoint
  `/users/<me.ID>/track_likes` instead of stale `/me/track_likes`.
  Completed 2026-07-07 19:28 EDT.
- [x] Long playlist/favorites views render bounded windows around the selection,
  and Playlists maps →/Enter to open/play plus ←/Esc to go back.
  Completed 2026-07-07 19:28 EDT.
- [x] Large playlists hydrate shallow track IDs in batches so 499-item playlists
  do not fail on an oversized `/tracks?ids=...` request.
  Completed 2026-07-07 19:30 EDT.
- [x] Short playable tracks start after completed download even when the file is
  smaller than the 1MB preload threshold.
  Completed 2026-07-07 19:39 EDT.
- [ ] `./test` and `./build` pass before each commit.

Next small behaviors:
- [x] API behavior: add fixture-tested `PlaylistTracks(id)`.
  Curiosity poke: what breaks when the playlist payload mixes full tracks and
  ID-only shallow tracks? Completed 2026-07-07 19:09 EDT.
- [x] API behavior: add fixture-tested `FavoriteTracks()`.
  Curiosity poke: does the library endpoint return liked tracks directly, or do
  we need a separate paginated track-likes endpoint? Completed 2026-07-07 19:12 EDT.
- [x] TUI auth notice behavior: render signed-in source in the header.
  Curiosity poke: how does the TUI behave if `Me()` fails after cookie discovery?
  Completed 2026-07-07 19:14 EDT.
- [x] Library behavior: render playlists and drill into tracks without blocking
  the Bubble Tea update loop.
  Curiosity poke: what should a system playlist without a numeric playlist ID do?
  Completed 2026-07-07 19:21 EDT.
- [x] Favorites behavior: fetch liked tracks through the logged-in api-v2 session
  and render/play them in a dedicated tab.
  Curiosity poke: does the library endpoint return liked tracks directly, or do
  we need a separate paginated track-likes endpoint? Completed 2026-07-07 19:21 EDT.
- [x] Bug fix: FavoriteTracks uses `/me` to find the signed-in numeric user ID,
  then fetches `/users/<id>/track_likes`.
  Curiosity poke: what happens when official `/me/likes/tracks` rejects the web
  session OAuth token? Completed 2026-07-07 19:28 EDT.
- [x] Bug fix: library list renderers window large sets at small terminal heights.
  Curiosity poke: can the selected row disappear above the viewport when Peter
  has many playlists? Completed 2026-07-07 19:28 EDT.
- [x] Bug fix: split large playlist track hydration into bounded batches.
  Curiosity poke: can an 8-track playlist pass while a 499-track playlist fails
  from URL/request-size limits? Completed 2026-07-07 19:30 EDT.
- [x] Bug fix: completed short downloads count as preloaded for the buffered
  player.
  Curiosity poke: can a valid playable track fail locally just because it never
  reaches the preload byte threshold? Completed 2026-07-07 19:39 EDT.

## Current Status (December 2024)

### ✅ COMPLETED PHASES
- **Phase 1-4**: Complete TUI implementation with working audio playback
- **Phase 5**: Real audio implementation with HTTP streaming
- **Recent Fix**: Audio playback reliability and user feedback system
- **Phase 6**: Direct URL playback feature with auto-restart functionality
- **Phase 7**: Audio streaming responsiveness improvements

#### Latest Improvements (Audio Streaming & UI Responsiveness)
- ✅ **Direct Play Feature**: Added `--play <url>` flag for immediate track playback
- ✅ **Debug Tools**: Implemented `--test-audio` and `--test-tui` for troubleshooting
- ✅ **Auto-Restart**: Position-preserving restart for premature playback stops
- ✅ **Error Investigation**: Deep analysis of Beep library premature stopping issues
- ✅ **State Management**: Enhanced premature stop detection and recovery
- ✅ **Buffered Streaming**: Implemented BufferedStreamPlayer with progressive download
- ✅ **Audio/UI Coordination**: Fixed blocking issues between audio loading and UI responsiveness
- ✅ **Timeout Management**: Added proper timeout protection to prevent hanging
- ✅ **Error Handling**: Enhanced error recovery and state transitions for audio playback

#### Technical Architecture Achieved
- Complete Bubble Tea TUI with Search/Player/Playlists/Favorites views
- Real SoundCloud API integration via github.com/zackradisic/soundcloud-api
- Beep audio library with streaming MP3/WAV support
- Comprehensive error handling and state management
- Test-driven development with unit and integration tests

### 🎯 NEXT PHASE: Production Readiness

## OAuth 2.0 Browser-Based Authentication

### SoundCloud API registration challenges mean you cannot get official API access

The SoundCloud API v1 is **currently closed for new registrations** as of 2024,
presenting a fundamental challenge for new open-source projects. The unofficial
API v2 violates Terms of Service and is unreliable. For a legitimate open-source
implementation, you'll need to implement one of these strategies:

1. **User-provided API credentials** - Transfer responsibility to users who have
   existing access
2. **Contact SoundCloud directly** - Request special developer access for your
   open-source project
3. **Implement OAuth with placeholder credentials** - Allow users to configure
   their own client ID/secret

### Secure OAuth implementation pattern

Using the GitHub CLI approach as a reference, implement a dual-flow OAuth
strategy:

```go
// Core OAuth flow structure
type OAuthFlow struct {
    config      *oauth2.Config
    keyring     keyring.Keyring
    deviceFlow  bool
    verifier    string // For PKCE
}

// Implement browser-based flow with PKCE
func (f *OAuthFlow) AuthorizeBrowser(ctx context.Context) (*oauth2.Token, error) {
    // Generate PKCE verifier
    f.verifier = oauth2.GenerateVerifier()

    // Start local callback server
    redirectURL := "http://127.0.0.1:8888/callback"
    f.config.RedirectURL = redirectURL

    // Generate authorization URL with PKCE challenge
    authURL := f.config.AuthCodeURL("state",
        oauth2.AccessTypeOffline,
        oauth2.S256ChallengeOption(f.verifier))

    // Open browser
    if err := browser.OpenURL(authURL); err != nil {
        return nil, fmt.Errorf("failed to open browser: %w", err)
    }

    // Start callback server and wait for code
    code := f.waitForCallback()

    // Exchange code for token with PKCE verifier
    return f.config.Exchange(ctx, code, oauth2.VerifierOption(f.verifier))
}
```

### Token storage security

Use **99designs/keyring** for cross-platform secure storage with fallback to
encrypted config file:

```go
type TokenManager struct {
    keyring keyring.Keyring
    appName string
}

func (tm *TokenManager) StoreToken(token *oauth2.Token) error {
    // Try keyring first
    data, _ := json.Marshal(token)
    err := tm.keyring.Set(keyring.Item{
        Key:  "soundcloud_token",
        Data: data,
        Label: "SoundCloud OAuth Token",
    })

    if err != nil {
        // Fallback to encrypted file with user warning
        log.Warn("Failed to store token securely, using encrypted file")
        return tm.storeEncryptedFile(token)
    }
    return nil
}
```

## API Integration Architecture

### Rate limiting implementation

SoundCloud enforces **15,000 requests per day** - implement aggressive rate
limiting:

```go
type RateLimitedClient struct {
    httpClient *http.Client
    limiter    *rate.Limiter
    cache      *ttlcache.Cache
}

func NewSoundCloudClient() *RateLimitedClient {
    return &RateLimitedClient{
        httpClient: &http.Client{Timeout: 30 * time.Second},
        limiter:    rate.NewLimiter(rate.Every(24*time.Hour/15000), 10), // burst of 10
        cache:      ttlcache.NewCache(),
    }
}

func (c *RateLimitedClient) Get(ctx context.Context, url string) (*http.Response, error) {
    // Check cache first
    if cached, exists := c.cache.Get(url); exists {
        return cached.(*http.Response), nil
    }

    // Rate limit
    if err := c.limiter.Wait(ctx); err != nil {
        return nil, err
    }

    // Make request with exponential backoff
    return c.doWithRetry(ctx, url)
}
```

### Error handling with retry logic

```go
func (c *RateLimitedClient) doWithRetry(ctx context.Context, url string) (*http.Response, error) {
    backoff := []time.Duration{1 * time.Second, 3 * time.Second, 10 * time.Second}

    var lastErr error
    for attempt, delay := range backoff {
        resp, err := c.httpClient.Get(url)

        if err == nil && resp.StatusCode < 500 {
            if resp.StatusCode == 429 { // Rate limited
                retryAfter := resp.Header.Get("Retry-After")
                time.Sleep(parseRetryAfter(retryAfter))
                continue
            }
            return resp, nil
        }

        lastErr = err
        if attempt < len(backoff)-1 {
            time.Sleep(delay)
        }
    }

    return nil, fmt.Errorf("request failed after retries: %w", lastErr)
}
```

## Audio Streaming Architecture

### Streaming buffer management

Implement progressive download with intelligent buffering:

```go
type AudioStreamer struct {
    url         string
    buffer      *ring.Ring
    bufferSize  int
    preloadSize int
    client      *http.Client
    mu          sync.RWMutex
}

func (s *AudioStreamer) Stream(ctx context.Context) (beep.StreamCloser, beep.Format, error) {
    // Start progressive download in background
    go s.downloadInBackground(ctx)

    // Wait for initial buffer fill
    s.waitForPreload()

    // Create custom streamer that reads from ring buffer
    return &BufferedStreamer{
        buffer: s.buffer,
        format: s.detectFormat(),
    }, s.format, nil
}

func (s *AudioStreamer) downloadInBackground(ctx context.Context) {
    req, _ := http.NewRequestWithContext(ctx, "GET", s.url, nil)
    req.Header.Set("Range", fmt.Sprintf("bytes=%d-", s.currentPos))

    resp, err := s.client.Do(req)
    if err != nil {
        return
    }
    defer resp.Body.Close()

    buffer := make([]byte, 32*1024) // 32KB chunks
    for {
        n, err := resp.Body.Read(buffer)
        if n > 0 {
            s.mu.Lock()
            s.buffer.Value = buffer[:n]
            s.buffer = s.buffer.Next()
            s.mu.Unlock()
        }

        if err == io.EOF {
            break
        }
    }
}
```

### Beep audio integration

```go
type Player struct {
    ctrl     *beep.Ctrl
    volume   *effects.Volume
    format   beep.Format
    speaker  *sync.Once
}

func (p *Player) Play(streamer beep.StreamCloser, format beep.Format) error {
    // Initialize speaker once
    p.speaker.Do(func() {
        speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
    })

    // Wrap with control and volume
    p.ctrl = &beep.Ctrl{Streamer: streamer}
    p.volume = &effects.Volume{
        Streamer: p.ctrl,
        Base:     2,
        Volume:   0,
    }

    speaker.Play(p.volume)
    return nil
}
```

## TUI Architecture with Bubble Tea

### Project structure

```
soundcloud-tui/
├── cmd/
│   └── sctui/
│       └── main.go
├── internal/
│   ├── ui/
│   │   ├── app/
│   │   │   └── app.go         # Main app model
│   │   ├── components/
│   │   │   ├── player/        # Player controls
│   │   │   ├── search/        # Search interface
│   │   │   └── playlist/      # Playlist view
│   │   └── styles/
│   │       └── theme.go        # Lipgloss styles
│   ├── audio/
│   │   ├── player.go          # Audio player interface
│   │   ├── beep.go            # Beep implementation
│   │   └── stream.go          # Streaming logic
│   ├── api/
│   │   ├── client.go          # SoundCloud API client
│   │   ├── auth.go            # OAuth implementation
│   │   └── models.go          # API data models
│   └── config/
│       ├── config.go          # Configuration management
│       └── keyring.go         # Secure storage
└── pkg/
    └── soundcloud/            # Public API wrapper
```

### Main application model

```go
type App struct {
    // Sub-components
    player   player.Model
    search   search.Model
    playlist playlist.Model

    // Audio state
    audioPlayer audio.Player
    nowPlaying  *api.Track

    // UI state
    activeView  View
    windowSize  tea.WindowSizeMsg

    // Business logic
    client *api.Client
    config *config.Config
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return a.handleKeypress(msg)

    case tea.WindowSizeMsg:
        a.windowSize = msg
        return a, a.propagateResize()

    case player.PlaybackMsg:
        return a.handlePlayback(msg)

    case api.SearchResultMsg:
        a.search.SetResults(msg.Tracks)
        return a, nil
    }

    // Delegate to active component
    return a.delegateUpdate(msg)
}
```

### Component communication pattern

Use commands for all async operations and cross-component communication:

```go
// Audio state updates
func audioProgressTick() tea.Cmd {
    return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
        return progressUpdateMsg{time: t}
    })
}

// API requests
func searchTracks(query string, client *api.Client) tea.Cmd {
    return func() tea.Msg {
        tracks, err := client.Search(query)
        if err != nil {
            return errMsg{err}
        }
        return searchResultMsg{tracks}
    }
}
```

## Testing Strategy

### Unit testing with mocks

```go
// Mock audio player for testing
type MockPlayer struct {
    mock.Mock
}

func (m *MockPlayer) Play(url string) error {
    args := m.Called(url)
    return args.Error(0)
}

// Test model updates
func TestPlayerModel_PlayTrack(t *testing.T) {
    mockPlayer := new(MockPlayer)
    mockPlayer.On("Play", "http://example.com/track.mp3").Return(nil)

    model := player.New(mockPlayer)
    _, cmd := model.Update(player.PlayMsg{URL: "http://example.com/track.mp3"})

    require.NotNil(t, cmd)
    mockPlayer.AssertExpectations(t)
}
```

### Integration testing with teatest

```go
func TestAppIntegration(t *testing.T) {
    // Create test model with mocked dependencies
    app := createTestApp()

    tm := teatest.NewTestModel(t, app,
        teatest.WithInitialTermSize(80, 24))

    // Simulate user interaction
    tm.Send(tea.KeyMsg{Type: tea.KeyCtrlS}) // Open search
    tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test song")})
    tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

    // Wait for search to complete
    teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
        return strings.Contains(string(out), "Search Results")
    })

    // Verify output
    golden := filepath.Join("testdata", "search_results.golden")
    teatest.RequireEqualOutput(t, tm.FinalOutput(t), golden)
}
```

## CI/CD Configuration

### GitHub Actions workflow

```yaml
name: Build and Release

on:
  push:
    branches: [main]
    tags: ["v*"]
  pull_request:

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
        go: ["1.21", "1.22"]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}

      - name: Install dependencies
        run: |
          if [ "$RUNNER_OS" == "Linux" ]; then
            sudo apt-get update
            sudo apt-get install -y libasound2-dev
          fi
        shell: bash

      - name: Run tests
        run: go test -v -race ./...

      - name: Run linter
        uses: golangci/golangci-lint-action@v3

  release:
    if: startsWith(github.ref, 'refs/tags/')
    needs: test
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
          - goos: darwin
            goarch: amd64
          - goos: darwin
            goarch: arm64
          - goos: windows
            goarch: amd64

    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.21"

      - name: Build binary
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          CGO_ENABLED: 0
        run: |
          go build -ldflags="-s -w" -o sctui-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/sctui

      - name: Upload release assets
        uses: softprops/action-gh-release@v1
        with:
          files: sctui-*
```

## Key Implementation Considerations

### Security best practices

1. **Never embed SoundCloud API secrets** in open source code
2. **Use PKCE for all OAuth flows** to prevent authorization code interception
3. **Store tokens in system keyring** with encrypted file fallback
4. **Validate all API responses** and handle rate limiting gracefully

### Performance optimization

1. **Aggressive caching** to minimize API calls (15k/day limit)
2. **Progressive audio download** with ring buffer for smooth playback
3. **Concurrent component updates** using Bubble Tea commands
4. **Lazy loading** for search results and playlists

### User experience

1. **Vim-style keybindings** for navigation
2. **Real-time search** with debouncing
3. **Progress bars** for playback and downloads
4. **Responsive layout** that adapts to terminal size

### Distribution strategy

1. **go install** support:
   `go install github.com/yourusername/soundcloud-tui@latest`
2. **Homebrew formula** for macOS users
3. **Snap package** for Linux with audio permissions
4. **GitHub releases** with pre-built binaries

This implementation plan provides a solid foundation for building a
production-ready SoundCloud TUI client while navigating the API access
limitations and ensuring security, performance, and excellent user experience.
