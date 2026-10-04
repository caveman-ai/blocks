# Hook contract

This file is the only rule table. Other documents link here.

One decision engine, `internal/hook`, behind one generic protocol, with one dialect per harness JSON
shape and one data profile per harness (see [INTEGRATION.md](INTEGRATION.md)). The engine is pure: it
takes the command, the repo's blocks and the config, and returns a decision. Harness facts were taken
from official docs on 2026-10-03; URLs in [research/harness-hooks-2026-10-03.md](research/harness-hooks-2026-10-03.md).

## Engine

```go
type Input struct {
    Command string   // the shell command about to run
    Cwd     string
    Session string   // harness session id when provided, else ""
    CallID  string   // harness tool-use id when provided, else ""
}

type Decision struct {
    Hint   string    // one line for the agent, delivered alongside the tool result; "" for none
    Events []Event   // appended to the state dir's stats.jsonl
}
```

v0 has no deny path. The hook allows every command; its outputs are a hint and counted events.
Enforcement of block effects lives in the runner, not here (decision 0006).

Rules, in order. Hints do not stack; the first rule that produces one wins.

| # | Condition | Effect |
|---|---|---|
| 0 | `Phase` is post | Skip every rule; replay the cached decision for this call from `cache/` with events emptied. |
| 1 | No `.blocks/` in `Cwd` or its parents | Return. No events. Budget 5 ms. The nearest parent holding `.blocks/` is the repo root for every later rule. |
| 2 | Same `(Session, Cwd, Command)` seen in the last 2 seconds | Return the cached decision with events emptied. Dedupes harnesses that load two hook configurations for one call. |
| 3 | Command is `caveman-blocks run <name> ...` or `python3 .blocks/<name>.py ...` | Event `call{block}`. Return. This is how "hint followed" gets a session. |
| 4 | Command contains an inline Python script: heredoc to `python`/`python3`, or `python -c`. A leading `bash -lc '...'`/`sh -c '...'` wrapper from the harness is unwrapped first. | Extract the body; continue. Any length. Other languages are not extracted in v0 (blocks are Python-only). |
| 5 | Body is an edit: reads a file, replaces a string or regex, writes the same path | Mark `edit`; continue. Edits are never captured. |
| 6 | Body matches a `matches` pattern of an indexed block, tested against the body. When `edit`, only blocks with `effects = "write-workspace"` are considered. Several hits: longest match wins, then name. | Hint: `Blocks: <name> covers this. Next time: caveman-blocks run <name> --<param> ...` using the block's required params. Event `hint{block, fp}`. Skip rule 7. |
| 7 | Body is 10+ lines and not `edit` | Capture one sighting to the state dir (below). Event `script{fp, lines}`. |
| 8 | Command reads a structured file whole: `cat`/`less`/`more`/`bat` of `.json`, `.jsonl`, `.ndjson`, `.log` | Hint naming the indexed block among `json-peek`, `jsonl-stats`, `first-error` that fits. If none is installed, hint `caveman-blocks add <name>` once per session. Event `hint`. |
| 9 | Evaluated on every call after the rules above: a shape with sightings from 2+ sessions in the last 14 days that no block's `provenance.source` names, or 5+ such shapes | Append ` Run caveman-blocks promote when the task is done.` to the hint, or emit it alone, at most once per shape per day. Event `promote-hint`. |

With `hint = false` in `config.toml` no rule emits a hint, and a rule 6 match no longer skips rule 7:
the body falls through to capture.

The hook reads `config.toml` and the block headers from the working tree, never through `git`, to stay
inside the budget. Only the runner's effects gate reads committed policy. It does not hash blocks
either: for rules 6 and 8 a block counts as indexed when its `[stamp].verified` is non-empty and it has
no `state`. That is a cheap approximation; `sync` and `verify` compute the real content hash.

