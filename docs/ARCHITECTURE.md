# Architecture

One static binary, `caveman-blocks` (alias `blocks`), used three ways: as a CLI by people and agents, as a
pre-run hook by the agent harness, and as a CI step. No daemon, no service, no model calls, no network
except `add`, `update` and `diff` against the first-party registry.

```
                 ┌──────────────────────────────────────────────────────────────┐
                 │  repo                                                        │
  agent session  │  AGENTS.md / CLAUDE.md   ← managed section: 8 rules + index  │
  ───────────────┤  .blocks/                                                    │
  shell call ──▶ │    json-peek.py …        committed blocks, one file each     │
   pre-run hook  │    blocks.lock           registry provenance                 │
   (allow/hint/  │    config.toml           repo policy                         │
    deny/rewrite)│    INDEX.md              generated                           │
                 │    .candidates/          gitignored: captured scripts        │
                 │    .out/                 gitignored: full outputs            │
                 │    .stats.jsonl          gitignored: counted events          │
                 └──────────────────────────────────────────────────────────────┘
                              ▲                      ▲
                     blocks promote (agent)    blocks verify / lint (CI)
```

## Components (Go packages under `internal/`)

| Package | Responsibility | Pure? |
|---|---|---|
| `blockfile` | Parse and write the `# /// block` header; validate; lint rules | yes |
| `index` | Render `INDEX.md` and the managed section in AGENTS.md / CLAUDE.md; freshness check | yes |
| `hook` | Decision engine: shell command in, decision out (allow, hint, deny, rewrite) | yes |
| `hook/dialect/<name>` | Parse and render one harness JSON shape to the generic protocol | yes |
| `hook/profiles.toml` | Per-harness data: events, config path, capabilities, transcripts | data |
| `hook/install` | Insert and remove our entry in each config file shape | no |
| `capture` | Script extraction from a shell command, edit-script filter, shape fingerprint, candidates store | mostly |
| `scan` | Transcript readers per agent; grouping by shape; report | mostly |
| `verify` | Run a block's example at HEAD, check the contract, write the stamp | no |
| `registry` | Embedded first-party blocks; `add`, `diff`, `update`; lock file | no |
| `promote` | Build the promotion brief for the agent; headless runners | no |
| `stats` | Append and summarize counted events | no |
| `runner` | `blocks run`: execute a block, cap stdout, write full output, enforce declared effects | no |

"Pure" packages take values and return values. They hold every decision and are tested with golden
tables. Impure packages touch the filesystem, processes or the network and stay thin.

## Data flow

