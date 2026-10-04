# 0003: One pre-run hook on the shell tool is the only harness integration

Date: 2026-10-03. Status: accepted.

## Context

The spec used three hook events (observe runs, inject at session start, intercept pre-run) plus an interceptor. A hook-free design was considered and rejected because it loses enforcement and automatic capture. The scan shows every agent script passes through the shell tool as a heredoc or inline script, so the pre-run of that tool sees everything.

## Decision

Install exactly one hook per agent, at user level, on the pre-run event of the shell tool. It captures scripts, hints when a script or a structured-file dump matches a block, and denies only exact repeats and disallowed effects. The index reaches the agent through the managed section in AGENTS.md and CLAUDE.md, which agents read natively. Lint and verify run in CI.

## Consequences

- Per-machine footprint: one hook entry per agent. Per-repo footprint: `.blocks/` and a marked section.
- The hook never rewrites a command. An output limiter appended to dumpers was in the first draft and was removed: pipelines hide exit codes, break `cd` tracking and heredocs, and harnesses already cap raw output. Compressing raw output is Caveman Wrap's territory.
- Harness contracts confirmed 2026-10-03 for Claude Code: `PreToolUse` on `Bash` can return allow or deny with a reason plus `additionalContext` in one response; exit 2 denies regardless of JSON; hooks from `~/.claude/settings.json` apply to every project once the folder is trusted. Details in [HOOK.md](../HOOK.md).
- Agents without a pre-run hook (OpenCode, Cline) get the passive layer only.
- Amendment, same day: Cursor, Copilot and Gemini have no context field on their pre-run event. On those three the adapter also registers the post-run event, used only to deliver the hint the pre-run decision already produced. One engine, one decision per call; a second event where the harness forces it. Claude Code and Codex need the pre-run event only. Details in [HOOK.md](../HOOK.md).
- The hook is on the hot path of every shell call: budget 5 ms no-op, 30 ms full decision.