Two identities per body. `script_sha` is SHA-256 of the body with string literals, numbers and
path-like tokens stripped and whitespace collapsed; it says "the same script again" and feeds the
literal vector in a sighting. `fp`, the shape fingerprint, is the first 12 hex of SHA-256 over
`"py|" + sorted import names + call names with counts bucketed to 1, 2, 3+`, where call names come from
a fixed table committed at `internal/capture/callnames.go` (`json.load`, `json.loads`, `json.dump`,
`json.dumps`, `open`, `print`, `re.sub`, `re.findall`, `re.search`, `Counter`, `glob`, `os.walk`, `subprocess`,
`sys.argv`, `argparse`, `urllib`, `requests`, `csv`, `yaml`, `pathlib`, `sqlite3`, `psycopg`,
`time.sleep`). A body with no imports and no table hits has no `fp` and is not captured. The table is
data; changing it is a golden-table change.

## State directory

The hook writes nothing inside the repository. All runtime state lives under
`$XDG_STATE_HOME/caveman-blocks/<repo-id>/` (default `~/.local/state/caveman-blocks/<repo-id>/`), where
`<repo-id>` is the first 16 hex of SHA-256 of `git rev-parse --path-format=absolute --git-common-dir`
run at the repo root, so every worktree of one repository shares state, falling back to the root's
absolute path outside git:

```
candidates/<fp>.jsonl   one line per sighting: {ts, session, script_sha, lines, literals: [...], command_head}
out/<id>.log            full block outputs written by the runner
stats.jsonl             counted events
hook.log                internal errors, rate limited to one line per minute
cache/                  per-call decisions for dedupe and post-run replay
```

The runner prunes `out/` entries older than 7 days and `cache/` entries older than 1 hour, at most once
a day, on its own invocations. Sightings in `candidates/` older than 14 days are pruned by the same daily run, and a stored
script is cut at 16 KiB.

Files are opened with `O_APPEND|O_NOFOLLOW` and created `0600`. A cloned repository therefore cannot
point the hook at a file of its choosing: the hook only reads `.blocks/`, and it refuses to read through
a symlink there. The scripts stored as sightings are scrubbed first with a golden-tested list: URL
userinfo, `Authorization` and `Cookie` header values, assignments to names containing `KEY`, `TOKEN`,
`SECRET`, `PASSWORD`, known token prefixes (`sk-`, `ghp_`, `xox`, `AKIA`), and any token over 32
characters with high entropy. Scrubbed spans are replaced by `«scrubbed»`.

## Failure policy

The hook must never break a session. The binary exits 0 in every reachable case and expresses
everything through JSON, because Copilot treats any non-zero exit on its pre-tool hook as a denial of
the agent's command. Internal errors go to `hook.log` and the decision is empty. The hook reads at most
1 MiB of stdin, and a 2-second watchdog answers `{}` and exits 0 if a decision has not been written by
then. Configured timeouts are 5 seconds everywhere; the engine's own budget is 30 ms for a full decision
and 5 ms for rule 1.

`hooks install` copies the binary to `~/.local/share/caveman-blocks/bin/caveman-blocks` and writes that
absolute path into the harness configuration. It refuses to write a path under an npm or npx cache,
which is garbage collected and would leave a dangling hook. `init`, `sync` and `doctor` compare the
copy's version with the invoking binary and refresh it by writing a temp file and renaming, never in
place, so a running hook is not disturbed. `doctor` reports version skew and a path that no longer
resolves.

## Delivery of the hint

Claude Code and Codex accept `additionalContext` on an allowed pre-run call and deliver it alongside
the tool result. Cursor, Copilot and Gemini have no context field on their pre-run event, only on the
post-run one. The hint arrives at the same moment on every harness, with the tool result. On the latter
three the adapter registers a second, post-run event that replays the pre-run decision from
`cache/` keyed by `CallID`, or by `hash(Session, Cwd, Command)` when the harness gives no id. Rule 2's
dedupe returns the cached decision rather than an empty one, so whichever of two loaded hook
configurations runs first, the post-run event finds a hint to deliver. Codex's `tool_input.command`
shape (string or argv) is pinned by a conformance fixture, and rule 4 unwraps the harness's own shell
wrapper before looking for a script.

## Adapters

Adapters are dialect plus profile; see [INTEGRATION.md](INTEGRATION.md).

