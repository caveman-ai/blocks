# Integrating a coding agent

Adding a harness must be an afternoon, not a project. The design gets there with four layers, each with
one job, and a data-driven profile so that most new harnesses are a config entry rather than code.

```
  harness ──stdin──▶ [dialect: parse] ──▶ [generic protocol] ──▶ [engine] ──▶ [generic protocol] ──▶ [dialect: render] ──stdout──▶ harness
                                                                   ▲
                                              profile: events, config path, capabilities, timeout unit
```

## Layer 1: the generic protocol

The binary speaks one normalized JSON contract on `caveman-blocks hook --harness generic`. Every native adapter is a
translation to and from it. Any agent, plugin or wrapper that can run a process and pass JSON can integrate
by speaking this directly, with no code in this repository.

Request:

```json
{ "protocol": 1, "phase": "pre", "harness": "myagent", "session": "abc", "call_id": "t1",
  "cwd": "/repo", "command": "python3 - <<'EOF' ... EOF", "output": null }
```

Response:

```json
{ "protocol": 1, "action": "allow", "reason": "", "hint": "Blocks: json-peek covers this. Next time: caveman-blocks run json-peek --path x.json" }
```

- `phase` is `pre` or `post`. `output` is set only on `post` and only by harnesses that provide it.
- `action` is `allow` in v0; `deny` is reserved (decision 0010). `reason` accompanies a deny. `hint` is one line or empty.
- The response never contains a rewritten command. Decision 0003.
- Unknown fields are ignored on input and never emitted on output. `protocol` is bumped only for a
  breaking change, and the binary answers the previous version for one major release.

## Layer 2: dialects

A dialect is a parser and renderer between a harness's JSON and the generic protocol. There are fewer
dialects than harnesses because several harnesses accept the same shapes:

| Dialect | Used by | Pre-run deny | Pre-run hint | Post-run hint |
|---|---|---|---|---|
| `claude` | Claude Code, Codex CLI, Cursor (third-party format), Copilot (PascalCase form) | `hookSpecificOutput.permissionDecision: "deny"` + `permissionDecisionReason` | `hookSpecificOutput.additionalContext` | `hookSpecificOutput.additionalContext` |
| `cursor` | Cursor native | `permission: "deny"` + `agent_message` | none | `additional_context` |
| `copilot` | Copilot camelCase | `permissionDecision: "deny"` + `permissionDecisionReason` | none | `additionalContext` |
| `gemini` | Gemini CLI | `decision: "deny"` + `reason` | none | `hookSpecificOutput.additionalContext` |
| `generic` | anything else | `action` | `hint` | `hint` |

Dialects live in `internal/hook/dialect/<name>` and each is under 150 lines: a struct for the input, a
struct for the output, two functions. Conformance fixtures under `internal/hook/testdata/dialect/<name>/` hold
real request and response pairs taken from the harness docs, so a dialect change that breaks a harness
fails a test here, not in a user's session.

## Layer 3: profiles

A profile is data, embedded in the binary as `internal/hook/profiles.toml`, one table per harness:

```toml
[claude-code]
dialect = "claude"
detect = ["~/.claude"]
config = "~/.claude/settings.json"
config_format = "claude-settings"          # how to merge our entry into the file
pre_event = { name = "PreToolUse", matcher = "Bash" }
post_event = {}                             # empty: pre-run hint is enough
timeout = { value = 5, unit = "s" }
command_field = "tool_input.command"
capabilities = ["pre-hint", "transcripts"]
instruction_files = ["AGENTS.md", "CLAUDE.md"]   # AGENTS.md gets the section; CLAUDE.md gets @.blocks/INDEX.md
skills_dir = ".claude/skills"
transcripts = "~/.claude/projects/*/*.jsonl"
trust_note = "Hooks run after the folder is trusted in an interactive session."

[cursor]
dialect = "cursor"
detect = ["~/.cursor"]
config = "~/.cursor/hooks.json"
config_format = "cursor-hooks"
pre_event = { name = "beforeShellExecution" }
post_event = { name = "afterShellExecution" }
timeout = { value = 5, unit = "s" }
command_field = "command"
capabilities = ["post-hint", "transcripts-commands-only"]
instruction_files = ["AGENTS.md"]
skills_dir = ".agents/skills"
transcripts = "~/.cursor/projects/*/agent-transcripts/*/*.jsonl"
```

`hooks install`, `hooks status`, `export` and `scan` read the profile and nothing else; `sync` is driven by which instruction files exist (HOOK.md). Adding a
harness that speaks an existing dialect and an existing config format is one table. A harness with a new
config file shape needs one `config_format` writer, a small function that knows how to insert and remove
our entry idempotently, identified by a marker key.

## Layer 4: capability tiers

The engine does not know which harness it serves. The profile's capabilities decide what the user gets:

| Tier | Capabilities | What works | Harnesses | Phase |
|---|---|---|---|---|
| A | `pre-hint` | everything, one event | Claude Code, Codex CLI | 1 |
| B | `post-hint` | everything, two events | Cursor (1), Copilot CLI (2), Gemini CLI (2) | 1–2 |
| C | none | rules, index, exports; no capture, no hints | Amp, OpenCode, Cline, anything that reads AGENTS.md | always |

Tier C is not a degraded mode to apologize for. It is the passive layer every harness gets, and it is
what makes the repo useful to a teammate who has not installed anything.

## Plugin shims for Tier C harnesses

Amp, OpenCode and Cline expose plugin APIs rather than file hooks. Moving one to Tier A or B is a shim
of about 20 lines in the harness's plugin language that forwards the call to `caveman-blocks hook
--harness generic` and maps the response. Reference shims will live under `integrations/<harness>/` with their own
conformance fixture. They are optional and separately versioned; the binary never depends on them.

## The checklist for a new harness

1. Read the harness's hook docs. Fill in a profile table. If its JSON matches an existing dialect, stop
   here: open a pull request with the table and a conformance fixture from the docs.
2. If not, write the dialect: two structs, two functions, fixtures.
3. If its config file shape is new, write the `config_format` writer with install, uninstall and status.
4. Add the transcript reader to `internal/scan/reader/<harness>` if transcripts exist and are useful.
5. Run `make e2e`, which replays every conformance fixture through the real binary.
6. Add a row to the table in [HOOK.md](HOOK.md) and the research record with the doc URLs and date.

## What stays out of the adapters

- No decision logic. If a harness needs special behavior, it is a capability flag the engine reads, never
  a branch on the harness name.
- No command rewriting, even where the harness allows it.
- No harness-specific state. The pre-to-post cache in the state dir is keyed by `call_id`, or by hash(session, cwd, command) when the harness gives no id, and shared.
- No network. Adapters read stdin, write stdout, and touch exactly one config file on install.
