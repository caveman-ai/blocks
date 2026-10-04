# Contributing

Code and tests define behavior. Rationale lives in `docs/decisions/`. Read the thesis, the format and the
relevant decision before changing behavior; a change that contradicts a decision needs a new decision
record that supersedes it.

## Setup

```
go test ./...          # unit and golden tests, hermetic
make lint              # gofmt, go vet
make e2e               # fixture repo under testdata/, real binary, hooks simulated via stdin
make bench-hook        # hook latency; fails above budget
make blocks-test       # first-party block examples, behaviors and matches samples
make blocks-lint       # ruff over blocks/; skips when neither uvx nor ruff is installed
```

## Rules

- Every hook decision is a row in a golden table under `internal/hook/testdata/`. New behavior adds rows
  first. Dialect conformance fixtures live under `internal/hook/testdata/dialect/<name>/`.
- First-party blocks in `blocks/` are Python 3.10+ standard library only, pass `caveman-blocks lint`, are
  `ruff check` clean with defaults (`make blocks-lint`; not run in CI, so it is on the contributor), carry a
  working `example`, and ship their fixtures under `blocks/fixtures/<name>/`, which `add` copies into user
  repos and CI verifies here.
- No model calls, no network calls in v0, no telemetry.
- Measured numbers are labelled measured. Nothing prints a saving in dollars or percent.
- Commits: conventional, scoped (`feat(hook): …`, `fix(registry): …`). Pull requests to `main`.
- `AGENTS.md` is the single instruction file in this repo. Claude Code reads it when no `CLAUDE.md` exists, and this repo dogfoods its own `sync`, so there is no `CLAUDE.md`.

## Adding a first-party block

1. Write `blocks/<name>.py` following `docs/FORMAT.md`.
2. Add `blocks/fixtures/<name>/` with the input the `example` needs. No network in examples.
3. Run `make blocks-verify`, `make blocks-lint` and `make blocks-test`; add a positive and a negative
   `matches` sample to `MATCH_SAMPLES` in `blocks/tests/test_blocks.py`.
4. Add one line to the registry table in `blocks/README.md`.
5. Open a pull request. Review checks the header, the `matches` patterns against real scripts from the
   scan corpus, and that the output is an answer, not data.
