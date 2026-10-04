# First-party registry

Blocks shipped in this repository, embedded in the binary, installed with `blocks add <name>`. Python 3
standard library only. Each has a fixture under `testdata/blocks/<name>/` that CI verifies.

| Name | Effects | Returns | Replaces (from the scan) |
|---|---|---|---|
| `json-peek` | read | keys, row count, one sample, truncated flag | 29% of inline scripts: load a JSON or JSONL file and print a summary |
| `jsonl-stats` | read | counts and sums grouped by a key | Counter and aggregate scripts |
| `test-summary` | exec | passed, failed, first failing test and its trace head, log path | `go test ... \| tail`, `pytest ... \| grep` |
| `first-error` | read | first error line, surrounding context, line number | tail and grep of build and CI logs |
| `wait-for` | read | whether the condition was met, elapsed seconds, last line | `until ... sleep` polling loops |
| `grep-defs` | read | function, type and constant signatures with line numbers | `grep -n "^func \|^type "` |
| `http-json` | network | status, selected keys, size, path of the full body | `curl ... \| python -c "json.load"` |
| `replace-in-file` | write-workspace | matches found, replaced, dry-run diff head | read, `str.replace`, write heredocs, with an asserted match count |

Candidates for the next round, pending scan data from design partners: `sql-peek`, `git-touched`,
`repo-map`, `diff-summary`.