| Harness | Pre-run event, matcher | Hint on pre-run | Post-run event (hint only) | User-level config | Phase | Notes |
|---|---|---|---|---|---|---|
| Claude Code | `PreToolUse`, `"matcher": "Bash"` | `hookSpecificOutput.additionalContext` | not needed | `~/.claude/settings.json` | 1 | Hooks wait for workspace trust in interactive sessions. Command at `tool_input.command`. |
| Codex CLI | `PreToolUse`, matcher `Bash` | `additionalContext` (about 2,500 tokens cap) | not needed | `~/.codex/hooks.json` | 1 | Each new or changed hook is trusted once in `/hooks`; `install` prints that step. |
| Cursor | `beforeShellExecution` | none | `afterShellExecution` → `additional_context` | `~/.cursor/hooks.json` | 1 | `preToolUse` is not reliable in the CLI. The pre-run response is `{}`, never `permission: allow`, so Cursor's own approval flow is untouched. Cursor also loads Claude Code hook files by default; rule 2 dedupes. |
| Copilot CLI | `preToolUse`, tool `bash` | none | `postToolUse` → `additionalContext` | `~/.copilot/hooks/blocks.json` | 2 | Non-zero exit on pre denies. Always exit 0. `timeoutSec: 5`. |
| Gemini CLI | `BeforeTool`, matcher `run_shell_command` | none | `AfterTool` → `additionalContext` | `~/.gemini/settings.json` | 2 | `timeout` in milliseconds: `5000`. |
| Amp, OpenCode, Cline | plugin APIs | | | | later | Passive layer only; optional shims under `integrations/`. |

`hooks install` never touches project-level hook files. `hooks status` shows which harnesses are
installed and, for Codex, whether the hook is trusted. `hooks uninstall` removes exactly the entries it
wrote, identified by a marker key.

## Instruction files

`sync` is driven by which files exist at the repo root, not by the harness profile. It writes the
managed section once, into `AGENTS.md`, creating the file if needed. Where `CLAUDE.md` or `GEMINI.md`
exist at the root, it adds one import line, `@.blocks/INDEX.md`, between the same marker comments
instead of a copy, because Claude Code reads `AGENTS.md` only when no `CLAUDE.md` exists and Gemini reads
`GEMINI.md` by default; both document `@` imports, pinned by a fixture in phase 1. `INDEX.md` holds the
full section body, byte-identical to the text between the `AGENTS.md` markers. `sync` never creates a
`CLAUDE.md`; `doctor` warns when a `CLAUDE.md` exists in a parent directory but not at the root, since
Claude Code would then read neither file, and suggests a root `CLAUDE.md` containing `@AGENTS.md`.
Harnesses that read several of these files see one copy and one or two import lines. `section = "import"`
(FORMAT.md) covers repos with hard size caps.

| Harness | Reads by default |
|---|---|
| Claude Code | `CLAUDE.md`; `AGENTS.md` only when no `CLAUDE.md` exists anywhere above cwd |
| Codex CLI | `AGENTS.md`, root to cwd, 32 KiB cap |
| Cursor | `AGENTS.md`, `.cursor/rules/*.mdc`; CLI also reads root `CLAUDE.md` |
| Copilot CLI | `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md` |
| Gemini CLI | `GEMINI.md`; `AGENTS.md` only if configured |

## Transcripts for `scan`

| Harness | Location | Commands | Output |
|---|---|---|---|
| Claude Code | `~/.claude/projects/<slug>/<session>.jsonl` (+ `subagents/`) | yes | yes |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | yes | yes |
| Cursor | `~/.cursor/projects/<slug>/agent-transcripts/<id>/<id>.jsonl` | yes | no |
| Copilot CLI | `~/.copilot/session-state/<id>/events.jsonl` | yes | yes |
| Gemini CLI | `~/.gemini/tmp/<hash>/chats/session-*.json` | yes | yes |

Every harness documents these formats as internal and unstable. Readers are best-effort and report what
they could not parse. Capture through the hook is the stable source once installed.
