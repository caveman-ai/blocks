# Harness hook contracts, checked 2026-10-03

Primary sources fetched the same day. Local observations are marked. Unconfirmed items are listed at the end.

## Claude Code (2.1.289)
- Hooks: https://code.claude.com/docs/en/hooks.md, guide: https://code.claude.com/docs/en/hooks-guide.md
- PreToolUse input: `session_id`, `cwd`, `transcript_path`, `permission_mode`, `tool_name`, `tool_input{command, description, timeout, run_in_background}`, `tool_use_id`.
- Output: `hookSpecificOutput{hookEventName, permissionDecision allow|deny|ask|defer, permissionDecisionReason, updatedInput, additionalContext}`; allow + updatedInput + additionalContext combine. `additionalContext` capped at 10,000 chars. Exit 2 denies regardless of JSON; other non-zero codes do not block. Timeout default 600 s. Timed-out hook does not block.
- Config: `~/.claude/settings.json` (all projects), `.claude/settings.json`, `.claude/settings.local.json`; merged. Interactive sessions hold hooks until the folder is trusted. `disableAllHooks` exists; no per-hook disable.
- PostToolUse can replace output via `updatedToolOutput` (`{stdout, stderr, interrupted, isImage}` for Bash) and add `additionalContext`.
- Skills: `.claude/skills/`, `~/.claude/skills/`; `.agents/skills/` not documented. https://code.claude.com/docs/en/skills.md
- AGENTS.md read since 2.1.277 only when no CLAUDE.md exists above cwd, unless `claude-md-and-agents-md` is set. https://code.claude.com/docs/en/memory.md
- Bash tool: separate process per command; `cd` persists inside the project; inline output cap about 30,000 chars; failures about 10,000. https://code.claude.com/docs/en/tools-reference
- Transcripts: `~/.claude/projects/<slug>/<session>.jsonl`, format internal. https://code.claude.com/docs/en/sessions.md

## Codex CLI
- https://developers.openai.com/codex/hooks
- `PreToolUse`, matcher `Bash` covers shell and `exec_command`. Input adds `turn_id`, `tool_name`, `tool_use_id`, `tool_input.command`.
- Output: `hookSpecificOutput.permissionDecision deny` + reason; `allow` + `updatedInput`; `additionalContext` (about 2,500 tokens, `additionalContextLimit`). `ask` rejected. Exit 2 denies.
- Config: `~/.codex/hooks.json` or `[hooks]` in `~/.codex/config.toml` (all projects); project `.codex/hooks.json` when trusted. Each hook trusted by hash via `/hooks`.
- AGENTS.md: yes, root to cwd, 32 KiB. https://learn.chatgpt.com/docs/agent-configuration/agents-md
- Skills: `.agents/skills` up to repo root, `~/.agents/skills`. https://learn.chatgpt.com/docs/build-skills
- Transcripts (local observation): `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` with `function_call` and outputs.

## Cursor
- https://cursor.com/docs/hooks, third-party format: https://cursor.com/docs/reference/third-party-hooks
- `beforeShellExecution` (input `command`, `cwd`): `permission allow|deny|ask`, `user_message`, `agent_message`; no rewrite, no context. `preToolUse`: `updated_input`; CLI support unconfirmed (forum, 2026-01).
- `afterShellExecution`, `postToolUse`: `additional_context`.
- Config: `~/.cursor/hooks.json` (global); project `.cursor/hooks.json` in trusted workspaces. Also loads Claude Code hooks when third-party configs are on (default). Exit 2 denies; other failures fail open unless `failClosed`.
- Instruction files: `AGENTS.md`, `.cursor/rules/*.mdc`; CLI reads root `CLAUDE.md`. Skills: `.agents/skills/`, `.cursor/skills/`, `.claude/skills/`, `.codex/skills/`.
- Transcripts (local observation): `~/.cursor/projects/<slug>/agent-transcripts/<id>/<id>.jsonl`, commands only.

## GitHub Copilot CLI
- https://docs.github.com/en/copilot/reference/hooks-reference
- `preToolUse` (camelCase: `sessionId`, `cwd`, `toolName`, `toolArgs`; PascalCase variant mirrors Claude Code). Output: `permissionDecision allow|deny|ask`, `permissionDecisionReason`, `modifiedArgs`. No context on pre. `postToolUse`: `additionalContext` (10 KB).
- Pre-tool crashes and any non-zero exit fail closed; timeouts fail open. `timeoutSec` default 30.
- Config: `~/.copilot/hooks/*.json` (all repos); repo `.github/hooks/*.json`; also reads repo `.claude/settings*.json`.
- Instruction files: `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`, `.github/instructions/**`. Skills: `.github/skills/`, `.agents/skills/`, `.claude/skills/`.
- Transcripts: `~/.copilot/session-state/<id>/events.jsonl`, commands and results.

## Gemini CLI
- https://geminicli.com/docs/hooks/reference/
- `BeforeTool`, matcher regex on tool name (`run_shell_command`; args `command`, `description`, `dir_path`, `is_background`). Output: `decision allow|deny`, `reason`; rewrite via `hookSpecificOutput.tool_input`; no context on BeforeTool. `AfterTool`: `additionalContext`.
- `timeout` in milliseconds, default 60000. Exit 2 blocks; other codes warn and proceed.
- Config: `.gemini/settings.json` (project), `~/.gemini/settings.json` (user), `/etc/gemini-cli/settings.json`.
- Instruction file: `GEMINI.md`; `AGENTS.md` only via `context.fileName`. Skills: `.gemini/skills/`, `.agents/skills/`.
- Transcripts: `~/.gemini/tmp/<hash>/chats/session-*.json`, inputs and outputs.

## Amp, OpenCode, Cline
- Amp `tool.call` plugin: allow, reject-and-continue, modify. `.amp/plugins/`, `~/.config/amp/plugins/`.
- OpenCode `tool.execute.before` plugin: throw to block, mutate `output.args` to rewrite. `.opencode/plugins/`, `~/.config/opencode/plugins/`.
- Cline SDK plugin `hooks.beforeTool`; return shapes unconfirmed.

## Standards and distribution
- PEP 723 / inline script metadata: https://peps.python.org/pep-0723/, https://packaging.python.org/en/latest/specifications/inline-script-metadata/. Custom types are ignored by tools; uv 0.7.19 confirmed by run, pipx by source.
- Agent Skills: https://agentskills.io/specification; `.agents/skills/` convention: https://agentskills.io/client-implementation/adding-skills-support.md; installer target table: https://github.com/vercel-labs/skills.
- npm per-platform packages: Biome and esbuild layouts (live registry). GoReleaser npm publishing is Pro-only (https://goreleaser.com/customization/publish/npm/); `homebrew_casks` replaces `brews` (https://goreleaser.com/customization/homebrew_casks/); godownloader archived.
- Startup latency, measured here under load: `/usr/bin/true` 10.9 ms median, static Go binary 16.2 ms, `node empty.js` 116.7 ms.

## Unconfirmed
- Claude Code behavior for `updatedInput` without an explicit `permissionDecision`.
- Cursor CLI firing `preToolUse`; Cursor default hook timeout.
- Copilot: exact field for the bash command in camelCase input; PascalCase acceptance of `updatedInput`.
- Codex: whether non-2 non-zero exits fail open on PreToolUse; whether `~/.codex/skills` is still read.
- Cline's current return shapes for blocking and rewriting.
