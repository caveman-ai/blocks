# Caveman Blocks: what agents actually write in code mode (2026-10-03)

Source: every Claude Code transcript on this machine (~/.claude/projects, 3,137 files, 4.6 GB, Aug–Oct 2026).
Scripts: scan.py (all Bash/Write calls, output sizes, exact-repeat clusters), scan2.py (what inline scripts do).
Raw output: scan-out.txt, scan2-out.txt. Bias: one heavy user, mostly Caveman repos, auto-mode told agents to
prefer Bash over Edit, which inflates "edit-file-in-place".

## Headline numbers
| Metric | Value |
|---|---|
| Bash tool calls | 201,785 |
| Inline scripts (heredoc, python -c, node -e) | 30,221 (15% of calls) |
| Script files written then executed in-session | ~140 |
| Inline script length | median 16 lines, p90 76, 70% >= 10 lines |
| Scripts that take params (argv/argparse) | 3% |
| Scripts that print JSON | 6% |
| Total Bash output returned to the model | 561 MB |
| Plain command output (compound/single) | median 2.7–3.4 KB, 59–66% over 2 KB |
| Inline script output | median 276 chars, 12% over 2 KB |
| Exact-normalized clusters repeating in 3+ sessions | 1,012 of 138K (4% of calls), inflated by forked subagents |

## What inline scripts do (overlapping categories, n=30,221)
| Category | Share | Block candidate? |
|---|---|---|
| Edit a file in place (read, replace, write) | 51% | No. It is an edit, not a procedure. Exclude from capture. |
| Load JSON/JSONL and print a summary | 29% | Yes: json-peek, jsonl-stats |
| Run tests, pipe to tail/grep | 13% | Yes: test-summary |
| Read a slice of a file | 11% | Covered by sed -n; no block |
| Aggregate / Counter / statistics | 9% | Yes: jsonl-stats |
| Render a report / markdown | 8% | Maybe later |
| DB query (psql, clickhouse, sqlite) | 7% | Yes, later: sql-peek |
| Regex extract from text | 7% | Yes: first-error, grep-defs |
| HTTP call and parse | 5% | Yes: http-json |
| Poll until a file/log changes | 4% | Yes: wait-for |

## Conclusions
1. Agent scripts are ephemeral heredocs inside Bash, not files. The spec's capture trigger ("a file the agent
   wrote and then executed") would miss 99% of them. Capture = look at Bash heredocs. Claude Code already keeps
   them in transcripts, so a scan is enough; no live capture needed for the MVP.
2. Repo-specific re-derivation is real but small at the exact level. Shape-level repetition is huge
   ("import json; json.load; print" alone is 2,800 runs). The reusable unit is a generic, parameterized block,
   not a mined repo procedure. First-party registry first; mining later.
3. Scripts already return small answers. Plain commands are what flood the context. The output cap pays most
   on cat/grep/git/go test, so the runner wrapper matters more than any block.
4. Agents almost never parameterize or emit JSON. The 8-rule authoring pack targets exactly the missing 97%.
5. Half of all scripts are edits. Any capture must filter them out or the candidate folder fills with noise.
