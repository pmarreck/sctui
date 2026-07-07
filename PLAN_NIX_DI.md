---
purpose: Track the nixification + audio dependency-injection refactor work
audience: agent
maintained_by: agent
---

# Nixify + Audio DI Refactor — Working Plan

Goal: `flake.nix` for build/test/run/CI on NixOS (+ 4 Nix platforms; Windows via Go
cross), and make the audio + UI test suite deterministic (no live network, no real
audio device) so `go test ./...` is green in the Nix sandbox (Garnix CI).

## Done (2026-07-07 EST)
- [x] `flake.nix`: buildGoModule package (ALSA linked via RPATH), `nix run`, dev shell, `checks.{build,test}`.
- [x] Fixed Go package-name conflict: `mocks.go`→`mocks_test.go`, `test_mocks.go`→`mocks_test.go`.
- [x] Removed duplicate inline mocks in `ui/player_test.go`, `ui/search_test.go`; added missing `GetDownloadURL`.
- [x] Removed unused `time` import in `ui/volume_controls_test.go`.
- [x] Rewrote buffered-player callbacks test: deterministic (channel sync, empty-URL sync fail, Stop() trigger).
- [x] **Audio DI**: `AudioSink` port + `speakerSink` adapter; injectable `httpClient` via `WithAudioSink`/`WithHTTPClient` options on `BeepPlayer`. Default `NewBeepPlayer()` unchanged.
- [x] Fixed latent self-deadlock: `Seek` called `GetDuration` (RLock while holding Lock) → `getDurationLocked`.
- [x] Extractor DI: `WithExtractorHTTPClient` option so `ValidateStreamURL` uses an injectable client.
- [x] Rewrote `audio/player_test.go` + `audio/stream_test.go` to inject fakes (fake sink, WAV responder, mock API); removed `time.Sleep` (zero-timeout ctx).
- [x] Added `audio/testhelpers_test.go`: fake `AudioSink`, HTTP `RoundTripper`, in-memory WAV generator (`wav.Encode`).
- [x] Fixed real prod bug: `renderErrorView` nil-error panic (guard `p.error == nil`).
- [x] Fixed prod: `renderPlayingView` reads live player volume; kept position/duration cached (Bubble Tea msg-driven).
- [x] Fixed prod: resume-from-paused sets `StatePlaying` immediately (UI feedback).
- [x] Fixed mock bug: `MockAudioPlayer.GetVolume` returned 1.0 for 0 (made mute untestable) → returns raw value.
- [x] Fixed test bugs: `TrackProgress` ns-vs-s units; `VolumeControl` mock default; `StreamInfoHandling` stale expectation; `ProgressBarAccuracy` byte-length→`lipgloss.Width`; `ProgressDisplay_VolumeDisplay` hardcoded icon + stale 50% boundary.
- [x] `gofmt -w` all modified files.
- [x] **Green in the Nix sandbox** (`nix build .#checks.<system>.test`) — no network, no audio device.
- [x] `doCheck = false` on the test check (silences buildGoModule's `getGoDirs` warning).
- [x] `jj git init --colocate` so the standard `nix build .` works; gitignored `result*`.
- [x] Added `./build` and `./test` wrapper scripts.

## Done (2026-07-07 EST, cont.)
- [x] Buffered player DI: `WithBufferedHTTPClient`/`WithBufferedRetry`/`WithPreloadTimeout` + injectable `preloadTimeout`. `ErrorHandling`/`ContextCancellation` tests inject a failing client → hermetic, no network. **Audio suite 10.2s → 0.46s.**

## Open follow-ups (not blocking; CI is green)
- [ ] Consider `gofmt -w` across the whole (never-formatted) tree, or leave to a separate cleanup PR.
- [ ] `--about` CLI flag doesn't exist (app uses `-help`); out of scope for nixify.

## Notes
- Windows is NOT a Nix build target; cross-compile via `GOOS=windows go build` (oto uses purego on Windows, no cgo). The flake covers the 4 Unix systems via `flake-utils.eachDefaultSystem`.
- oto binds ALSA via cgo+pkg-config on Linux → Nix bakes libasound into RPATH (no runtime wrapper needed). macOS uses AudioToolbox (no extra inputs).
