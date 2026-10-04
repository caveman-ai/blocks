# 0007: Go, one static binary, npm shim for `npx`

Date: 2026-10-03. Status: accepted.

## Context

The binary runs as a hook on every shell call, must install with one line, and must not require a runtime the customer repo lacks. Caveman Wrap is a Node CLI with Go companions. A Node implementation was considered: simplest to write, but adds 40–80 ms startup to every shell call and requires Node on the machine.

## Decision

Write the CLI and hook in Go as one static binary with zero runtime dependencies. Distribute through GitHub releases, a Homebrew tap, a `curl | sh` installer, and an npm package that holds a small JS shim plus per-platform optional packages so `npx caveman-blocks` works for the first-minute install. First-party blocks are Python 3.10+ standard library only, because Python 3 is present on nearly every development machine and runs on Windows.

## Consequences

- Hot-path latency a few milliseconds above the process-spawn floor (measured 2026-10-03: a static Go binary at about 16 ms median against 117 ms for an empty Node script on a loaded M-series laptop).
- The npm shim is a bootstrap, never the hot path. `hooks install` copies the binary to `~/.local/share/caveman-blocks/bin/` and writes that path into the hook configuration; it refuses paths under npm or npx caches, which are garbage collected. A hook that went through `npx` or the JS shim would pay a Node start on every shell call.
- npm layout: a main package with a 20-line shim and per-platform `optionalDependencies` packages (`os`, `cpu`, `files`), the esbuild and Biome pattern. GoReleaser's own npm publisher is Pro-only and uses a postinstall download instead, so publishing is a small CI script or the MIT `goreleaser-npm-publisher`.
- Homebrew through GoReleaser `homebrew_casks` (the `brews` key is deprecated); macOS binaries must be signed and notarized or users see a damaged-app error. `install.sh` is hand-written and verifies the release checksum.
- Cross-compilation and release automation through GoReleaser.
- Contributors need Go; block authors only need Python.
- Windows: the binary builds, but heredoc extraction targets POSIX shells in v0; PowerShell is a later adapter.
