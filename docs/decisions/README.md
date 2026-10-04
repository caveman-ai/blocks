# Decisions

Architecture decision records. One file per decision, numbered, never edited after acceptance; a later
record supersedes an earlier one. Code and tests define behavior; these records hold the rationale.

| # | Decision | Status |
|---|---|---|
| [0001](0001-registry-first-mining-later.md) | Curated first-party blocks first, repo mining later | accepted |
| [0002](0002-single-file-block-format.md) | A block is one file with a `# /// block` header | accepted |
| [0003](0003-one-pre-run-hook.md) | One pre-run hook on the shell tool is the harness integration (post-run event only to carry the hint where the harness requires it) | accepted |
| [0004](0004-agent-promotes.md) | The session agent promotes; the CLI never calls a model | accepted |
| [0005](0005-counted-not-inferred.md) | Savings are counted events, not inferred lift | accepted |
| [0006](0006-effects-declared-enforced-at-call.md) | Side effects are declared in the header and enforced at the call site, no sandbox | accepted |
| [0007](0007-go-single-binary.md) | Go, one static binary, npm shim for `npx` | accepted |
| [0008](0008-independent-of-caveman-wrap.md) | Independent of Caveman Wrap: own installer, adapters and CLI | accepted |
| [0009](0009-blocks-stay-in-repo.md) | User blocks never leave the repo; only the first-party registry is public | accepted |
