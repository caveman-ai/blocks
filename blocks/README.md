# First-party registry

Blocks shipped in this repository, embedded in the binary with their fixtures, installed with
`caveman-blocks add <name>`, which copies the block to `.blocks/<name>.py` and its fixtures to
`.blocks/fixtures/<name>/`. Python 3.10+ standard library only, `ruff` clean with defaults. This repo's
CI runs `lint --first-party` and `verify --fixtures-root blocks/fixtures` over `blocks/`; embedded copies
carry no stamp.

| Name | Effects | Returns | Co-occurs with (scan; overlapping categories, one machine, upper bounds) |
|---|---|---|---|
| `json-peek` | read | kind, keys, rows, sample, truncated | 29% of inline scripts load a JSON or JSONL file; `json-peek` replaces the first exploratory look, not every computation |
| `jsonl-stats` | read | counts and sums grouped by a key | Counter and aggregate scripts |
| `test-summary` | exec | passed, failed, first failing test and its trace head, log path | `go test ... \| tail`, `pytest ... \| grep` |
| `first-error` | read | first error line, surrounding context, line number | tail and grep of build and CI logs |
| `wait-for` | read | whether the condition was met, elapsed seconds, last line | `until ... sleep` polling loops |
| `grep-defs` | read | function, type and constant signatures with line numbers | `grep -n "^func \|^type "` |
| `http-json` | network | status, selected keys, size, path of the full body | `curl ... \| python -c "json.load"`. Example uses `file://$FIXTURES/...` so the example itself needs no network; `verify` still skips it until `network` is in `allow_effects` |
| `replace-in-file` | write-workspace | matches found, replaced, dry-run diff head | read, `str.replace`, write heredocs (51%, never captured) with an asserted match count; its `matches` patterns hint on them |

Candidates for the next round, pending scan data from design partners: `sql-peek`, `git-touched`,
`repo-map`, `diff-summary`.
