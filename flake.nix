{
  description = "sctui — a SoundCloud terminal UI (search + stream playback)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        inherit (pkgs) lib stdenv;

        # ── Dependency hash ────────────────────────────────────────────────
        # buildGoModule needs the hash of the vendored Go module tree. To
        # (re)generate it: set this to `lib.fakeHash`, run `nix build`, and
        # paste the hash Nix reports. Shared by the package and the test check.
        vendorHash = "sha256-2KaNSQn+QV4Rpb/VfAuNzB6SSpDXza0COeWY1ZGsSnw=";

        # ── Native audio deps ──────────────────────────────────────────────
        # The audio backend (ebitengine/oto) binds ALSA via cgo on Linux and
        # discovers it with pkg-config at build time. Because it's linked (not
        # dlopen'd), Nix bakes libasound into the binary's RPATH, so no runtime
        # wrapper is needed. macOS uses the AudioToolbox framework — no extra
        # inputs. These lists are empty on non-Linux.
        audioNativeInputs = lib.optionals stdenv.isLinux [ pkgs.pkg-config ];
        audioBuildInputs = lib.optionals stdenv.isLinux [ pkgs.alsa-lib ];
        goParallelism = ''
          cores="''${NIX_BUILD_CORES:-1}"
          if [ -z "$cores" ] || [ "$cores" = "0" ]; then
            cores="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 1)"
          fi
          export GOMAXPROCS="$cores"
          export GOFLAGS="''${GOFLAGS:-} -p=$cores"
          echo "Using $cores Go build/test workers"
        '';

        sctui = pkgs.buildGoModule {
          pname = "sctui";
          version = self.shortRev or self.dirtyShortRev or "dev";
          src = ./.;
          inherit vendorHash;

          # Only build the TUI app. cmd/test is a live-API smoke script that
          # needs network + real SoundCloud access; it has no place in a
          # sandboxed build.
          subPackages = [ "cmd/sctui" ];

          # The full suite lives under ./tests and is run by the `test` check
          # below, not during the package build (keeps `nix build` lean).
          doCheck = false;

          nativeBuildInputs = audioNativeInputs ++ [ pkgs.makeWrapper ];
          buildInputs = audioBuildInputs;

          preBuild = goParallelism;

          # sctui shells out to `sqlite3` for browser cookies and `ffmpeg` for
          # SoundCloud AAC/HLS decoding; put both on PATH for installed runs.
          postInstall = ''
            wrapProgram $out/bin/sctui --prefix PATH : ${lib.makeBinPath [ pkgs.sqlite pkgs.ffmpeg ]}
          '';

          meta = {
            description = "SoundCloud terminal UI (search + stream playback)";
            mainProgram = "sctui";
            platforms = lib.platforms.unix;
          };
        };
      in
      {
        # ── build ────────────────────────────────────────────────────────
        # nix build            → ./result/bin/sctui
        packages.default = sctui;
        packages.sctui = sctui;

        # ── run ──────────────────────────────────────────────────────────
        # nix run              → launches the TUI
        # nix run . -- -search "lofi"
        apps.default = {
          type = "app";
          program = lib.getExe sctui;
        };

        # ── CI ───────────────────────────────────────────────────────────
        # Garnix auto-evaluates every attr under `checks`. `nix flake check`
        # runs them all locally.
        checks = {
          # Reuse the exact package build as a check.
          build = sctui;

          # Full test suite: `go test ./...` over the sandboxed source.
          test = pkgs.buildGoModule {
            pname = "sctui-tests";
            version = "test";
            src = ./.;
            inherit vendorHash;

            # We run the suite ourselves in buildPhase; skip buildGoModule's
            # default checkPhase (its getGoDirs helper isn't set up here).
            doCheck = false;

            # sqlite provides sqlite3 for the session-cookie round-trip test.
            nativeBuildInputs = audioNativeInputs ++ [ pkgs.sqlite ];
            buildInputs = audioBuildInputs;

            # Replace the default compile step with the test run.
            buildPhase = ''
              runHook preBuild
              export HOME="$TMPDIR"
              ${goParallelism}
              echo "── go test ./... ──"
              go test ./...
              runHook postBuild
            '';

            installPhase = ''
              mkdir -p "$out"
              echo "tests passed" > "$out/result"
            '';
          };
        };

        # ── dev shell ────────────────────────────────────────────────────
        # nix develop          → go, gopls, staticcheck, and audio libs
        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.gotools
            pkgs.go-tools # staticcheck et al.
            pkgs.sqlite # sqlite3, for reading the browser session cookie
            pkgs.ffmpeg # AAC/HLS decoding for current SoundCloud streams
          ]
          ++ audioNativeInputs
          ++ audioBuildInputs;

          # cgo links libasound by full store path, but the dev binary won't
          # carry an RPATH — expose it so `go run ./cmd/sctui` works in-shell.
          shellHook = lib.optionalString stdenv.isLinux ''
            export LD_LIBRARY_PATH=${lib.makeLibraryPath audioBuildInputs}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
          '';
        };

        formatter = pkgs.nixpkgs-fmt;
      });
}
