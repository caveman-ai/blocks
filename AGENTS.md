# Caveman Blocks

Go CLI and hook that turns agent-written scripts into verified, reusable blocks in a repo's `.blocks/`.
Thesis: [docs/THESIS.md](docs/THESIS.md). Format: [docs/FORMAT.md](docs/FORMAT.md). Architecture:
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Decisions: [docs/decisions/](docs/decisions/README.md).

## Before editing

- Read the decision that covers the area. Contradicting one needs a superseding record.
- Hook behavior is defined by golden tables in `internal/hook/testdata/`. Change the table first.
- First-party blocks are Python 3.10+ standard library only and must pass `caveman-blocks lint` and `caveman-blocks verify`.

## Checks

`go test ./...` → `make lint` → `make e2e` → `make bench-hook`. Missing tooling means a skipped check,
never a pass.

## Rules

- No model calls in the CLI. No network code outside `internal/registry`, and `net/http` stays out of the binary (hook budget).
- Telemetry sends counts only, through the system `curl`, exactly as [docs/TELEMETRY.md](docs/TELEMETRY.md) lists. A new event field changes that contract first. Never a free-text field.
- Counted numbers are labelled measured. Never print a saving in dollars or percent.
- Scoped conventional commits. PRs to `main`. `CLAUDE.md` is a symlink to `AGENTS.md`.
- Work in a clean branch or worktree off `origin/main` (never commit directly to `main`); stage explicit paths, run the checks above, open a PR, and squash-merge on green CI.

<!-- caveman-blocks:start -->
BLOCKS. Scripts in this repo are reusable blocks in .blocks/. Rules:
1. Check the block index below first. If a block fits, run it. If one almost fits, add a param to it.
2. A script you would run twice, or over ~10 lines, becomes a block, not a heredoc or a /tmp file.
3. Inputs are params. No hardcoded paths, ids, ports or dates.
4. Print one small JSON object: the answer, not the data. Filter, count and truncate in code.
5. Exit 0 on success, non-zero with {"error": ...} on failure.
6. Safe to re-run: idempotent, no prompts, cleans up after itself.
7. Header first: name, summary, params, effects, example. caveman-blocks lint checks it.
8. Compose: call existing blocks with caveman-blocks run instead of copying their code.

grep-defs     --path <path> [--kinds …]         Function, class, type and constant definitions in a file or…
<!-- caveman-blocks:end -->
