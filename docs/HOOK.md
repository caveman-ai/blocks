# Hook contract

One decision engine, `internal/hook`, and one thin adapter per harness. The engine is pure: it takes the
command string, the repo state and the config, and returns a decision. Adapters translate stdin and
stdout and know where each harness keeps its configuration. Harness facts below were taken from the
official docs on 2026-10-03; URLs in [research/harness-hooks-2026-10-03.md](research/harness-hooks-2026-10-03.md).

## Engine

```go
type Input struct {
    Command string   // the shell command about to run
    Cwd     string
    Session string   // harness session id, for provenance counts only
    Phase   Phase    // Pre or Post
    Output  string   // Post only: captured stdout, when the harness provides it
}

type Decision struct {
    Action  Action   // Allow, Deny
    Reason  string   // Deny: shown to the agent. Allow: debug log only
    Hint    string   // one line for the agent, delivered alongside the tool result
    Events  []Event  // appended to .stats.jsonl
}
```

Rules, in order; the first matching rule that produces a deny ends evaluation, hints accumulate to one
line at most:

| # | Condition | Action |
|---|---|---|
| 1 | No `.blocks/` in `Cwd` or its parents | Allow, no events, return immediately |
| 2 | Command contains an inline script of 10+ lines (heredoc to `python`, `python3`, `node`, `bash`, `sh`; `python -c`; `node -e`) | Extract body; continue |
| 3 | Body is an edit (reads a file, replaces a string or regex, writes the same file) | Allow, no capture |
| 4 | Body's normalized hash equals the source hash of a promoted block | Deny: `"<name> does this. Run: blocks run <name> <args>"` |
| 5 | Body matches a block's `matches` pattern | Allow; hint `"Blocks: <name> covers this. Next time: blocks run <name> ..."`; capture |
| 6 | 10+ line body, no match | Allow; capture to `.candidates/` |
| 7 | Command is `blocks run <name>` and the block's `effects` is `network` or `external` and not in `allow_effects` | Deny: `"<name> declares <effect>. Add it to allow_effects in .blocks/config.toml to permit."` |
| 8 | Command dumps a structured file (`cat`/`head`/`tail` of `.json`, `.jsonl`, `.log`, `.csv`) | Allow; hint naming the block that returns an answer |
| 9 | `.candidates/` holds a shape seen in 2+ sessions, or 5+ candidates, and no promote hint in the last hour | Append `"Run blocks promote when the task is done."` to the hint |

The engine never rewrites a command. See decision 0003 for why.

Normalization for rule 4: strip string literals, numbers and paths, collapse whitespace, hash. Shape for
rules 5 and 9: sorted imports plus a fixed set of call names, the same fingerprint `blocks scan` uses.

## Failure policy

The hook must never break a session. The binary exits 0 in every case it can reach and expresses deny
through JSON, because Copilot treats any non-zero exit on its pre-tool hook as deny and the others treat
most non-zero exits as a failed hook. Internal errors are logged to `.blocks/.hook.log`, rate limited,
and the decision is Allow with no hint. Configured timeouts are 5 seconds everywhere; the engine's own
budget is 30 ms.

## Delivery of the hint

Claude Code and Codex accept `additionalContext` on an allowed pre-run call and deliver it alongside the
tool result. Cursor, Copilot and Gemini have no context field on their pre-run event, only on the
post-run one. The hint is therefore delivered at the same moment on every harness, with the tool result,
but the adapter for the last three registers a second, post-run event to carry it. The engine runs once
per call; the post-run adapter replays the pre-run decision from a 60-second cache keyed by tool-use id
rather than re-evaluating.

## Adapters

| Harness | Pre-run event, matcher | Deny | Hint | Post-run event (hint only) | User-level config | Notes |
|---|---|---|---|---|---|---|
| Claude Code | `PreToolUse`, `"matcher": "Bash"` | `hookSpecificOutput.permissionDecision: "deny"` + `permissionDecisionReason` | `hookSpecificOutput.additionalContext` on allow | not needed | `~/.claude/settings.json` | Hooks wait for workspace trust in interactive sessions. Command at `tool_input.command`. |
| Codex CLI | `PreToolUse`, matcher `Bash` (covers `exec_command`) | same shape as Claude Code | `additionalContext` on allow, about 2,500 tokens cap | not needed | `~/.codex/hooks.json` | Every new or changed hook must be trusted once in `/hooks`; `install` prints that step. |
| Cursor | `beforeShellExecution` | `permission: "deny"` + `agent_message` | none on pre | `afterShellExecution` → `additional_context` | `~/.cursor/hooks.json` | `preToolUse` is not reliable in the CLI; `beforeShellExecution` is. Set `failClosed: false`. |
| Copilot CLI | `preToolUse`, tool `bash` | `permissionDecision: "deny"` + `permissionDecisionReason` | none on pre | `postToolUse` → `additionalContext` (10 KB cap) | `~/.copilot/hooks/blocks.json` | Non-zero exit on pre denies. Always exit 0. `timeoutSec: 5`. |
| Gemini CLI | `BeforeTool`, matcher `run_shell_command` | `decision: "deny"` + `reason` | none on pre | `AfterTool` → `additionalContext` | `~/.gemini/settings.json` | `timeout` is milliseconds: `5000`. Instruction file is `GEMINI.md` unless `context.fileName` adds `AGENTS.md`. |
| Amp, OpenCode, Cline | plugin APIs, not file hooks | | | | | Passive layer only in v0: managed section plus export. |

`blocks hooks install` writes the entry with the absolute path of the native binary and an adapter flag,
for example `/usr/local/bin/caveman-blocks hook --harness claude`. It never touches project-level hook
files. `blocks hooks status` shows which harnesses are installed and, for Codex, whether the hook is
trusted. `blocks hooks uninstall` removes exactly the entries it wrote, identified by a marker comment or
key.

## Instruction files for the managed section

| Harness | Reads by default |
|---|---|
| Claude Code | `CLAUDE.md`; `AGENTS.md` only when no `CLAUDE.md` exists anywhere above cwd |
| Codex CLI | `AGENTS.md`, root to cwd, 32 KiB cap |
| Cursor | `AGENTS.md`, `.cursor/rules/*.mdc`; CLI also reads root `CLAUDE.md` |
| Copilot CLI | `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md` |
| Gemini CLI | `GEMINI.md`; `AGENTS.md` only if configured |

`blocks sync` writes the section into every one of these that exists at the repo root and creates
`AGENTS.md` if none does. The section is identical everywhere and bounded by
`<!-- caveman-blocks:start -->` and `<!-- caveman-blocks:end -->`.

## Transcripts for `blocks scan`

| Harness | Location | Commands | Output |
|---|---|---|---|
| Claude Code | `~/.claude/projects/<slug>/<session>.jsonl` (+ `subagents/`) | yes | yes |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | yes | yes |
| Cursor | `~/.cursor/projects/<slug>/agent-transcripts/<id>/<id>.jsonl` | yes | no |
| Copilot CLI | `~/.copilot/session-state/<id>/events.jsonl` | yes | yes |
| Gemini CLI | `~/.gemini/tmp/<hash>/chats/session-*.json` | yes | yes |

Every harness documents these formats as internal and unstable. Readers are best-effort and report what
they could not parse. Capture through the hook is the stable source once installed.
