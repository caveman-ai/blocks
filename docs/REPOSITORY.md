# Repository layout and tech stack

## Stack

| Concern | Choice | Why |
|---|---|---|
| CLI and hook | Go 1.26, one static binary, standard library plus `pelletier/go-toml/v2` and `spf13/cobra` only | Runs on every shell call; no runtime dependency on the user's machine; cross-compiles. Decision 0007 |
| First-party blocks | Python 3.10+ standard library | Present on nearly every development machine, including Windows; stdlib keeps blocks dependency-free |
| Regex | Go `regexp` (RE2) for `matches` patterns | Linear time; a block cannot hang the hook with a pathological pattern |
| Tests | `go test` with golden tables; `testscript` for CLI end-to-end; Python `unittest` fixtures for blocks | Hermetic, fast, readable diffs |
| Lint | `gofmt`, `go vet` (`make lint`) | Standard |
| Release | GoReleaser: GitHub releases with checksums, `homebrew_casks` (skipped for prereleases); hand-written `install.sh`; npm shim plus per-platform packages published by CI, prereleases under the `next` tag. macOS signing and notarization are NOT configured yet; until they are, Homebrew users will hit Gatekeeper and the cask is not published | Decision 0007 |
| CI | GitHub Actions: `test`, `lint`, `e2e`, `bench-hook`, `blocks-test`, `blocks-verify` on macOS and Linux. On Windows a `windows` job vets, builds and runs `scripts/smoke-hooks.sh` (hooks install, status, uninstall under Git Bash); the Go test suite and the Python blocks are not run there | Hook budget enforced in CI |
| Docs | Markdown in `docs/`, decision records in `docs/decisions/` | Rationale next to code |

Not used: a model API, embeddings, tree-sitter, a database, a daemon, a network library (`net/http` stays
out of the binary; telemetry hands its payload to the system `curl`, decision 0011).

## Layout

```
caveman-blocks/
  cmd/caveman-blocks/        main.go: cobra root, subcommands wired to internal packages
  internal/
    blockfile/               parse, write, validate, lint      (pure; golden tests)
    index/                   INDEX.md and managed-section rendering, freshness check (pure)
    hook/                    decision engine                   (pure; golden tables in internal/hook/testdata/)
      protocol/              generic JSON request/response, versioned (pure)
      dialect/<name>/        claude, cursor, generic: parse + render (pure; conformance fixtures)
      profiles.toml          per-harness data: dialect, events, timeout, config path and format
      install/               config_format writers: insert, remove, status for each config file shape
      testdata/              golden decision tables, dialect conformance fixtures, install cases
    capture/                 script extraction, edit filter, normalization, shape, scrub, sightings store
      callnames.go           the fixed call-name table used by the shape fingerprint
      testdata/scrub/        golden scrub cases
    scan/                    transcript readers (readers.go, one per harness), grouping, report
      testdata/<harness>/    scrubbed transcript samples for the readers
    verify/                  run example from the working tree, check contract, write stamp
    registry/                embedded first-party blocks (go:embed), add, diff, update, lock file
    promote/                 brief rendering, candidate ranking, retire
    runner/                  caveman-blocks run: effects gate, path confinement, exec, cap, spill
    stats/                   append and summarize events
    telemetry/               saved choice, wire events, daily rollup of stats.jsonl, flush through the system curl
    export/                  SKILL.md rendering for export   (pure; golden test)
    repo/                    repo root, .blocks/, config.toml at HEAD, state dir, default branch, instruction files
  blocks/                    first-party blocks, one file each, plus fixtures/<name>/ and tests/; README.md is the registry table
  testdata/
    e2e/                     testscript scenarios run by make e2e (build tag e2e)
  npm/                       shim package and per-platform package templates
  plugins/caveman-blocks/    Claude Code and Codex plugin (.claude-plugin and .codex-plugin manifests, hooks/hooks.json and shim, skill), and opencode/ and pi/ plugin files;
                             listed by .claude-plugin/marketplace.json and .agents/plugins/marketplace.json; plugins/embed.go embeds the OpenCode and pi files
  docs/                      see README
  .github/workflows/         ci.yml, release.yml
  Makefile                   test, lint, e2e, bench-hook, build, blocks-test, blocks-lint, blocks-verify
  AGENTS.md                  instructions for agents working in this repo; there is no CLAUDE.md
```

## Package rules

- `blockfile`, `index`, `hook` and the pure parts of `capture` and `scan` import nothing from the impure
  packages. They take values and return values. Every behavior change starts as a new golden row.
- Adapters are the only code that knows a harness's field names. The engine sees `hook.Input`.
- No package opens a network connection. `telemetry` hands one JSON payload to the system `curl`
  ([TELEMETRY.md](TELEMETRY.md)). When `diff` and `update` land, `registry` is the only one allowed network code.
- No package reads environment variables except `repo` (for `HOME` and `XDG_*`), the adapters (for
  harness-specific paths) and `telemetry` (its off switches, through an injected getter).
- Errors are values with a stable code (`F001` and friends for lint, `H001` for hook internals) so the
  CLI output and the docs can name them.

## Commands

| Command | Purpose |
|---|---|
| `init` | Create `.blocks/` and `config.toml` in the current directory; run `sync` |
| `hooks install\|uninstall\|status [--harness x]` | Per-machine hook entries for detected harnesses (`claude-code`, `codex`, `cursor`, `opencode`, `pi`), or the named ones. `status --json` prints one object: `version` and per harness `name`, `installed`, `binary`, `binary_present`, `config` |
| `add <name> [--force]` | Copy a first-party block into `.blocks/`, record in `blocks.lock`, `verify`, `sync` |
| `run <name> [--param v]` | Execute a block: effects gate from committed config, path confinement, cap, spill |
| `lint [path] [--first-party]` | Format and rule checks; exits non-zero with coded findings |
| `verify [name\|--changed\|--all] [--check] [--policy-ref r] [--fixtures-root d] [--first-party]` | Run examples, write content-hash stamps, quarantine failures; `--check` writes nothing and fails; `--first-party` is for this repo's unstamped registry; see CI.md |
| `sync [--check]` | Regenerate `INDEX.md`, managed sections and exports; `--check` for CI |
| `scan [--since 30d] [--harness x]` | Repeat table from local transcripts |
| `promote [fp]` | List repeated shapes or print the promotion brief |
| `retire <name>` | Remove a block and `sync` |
| `diff [name]` / `update [name]` | Phase 2: compare and refresh first-party blocks against a newer registry |
| `export` | Write `SKILL.md` wrappers into `.claude/skills/` and `.agents/skills/` |
| `stats [--since 7d]` | Summarize counted events |
| `hook --harness <claude\|codex\|cursor\|generic> [--phase pre\|post]` | Entry point the harness calls; `generic` speaks the protocol in INTEGRATION.md; always exits 0 |
| `telemetry status\|on\|off` | Show, enable or disable usage counts; see [TELEMETRY.md](TELEMETRY.md). A hidden `telemetry flush` is the detached sender |
| `doctor` | Report binary and hook copy versions, hook configs and trust notes, Python, parent `CLAUDE.md`, CI workflow, sync state, indexed blocks |
| `version [--json]` | Print the version; `--json` adds `capabilities` (`hooks_status_json`) |

Exit codes: 0 success; 1 failure (lint finding, failed or stale verify, stale sync, refused run); 2 usage
error. `run` passes the block's exit code through. `hook` always exits 0.
