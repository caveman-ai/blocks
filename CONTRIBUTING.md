# Contributing

Code and tests define behavior. Rationale lives in `docs/decisions/`. Read the thesis, the format and the
relevant decision before changing behavior; a change that contradicts a decision needs a new decision
record that supersedes it.

## Setup

```
go test ./...          # unit and golden tests, hermetic
make lint              # gofmt, go vet, golangci-lint
make e2e               # fixture repo under testdata/, real binary, hooks simulated via stdin
make bench-hook        # hook latency; fails above budget
```

## Rules

- Every hook decision is a row in a golden table under `internal/hook/testdata/`. New behavior adds rows
  first.
- First-party blocks in `blocks/` are Python 3 standard library only, pass `blocks lint`, carry a working
  `example`, and ship a fixture under `testdata/blocks/<name>/` that `blocks verify` runs in CI.
- No model calls, no network calls outside `registry`, no telemetry.
- Measured numbers are labelled measured. Nothing prints a saving in dollars or percent.
- Commits: conventional, scoped (`feat(hook): …`, `fix(registry): …`). Pull requests to `main`.
- `AGENTS.md` and `CLAUDE.md` stay byte-identical; CI checks it.

## Adding a first-party block

1. Write `blocks/<verb-noun>.py` following `docs/FORMAT.md`.
2. Add `testdata/blocks/<verb-noun>/` with the input the `example` needs.
3. Run `blocks lint` and `blocks verify` against the fixture.
4. Add one line to the registry table in `blocks/README.md`.
5. Open a pull request. Review checks the header, the `matches` patterns against real scripts from the
   scan corpus, and that the output is an answer, not data.