**Session start.** Nothing runs. The agent reads its instruction file as it always does and sees the
managed section: the eight rules and the index, one line per block, at most 40 lines, in a fixed order.
`blocks sync` writes the identical section between marker comments into every instruction file present
at the repo root (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`, a
`.cursor/rules/blocks.mdc`) and creates `AGENTS.md` when none exists. Claude Code reads `AGENTS.md` only
when no `CLAUDE.md` exists, which is why every file present gets the section. It sits in the cached
prompt prefix and never changes mid-session.

**Shell call.** The harness invokes the hook with the command on stdin. The engine runs, in order:

1. No `.blocks/` in cwd or any parent → allow, exit. Target: under 5 ms.
2. Extract an inline script (heredoc, `python -c`, `node -e`, `bash -c`) if present.
3. Edit-script filter: a script that reads a file, replaces text and writes it back is an edit, not a
   procedure. Allow, do not capture.
4. Script of ten or more lines → save to `.candidates/` with its command line and a shape fingerprint.
5. Shape fingerprint matches a block's declared `matches` patterns → allow with one line of context:
   which block, and the call to use next time.
6. Normalized script hash equals a promoted block's → deny with the block call. Exact repeats only.
7. `blocks run <name>` where the block declares `network` or `external` effects and the repo config does
   not allow them → deny with the config line to add.
8. Read-only dumper of a structured file (`cat x.json`, `cat x.jsonl`, `cat x.log`, `tail -n 500 x.log`)
   → allow with a hint naming the block that returns an answer instead (`json-peek`, `jsonl-stats`,
   `first-error`). Never a rewrite.
9. Append one counted event to `.stats.jsonl`.

The engine never rewrites a command. Appending `| head` was considered and rejected: a pipeline hides the
producer's exit code, runs `cd` in a subshell so the harness loses directory tracking, kills producers
with SIGPIPE, and breaks heredocs and background runs. Harnesses already cap raw output (Claude Code at
about 30,000 characters inline). Compressing raw command output is Caveman Wrap's job and stays there;
Blocks replaces the script, not the stream.

**Block call.** `blocks run <name> --param v` executes the block, caps stdout at 2 KB, writes the full
output to `.blocks/.out/<id>.log` and appends the path to the answer. Blocks call other blocks the same
way. Exit code passes through.

**Promotion.** See [AGENT-PROMOTION.md](AGENT-PROMOTION.md). The agent writes the block file; the CLI
checks it.

**Verification.** `blocks verify [--changed]` parses each block, runs `example` at HEAD, requires exit 0,
parseable JSON on stdout and under 2 KB, then writes `verified = "<sha> <date>"` into the header. A failure
writes `state = "quarantined"`, which removes the block from the index. CI runs it on every pull request.

**Scan.** `blocks scan` reads the local transcripts each agent already keeps, applies the same extraction
and edit filter as the hook, groups by shape, and prints the repeat table with the first-party blocks
that would cover each group. Works before anything is installed. Transcript formats are internal to each
harness and change between releases, so readers are best-effort, versioned per harness, and fail soft
with a count of entries they could not parse. The hook's own capture is the stable path.

## Formats on disk

- Block file: see [FORMAT.md](FORMAT.md).
- `config.toml`: `promote = "commit" | "pr"`, `allow_effects = []`, `index_max = 40`, `hint = true`.
  Every key has a default; the file is optional.
- `blocks.lock`: TOML, one table per first-party block: `source`, `version`, `sha256`.
- `.candidates/<fingerprint>.<ext>` plus `<fingerprint>.json` sidecar: command line, cwd relative to
  repo root, shape, first and last seen, count.
- `.stats.jsonl`: one JSON object per event: `ts`, `kind` (`script`, `hint`, `deny`, `run`), `block`,
  and for `run` events `bytes_full` and `bytes_returned`. Local only.

## Install and distribution

- `blocks hooks install` detects the agents present on the machine and writes one pre-run hook entry
  per agent into that agent's user-level configuration, pointing at the absolute path of the native
  binary. It never writes repo-level hook configuration, so a cloned repo cannot install hooks.
- `blocks init` is per repo: creates `.blocks/`, the gitignore lines, `config.toml` with defaults, and
  runs `sync`.
- Channels: GitHub releases with checksums, Homebrew tap, `install.sh`, and `npx caveman-blocks` through
  a shim package with per-platform optional packages. The npm path exists for the first-minute install
  and immediately hands over to the native binary; see decision 0007.

## Performance budget

The hook runs on every shell call. Budget: 5 ms for the no-op path, 30 ms for a full decision, measured
on a 2020 laptop. Regex compilation is done at build time where possible and cached otherwise. The
`matches` patterns of all blocks in a repo are compiled once per process and the process is short-lived,
so the index must stay small; this is a second reason for the 40-block cap.

## Security boundaries

- The hook binary is installed at user level by the person, never from repo config. A cloned repo cannot
  make the hook do anything except read its `.blocks/` folder.
- `.blocks/` is data. The runner executes blocks only through declared entry points.
- Declared `network` and `external` effects are denied by default; the repo opts in via `config.toml`,
  which is itself reviewed in pull requests.
- Capture scrubs values of environment-looking assignments and high-entropy tokens before writing a
  candidate. Candidates are gitignored regardless.
- Nothing is sent anywhere. There is no telemetry in v0.
