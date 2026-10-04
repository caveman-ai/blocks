# Repository layout and tech stack

## Stack

| Concern | Choice | Why |
|---|---|---|
| CLI and hook | Go 1.26, one static binary, standard library plus `pelletier/go-toml/v2` and `spf13/cobra` only | Runs on every shell call; no runtime dependency on the user's machine; cross-compiles. Decision 0007 |
| First-party blocks | Python 3.11+ standard library | Present on nearly every development machine, including Windows; stdlib keeps blocks dependency-free |
| Regex | Go `regexp` (RE2) for `matches` patterns | Linear time; a block cannot hang the hook with a pathological pattern |
| Tests | `go test` with golden tables; `testscript` for CLI end-to-end; Python `unittest` fixtures for blocks | Hermetic, fast, readable diffs |
| Lint | `gofmt`, `go vet`, `golangci-lint` with the default set plus `errcheck` | Standard |
| Release | GoReleaser: GitHub releases with checksums, `homebrew_casks`, signed and notarized macOS binaries; hand-written `install.sh`; npm shim plus per-platform packages published by CI | Decision 0007 |
| CI | GitHub Actions: `test`, `lint`, `e2e`, `bench-hook` on macOS and Linux; Windows builds and runs unit tests only in v0 | Hook budget enforced in CI |
| Docs | Markdown in `docs/`, decision records in `docs/decisions/` | Rationale next to code |

Not used: a model API, embeddings, tree-sitter, a database, a daemon, telemetry.

## Layout

```
caveman-blocks/
  cmd/caveman-blocks/        main.go: cobra root, subcommands wired to internal packages
  internal/
    blockfile/               parse, write, validate, lint      (pure; golden tests)
    index/                   INDEX.md and managed-section rendering, freshness check (pure)
    hook/                    decision engine                   (pure; golden decision tables)
      protocol/              generic JSON request/response, versioned (pure)
      dialect/<name>/        claude, cursor, copilot, gemini, generic: parse + render (pure; conformance fixtures)
      profiles.toml          per-harness data: dialect, events, config path, capabilities, transcripts
      install/               config_format writers: insert, remove, status for each config file shape
    capture/                 script extraction, edit filter, normalization, shape, candidates store
    scan/                    transcript readers (one per harness, versioned), grouping, report
    verify/                  run example at HEAD, check contract, write stamp
    registry/                embedded first-party blocks (go:embed), add, diff, update, lock file
    promote/                 brief rendering, candidate ranking, retire
    runner/                  blocks run: exec, cap, spill, effects check
    stats/                   append and summarize events
    repo/                    find repo root and .blocks/, config.toml, instruction-file discovery
  blocks/                    first-party blocks, one file each; README.md is the registry table
  testdata/
    blocks/<name>/           fixtures each block's example needs
    repo/                    a small fixture repository for end-to-end tests
    transcripts/<harness>/   scrubbed transcript samples for scan readers
    hook/                    golden decision tables: command in, decision out
  npm/                       shim package and per-platform package templates
  integrations/<harness>/    optional plugin shims for Tier C harnesses (Amp, OpenCode, Cline), separately versioned
  docs/                      see README
  .github/workflows/         ci.yml, release.yml
  Makefile                   test, lint, e2e, bench-hook, build
  AGENTS.md = CLAUDE.md      instructions for agents working in this repo
```

## Package rules

- `blockfile`, `index`, `hook` and the pure parts of `capture` and `scan` import nothing from the impure
  packages. They take values and return values. Every behavior change starts as a new golden row.
- Adapters are the only code that knows a harness's field names. The engine sees `hook.Input`.
- `registry` is the only package allowed to make network calls, and only in `add`, `diff` and `update`.
- No package reads environment variables except `repo` (for `HOME` and `XDG_*`) and the adapters (for
  harness-specific paths).
- Errors are values with a stable code (`F001` and friends for lint, `H001` for hook internals) so the
  CLI output and the docs can name them.

## Commands

| Command | Purpose |
|---|---|
| `init` | Create `.blocks/`, gitignore lines, `config.toml`; run `sync` |
| `hooks install\|uninstall\|status` | Per-machine hook entries for detected harnesses |
| `add <name>` | Copy a first-party block into `.blocks/`, record in `blocks.lock`, `sync` |
| `run <name> [--param v]` | Execute a block with cap, spill and effects check |
| `lint [path]` | Format and rule checks; exits non-zero with coded findings |
| `verify [name\|--changed\|--all]` | Run examples, write stamps, quarantine failures |
| `sync [--check]` | Regenerate `INDEX.md`, managed sections and exports; `--check` for CI |
| `scan [--since 30d] [--harness x]` | Repeat table from local transcripts |
| `promote [fingerprint]` | List candidates or print the promotion brief |
| `retire <name>` | Remove a block and `sync` |
| `diff [name]` / `update [name]` | Compare and refresh first-party blocks against the embedded registry |
| `export` | Write `SKILL.md` wrappers |
| `stats [--since 7d]` | Summarize counted events |
| `hook --harness <x>` | Entry point the harness calls; not for people |
| `doctor` | Check binary path in hook configs, Python availability, trust state |
