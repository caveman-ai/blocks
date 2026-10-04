# Caveman Blocks

Go CLI and hook that turns agent-written scripts into verified, reusable blocks in a repo's `.blocks/`.
Thesis: [docs/THESIS.md](docs/THESIS.md). Format: [docs/FORMAT.md](docs/FORMAT.md). Architecture:
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Decisions: [docs/decisions/](docs/decisions/README.md).

## Before editing

- Read the decision that covers the area. Contradicting one needs a superseding record.
- Hook behavior is defined by golden tables in `internal/hook/testdata/`. Change the table first.
- First-party blocks are Python 3 standard library only and must pass `blocks lint` and `blocks verify`.

## Checks

`go test ./...` → `make lint` → `make e2e` → `make bench-hook`. Missing tooling means a skipped check,
never a pass.

## Rules

- No model calls in the CLI. No network outside `internal/registry`. No telemetry.
- Counted numbers are labelled measured. Never print a saving in dollars or percent.
- Scoped conventional commits. PRs to `main`. `AGENTS.md` and `CLAUDE.md` byte-identical.
