# Learnings

- 2026-07-07: After adding new files in this jj + Nix flake repo, run
  `jj file track <paths>` explicitly before `./test`; otherwise the flake
  source can omit new files even though package-local `go test` sees them.
