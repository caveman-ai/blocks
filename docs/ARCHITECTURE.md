# Architecture

One static binary, `caveman-blocks`, used three ways: as a CLI by people and agents, as a hook by the
agent harness, and as a CI step. No daemon, no service, no model calls, no network in v0.

```
                 ┌───────────────────────────────────────────────────────────────┐
                 │  repo (committed)                                             │
  agent session  │  AGENTS.md            ← managed section: 8 rules + index       │
  ───────────────┤  CLAUDE.md, GEMINI.md ← one import line: @.blocks/INDEX.md     │
  shell call ──▶ │  .blocks/                                                     │
   hook: hint +  │    json-peek.py …     blocks, one file each                   │
   capture only  │    fixtures/<name>/   inputs the examples need                │
                 │    config.toml        repo policy                             │
                 │    blocks.lock        registry provenance                     │
                 │    INDEX.md           generated                               │
                 └───────────────────────────────────────────────────────────────┘
                 ┌───────────────────────────────────────────────────────────────┐
                 │  state dir (per machine, outside the repo)                    │
                 │  ~/.local/state/caveman-blocks/<repo-id>/                     │
                 │    candidates/<fp>.jsonl   out/<id>.log   stats.jsonl   cache/ │
                 └───────────────────────────────────────────────────────────────┘
                              ▲                      ▲
                   caveman-blocks promote (agent)   verify / lint / sync --check (CI)
```

## Components (Go packages under `internal/`)

| Package | Responsibility | Pure? |
|---|---|---|
| `blockfile` | Parse and write the `# /// block` header; validate; lint rules; content hash | yes |
| `index` | Render `INDEX.md` and the managed section; byte budget; freshness check | yes |
| `hook` | Decision engine: shell command in, hint and events out | yes |
| `hook/protocol` | Generic JSON request and response, versioned | yes |
| `hook/dialect/<name>` | Parse and render one harness JSON shape to the protocol | yes |
| `hook/profiles.toml` | Per-harness data: dialect, events, config path, capabilities, transcripts | data |
| `hook/install` | Insert, remove and report our entry in each config file shape | no |
| `capture` | Script extraction, edit detection, normalization, shape fingerprint, scrub, sightings store | mostly |
| `scan` | Transcript readers per harness; grouping by shape; report | mostly |
| `verify` | Run an example, check the contract, write the stamp | no |
| `registry` | Embedded first-party blocks and fixtures (`go:embed`); `add`; lock file | no |
| `promote` | Rank sightings, render the brief, propose provenance; `retire` | no |
| `runner` | `run`: effects gate from committed config, exec, cap, spill | no |
| `stats` | Append and summarize events | no |
| `repo` | Repo root, `.blocks/`, `config.toml`, state dir, default branch, instruction files | no |

Pure packages take values and return values, hold every decision, and are tested with golden tables.
Impure packages touch the filesystem or processes and stay thin.

## Data flow

