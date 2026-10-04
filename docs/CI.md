# CI for a repo that uses Blocks

`init` does not write a workflow. It prints this snippet, and `doctor` checks for it.

```yaml
# .github/workflows/blocks.yml
on: pull_request
jobs:
  blocks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - run: curl -fsSL https://caveman.so/blocks/install.sh | sh
      - run: caveman-blocks lint .blocks
      - run: caveman-blocks verify --check --all --policy-ref origin/${{ github.base_ref }}
      - run: caveman-blocks sync --check
```

No secrets. The policy for `allow_effects` comes from the base branch, so a pull request cannot grant
itself `network` or `external`.

| Situation | `verify --check` result |
|---|---|
| Stamp matches, example passes | pass |
| Stamp missing or stale | fail: run `caveman-blocks verify` locally and commit |
| Example fails | fail |
| Committed block with `state = "quarantined"` | fail: fix and re-verify, or `retire` |
| Effect not allowed by the base branch | skip, listed |
| `requires` binary missing on the runner | skip, listed |
| Generated text stale (`sync --check`) | fail |

Pass with skips is a pass. The listing of skips is the signal that a block needs a grant or a tool on
the runner.

## This repository's own CI

First-party blocks in `blocks/` ship without a `[stamp]` table, because `add` writes the stamp in the
user's repo. `make blocks-verify` therefore runs `lint --first-party blocks` and
`verify --check --all --first-party --fixtures-root blocks/fixtures --blocks-dir blocks`, where
`--first-party` treats a block with no stamp as stamped with its current hash, so the example alone
decides pass or fail, and writes nothing. Outside `--first-party` a missing stamp is stale, as in the
table above. No `.blocks/config.toml` exists here, so `http-json` (`network`) is a listed skip. CI also
runs `make e2e` and `make bench-hook`; the latter skips rule 1 when spawning `/usr/bin/true` alone
takes over half its 5 ms budget, and skips entirely above 20 ms.
