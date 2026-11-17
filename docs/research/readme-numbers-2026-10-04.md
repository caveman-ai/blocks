# Numbers in the README (2026-10-04)

Every number the README prints, with how it was measured. Token counts are tiktoken `o200k_base`.
Binary: `caveman-blocks` built from `9a8f341`. Go 1.26.1, macOS arm64.

## One week of scripts: `caveman-blocks scan --since 7d`

The author's machine, run 2026-10-04. The first five of the 20 rows printed, then the summary, verbatim except that file paths are redacted as `[path]`:

```text
  394x  import json;r=json.load(open('[path]/report.jso…  ~3 lines  32 sessions  covered by: json-peek (add)
  225x  import sys,json; d=json.load(sys.stdin); [print(r['type'], …  ~1 lines  37 sessions  covered by: json-peek (add)
  222x  import json;d=json.load(open('[path]/schem…  ~6 lines  25 sessions  covered by: json-peek (add)
  189x  import json; for f in ['getting-started','onboarding-connec…  ~4 lines  24 sessions  covered by: json-peek (add)
  144x  import json; m=json.load(open('model.json')); bad=0; n=0; s…  ~7 lines  21 sessions  covered by: json-peek (add)
  … 1835 more shapes

Measured: 94407 shell commands, 11726 inline Python scripts, 1855 shapes. Not grouped: 6331 edits, 3 scripts without a shape.
Read 2012 transcript files (claude 1746 files, 0 skipped; codex 264 files, 2 skipped; cursor 2 files, 0 skipped).
```

"covered by" means a block's `matches` pattern fired. The 394x shape filters a test report rather than
peeking at it, so `json-peek` would hint on it but not answer it; the README does not claim it does.

## The demo: a real heredoc against `json-peek`

The script is verbatim from a Claude Code transcript, with its leading `cd … &&` dropped and its file
path changed to a public file. Rerun 2026-11-17 with `caveman-blocks` built from `main`. Both sides ran on
GitHub's REST API description, `descriptions/api.github.com/api.github.com.json` in
[github/rest-api-description](https://github.com/github/rest-api-description) at `c9c95fe34c`:
14,275,550 bytes, 2,745,352 tokens.

| | Agent's heredoc | `caveman-blocks run json-peek --path …` |
|---|---:|---:|
| What the agent writes | 96 tokens | 16 tokens |
| What comes back | 34 tokens | 287 tokens |

The file is an object without an `operations` key, so the heredoc prints the first five top-level keys
and the first key-value pair (`"openapi": "3.0.3"`) and stops. `json-peek` returns 20 keys and a
truncated sample of every top-level field.

## Blocks against what agents type now

Run in a clone of this repo at `9a8f341` with the blocks added.

| Input | Raw | What agents type now | Block |
|---|---:|---:|---:|
| `go test -v ./...`, 687 lines, 326 tests passing | 10,561 | 397 (`go test ./... 2>&1 \| tail -20`) | 76 (`test-summary --cmd "go test -v ./..."`) |
| `internal/hook/engine.go`, 366 lines | 3,111 | 250 (`grep -n "^func \|^type "`, 20 lines) | 489 (`grep-defs --path …`) |
| `api.github.com.json` above | 2,745,352 | 34 (the heredoc's output) | 287 (`json-peek`) |

`grep-defs` loses to a bare grep: it adds a file, kind, name and line key to every definition and also
returns constants.

## Outside sources

Checked against the source pages on 2026-10-04. Quotes are verbatim.

| Source | Quote |
|---|---|
| Purdue, [Analyzing and Mitigating Cost-Inefficient Behaviors in Coding Agents](https://arxiv.org/abs/2609.30725), Sep 2026 | Script regeneration is "regenerating near-duplicate scripts with minor differences rather than editing existing scripts, wasting output tokens and forgoing artifact reuse." "SimScrpt affects up to 68.00% of tasks and contributes up to 9.57% of task cost." "DevSkills robustly reduce cost in six settings by 7.88–41.73%"; "SynSkills robustly reduce average cost in three of eight settings by 8.86–22.32%". |
| Anthropic, [Code execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp), Nov 2025 | "Once an agent develops working code for a task, it can save that implementation for future use." "This reduces the token usage from 150,000 tokens to 2,000 tokens". |
| Cloudflare, [Code Mode: the better way to use MCP](https://blog.cloudflare.com/code-mode/), Sep 2025 | "LLMs are better at writing code to call MCP, than at calling MCP directly." No numbers. |
| CUNY and Ohio State, [What Keeps Agent Skills from Being Reusable?](https://arxiv.org/abs/2608.08453), Aug 2026 | "91.8% of skills contain at least one detected defect", over 138,133 `SKILL.md` files. |
| Microsoft Research and UIUC, [Agent Skills Can Be Harmful](https://arxiv.org/abs/2608.11888), Aug 2026 | "Excessive Procedure accounts for 114 of 182 cases (62.6%)" of skill-induced efficiency regressions. |