**Session start.** Nothing runs. The agent reads its instruction file and sees the managed section or
the import that resolves to it. The section sits in the cached prompt prefix and never changes
mid-session. Which files get what is in [HOOK.md](HOOK.md#instruction-files).

**Shell call.** The harness invokes the hook with the command. The engine applies the rule table in
[HOOK.md](HOOK.md#engine): find `.blocks/` or return; dedupe; extract an inline script; detect edits;
hint when the body matches a block or dumps a structured file; capture a sighting for scripts of ten or
more lines; suggest promotion when shapes repeat. The hook never denies and never rewrites a command.

**Block call.** `caveman-blocks run <name> --param v` loads the header, reads `allow_effects` from the
committed `config.toml` at `HEAD`, refuses `network` and `external` unless allowed, rejects `path`
params outside the repo root, runs `python3 .blocks/<name>.py` from the repo root, caps stdout at 2 KB,
spills the full text to the state dir and records a `run` event with full and returned byte counts.

**Promotion.** See [AGENT-PROMOTION.md](AGENT-PROMOTION.md). The agent writes the block; the CLI checks it.

**Verification.** `verify <name>|--changed|--all` runs each block's `example` from the repo root with
`$FIXTURES` expanded, requires exit 0, exactly one JSON object under 2 KB containing every `returns.keys`
entry, then writes `verified = "<content-hash>"`. A failure writes `state = "quarantined"`. Both writes
are local; the agent or person commits them. In CI, `verify --check` recomputes hashes and reruns
examples, writes nothing, and fails the pull request on any mismatch, failure or disallowed effect.
`--changed` compares against the merge base with the default branch.

**Sync.** `sync` renders the index from blocks whose stamp matches their content, writes `INDEX.md`, the
`AGENTS.md` section and the import lines, enforces the 3 KiB budget, and refreshes exports. `sync --check`
fails in CI when any generated text is stale. Text outside the markers is never touched.

**Scan.** `scan` reads the transcripts each harness keeps, applies the same extraction, edit filter,
scrub and shape fingerprint as the hook, groups by shape and prints the repeat table with the first-party
blocks that would cover each group. Works before anything is installed. Readers are best-effort and
versioned per harness because the formats are documented as internal.

**Add.** `add <name>` copies the embedded block to `.blocks/<name>.py` and its fixtures to
`.blocks/fixtures/<name>/`, strips any `verified` line, records `source`, `version` and the content hash
in `blocks.lock`, runs `verify` and `sync`.

## Formats on disk

- Block file: [FORMAT.md](FORMAT.md).
- `config.toml`: `promote = "commit"`, `allow_effects = []`, `index_max = 25`, `hint = true` (set
  `false` to keep capture and stats but emit no hints). Every key has a default; the file is optional.
- `blocks.lock`: TOML, one table per first-party block: `source`, `version`, `hash` (content hash minus
  stamp lines, so `verify` does not alter it).
- `INDEX.md`: generated; identical to the `AGENTS.md` section.
- State dir layout, sighting schema and scrub list: [HOOK.md](HOOK.md#state-directory).
- `stats.jsonl`: one object per event: `ts`, `session`, `kind` (`script`, `hint`, `promote-hint`,
  `run`), `block`, `fp`, and for `run` events `bytes_full`, `bytes_returned`, `exit`. Only the runner
  writes `run`. "Hint followed" is a `run` of the hinted block later in the same session.

## Install and distribution

- `hooks install` detects harnesses on the machine, copies the binary to a stable path, and writes one
  entry per harness into that harness's user-level configuration. It never writes repo-level hook
  configuration, so a cloned repo cannot install hooks.
- `init` is per repo: `.blocks/`, `config.toml` with defaults, `sync`.
- Channels: GitHub releases with checksums, Homebrew tap, `install.sh`, and `npx caveman-blocks` through
  a shim package with per-platform optional packages. The npm path exists for the first-minute install
  and hands over to the stable native path; decision 0007.

## Performance budget

The hook runs on every shell call. Budget: 5 ms for rule 1, 30 ms for a full decision, measured on a 2020
laptop in CI's `bench-hook`. All `matches` patterns in a repo are compiled per process, so the index cap
is also a latency cap.

## Security boundaries

- The hook binary is installed at user level by the person, never from repo configuration.
- The hook reads `.blocks/` and writes only to the state dir, with `O_NOFOLLOW`; it refuses symlinks
  under `.blocks/`. A cloned repo cannot direct a write.
- Effects are read from committed config at `HEAD`, enforced in the runner, and are hygiene rather than
  a security boundary; see [FORMAT.md](FORMAT.md#effects).
- Captured scripts are scrubbed before storage and shown scrubbed in the promotion brief.
- `verify` refuses disallowed effects, so CI on a fork pull request runs only `read`, `write-workspace`
  and `exec` blocks inside the CI container, under `on: pull_request` with no secrets.
- Nothing is sent anywhere. No telemetry.
