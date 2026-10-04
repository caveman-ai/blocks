<div align="center">

<a href="https://github.com/caveman-ai/blocks"><img src="docs/assets/banner.png" alt="Caveman / Blocks. The word BLOCKS in solid pixel letters with a double-line shadow, the Caveman rock as the O. Why many script when few do trick. Agent write same script every session, throw it away, write it again. Blocks carve it once, keep it in your repo and check it every PR. Say this to shell: npx caveman-blocks scan. Works with Claude Code, Codex, Cursor and any AGENTS.md agent" width="880"></a>

# why many script when few do trick

**skills.sh for code mode. Agent write same script every session, throw it away, write it again. Blocks keep it in your repo, check it every PR, hand it back.**

<a href="https://github.com/caveman-ai/blocks/stargazers"><img src="https://img.shields.io/github/stars/caveman-ai/blocks?style=flat-square&color=111&label=stars" alt="GitHub stars"></a> <a href="#install"><img src="https://img.shields.io/badge/works_with-Claude_Code_·_Codex_·_Cursor-111?style=flat-square" alt="Claude Code, Codex, Cursor"></a> <a href="./LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-111?style=flat-square" alt="Apache-2.0"></a>

<table>
<tr>
<td align="center" width="33%"><h3>394×</h3>one script, rewritten in a week<br><sub>32 sessions, one developer, <a href="#one-script-written-394-times-in-a-week">counted by <code>scan</code></a></sub></td>
<td align="center" width="33%"><h3>up to 9.57%</h3>of task cost spent regenerating scripts<br><sub><a href="https://arxiv.org/abs/2609.30725">Purdue</a>, arXiv, Sep 2026</sub></td>
<td align="center" width="33%"><h3>397 → 76 tokens</h3>for one Go test run<br><sub><code>| tail -20</code> vs <a href="#blocks-against-what-agents-type-now"><code>test-summary</code></a>, this repo's 326 tests</sub></td>
</tr>
</table>

*"Once an agent develops working code for a task, it can save that implementation for future use."* **[Anthropic](https://www.anthropic.com/engineering/code-execution-with-mcp)**<br>
Code mode by **[Cloudflare](https://blog.cloudflare.com/code-mode/)** · Script waste measured by **[Purdue](https://arxiv.org/abs/2609.30725)** · From the makers of **[Caveman](https://github.com/JuliusBrussee/caveman)**, #1 on [Hacker News](https://news.ycombinator.com/item?id=47647455)

**[How it works](#how-it-works) · [Install](#install) · [The numbers](#the-numbers) · [What you get](#what-you-get)**

</div>

---

<table>
<tr><th width="50%">Normal agent · writes 101 tokens of script</th><th width="50%">🪨 Blocks agent · 21 tokens</th></tr>
<tr>
<td valign="top">

```bash
python3 - <<'EOF'
import json
ops=json.load(open('cloud/agent-surface/generated/operations.json'))
print(type(ops), (list(ops.keys())[:5] if isinstance(ops,dict) else len(ops)))
items = ops['operations'] if isinstance(ops,dict) and 'operations' in ops else ops
print(json.dumps(items[0] if isinstance(items,list) else list(items.items())[0], indent=1)[:800])
EOF
```

</td>
<td valign="top">

```bash
caveman-blocks run json-peek --path cloud/agent-surface/generated/operations.json
```

```json
{"kind": "object", "keys": ["schema_version", "contract_sha256",
 "operations", "exclusions", "operations[].id", … 15 more],
 "rows": 1, "sample": {"schema_version": 1, …}, "truncated": true}
```

</td>
</tr></table>

**Same question. 101 token become 21. Every session after, still 21.**

<sub>A real script from a Claude Code transcript, run again on the same 1.4 MB file (251,893 tokens). Answers trimmed here; in full they are 236 and 227 tokens. The heredoc's <code>[:800]</code> cut its answer off mid-object; the block returned all 20 keys. Token counts: tiktoken o200k. <a href="docs/research/readme-numbers-2026-10-04.md#the-demo-a-real-heredoc-against-json-peek">Method</a></sub>

## How it works

`AGENTS.md` says how. A block does it. `init` writes eight rules and a one-line-per-block index into your `AGENTS.md`:

| Rule | What it means |
|---|---|
| **Index first** | A block fits: run it. Almost fits: add a param to it |
| **Twice means block** | A script you would run twice, or one over ~10 lines, becomes a block. Not a heredoc, not a `/tmp` file |
| **Inputs are params** | No hardcoded paths, ids, ports or dates |
| **Answer, not data** | One small JSON object. Filter, count and truncate in code. The runner cuts anything over 2 KB |
| **Fails loud** | Exit 0 on success. Non-zero with `{"error": ...}` on failure |
| **Safe to re-run** | Idempotent, no prompts, cleans up after itself |
| **Header first** | Name, summary, params, effects, example. `caveman-blocks lint` checks it |
| **Compose** | Call other blocks with `caveman-blocks run`. Never copy their code |

One hook on the shell tool watches for heredocs. One a block covers gets a single line back: `Blocks: json-peek covers this. Next time: caveman-blocks run json-peek --path …`. A new one becomes a candidate the agent can promote into a block. The hook never denies or rewrites a command. `verify` runs every block's own example and stamps a hash into the file, and CI re-checks it on every pull request. A broken block drops out of the index. Full format: [docs/FORMAT.md](docs/FORMAT.md).

## Install

```bash
npx caveman-blocks init
```

Run it in your repo, then commit `.blocks/` and `AGENTS.md`. Everyone who clones it gets the blocks. One rock. That it.<br>
<sub>Pre-release: <code>v0.1.0-rc.1</code> binaries are on <a href="https://github.com/caveman-ai/blocks/releases">Releases</a>. npx, Homebrew and <code>curl | sh</code> go live with <code>v0.1.0</code>.</sub>

<details>
<summary><strong>More</strong>: see your waste first, add blocks, hints in your agent, plugins, CI, uninstall</summary>

```bash
npx caveman-blocks scan               # what your agents retyped this month. Nothing leaves your machine
npx caveman-blocks add json-peek      # copy a first-party block in. You own it now
npx caveman-blocks hooks install      # once per machine: hints in Claude Code, Codex, Cursor and OpenCode
claude plugin marketplace add caveman-ai/blocks && claude plugin install caveman-blocks@caveman-blocks   # Claude Code, as a plugin
codex plugin marketplace add caveman-ai/blocks && codex plugin add caveman-blocks@caveman-blocks         # Codex, as a plugin
```

Add the [CI step](docs/CI.md) so a pull request can't ship a broken block. Changed your mind: `caveman-blocks hooks uninstall` removes only the entries it added. Something off: `caveman-blocks doctor`.

</details>

## The numbers

| Who | Found |
|---|---|
| **[Purdue](https://arxiv.org/abs/2609.30725)** | Agents regenerating near-duplicate scripts: up to **68.00% of tasks** and **9.57% of task cost**. Seven behavioral principles cut cost 7.88–41.73% in six settings; synthesized skills helped in three of eight |
| **[Anthropic](https://www.anthropic.com/engineering/code-execution-with-mcp)** | Agents writing code instead of calling tools: **150,000 tokens down to 2,000**. Working code can be saved for future use |
| **[CUNY and Ohio State](https://arxiv.org/abs/2608.08453)** | **91.8% of 138,133** public `SKILL.md` files have at least one defect. Prose nobody runs rots |
| **[Microsoft Research and UIUC](https://arxiv.org/abs/2608.11888)** | **62.6%** of the efficiency regressions skills cause come from too much procedure. So: eight short rules |

### One script, written 394 times in a week

In one week, agents on one machine wrote near-duplicates of a single script 394 times, across 32 sessions:

```text
$ npx caveman-blocks scan --since 7d
  394x  import json;r=json.load(open('tests/flowbook/out/report.jso…  ~3 lines  32 sessions  covered by: json-peek (add)
  225x  import sys,json; d=json.load(sys.stdin); [print(r['type'], …  ~1 lines  37 sessions  covered by: json-peek (add)
  … 1853 more shapes
Measured: 94407 shell commands, 11726 inline Python scripts, 1855 shapes. Not grouped: 6331 edits, 3 scripts without a shape.
```

<sub>One heavy user, mostly Caveman repos. Over two months it was 30,221 scripts: half were file edits, which are never captured, and 3% took a parameter. <a href="docs/research/transcript-scan-2026-10-03.md">Study</a> · <a href="docs/research/readme-numbers-2026-10-04.md#one-week-of-scripts-caveman-blocks-scan---since-7d">This run</a></sub>

### Blocks against what agents type now

| Input | Raw | What agents type now | Block |
|---|---:|---:|---:|
| `go test -v ./...`, 326 tests | 10,561 | 397 · `\| tail -20` | **76** · `test-summary` |
| 1.4 MB JSON file | 251,893 | 236 · their own heredoc | **227** · `json-peek` |
| 366-line Go file | 3,111 | **250** · `grep -n "^func \|^type "` | 489 · `grep-defs` |

<sub>Tokens, tiktoken o200k. <code>grep-defs</code> loses to a bare grep: it labels every definition with file, kind, name and line. Not measured yet: how often agents follow a hint, and whole sessions. That's the first <a href="docs/ROADMAP.md">dogfood gate</a>. <a href="docs/research/readme-numbers-2026-10-04.md#blocks-against-what-agents-type-now">Method</a></sub>

## What you get

| Command | What it does |
|---|---|
| `scan` | Your agents' repeated scripts, from transcripts already on disk |
| `init` | Rules and index in `AGENTS.md`, `.blocks/` in the repo |
| `add <name>` | A first-party block and its test data, copied in. Your code from then on |
| `run <name>` | Run a block: effects checked, paths kept inside the repo, output capped at 2 KB |
| `hooks install` | Hints in Claude Code, Codex and Cursor when a block covers what the agent is writing |
| `promote` | The brief that turns a repeated script into a block |
| `lint` · `verify` · `sync` | Check it, run its example and stamp it, rebuild the index. `--check` for CI |
| `stats` | Scripts captured, hints followed, block calls. Counted, local |

First-party blocks, Python standard library only: `json-peek` · `jsonl-stats` · `test-summary` · `first-error` · `wait-for` · `grep-defs` · `http-json` · `replace-in-file`. [What each returns](blocks/README.md). `export` wraps them as `SKILL.md` for agents that read skills.

Not yet: Python only, no sandbox, and the Codex and Cursor hooks haven't run in a live session. Copilot CLI and Gemini CLI are next.

## Privacy, license, cite

Runs on your machine, sends nothing. No telemetry, no model calls, and the CLI makes no network calls. Blocks stay in your repo; captured scripts are scrubbed of secrets and kept outside it. [Trust boundary](docs/ARCHITECTURE.md).

[Apache-2.0](./LICENSE). Fork it, ship it, put it in your monorepo. "Caveman" and the rock logo are trademarks; see [TRADEMARKS.md](TRADEMARKS.md).

```bibtex
@software{brussee2026blocks, author = {Brussee, Julius}, year = {2026},
  title = {Caveman Blocks: why many script when few do trick}, url = {https://github.com/caveman-ai/blocks}}
```

---

<div align="center">

🪨 **Blocks save your agent the retyping. Star cost zero. Fair trade.**

<sub><a href="docs/THESIS.md">Thesis</a> · <a href="docs/FORMAT.md">Format</a> · <a href="docs/decisions/README.md">Decisions</a> · <a href="CONTRIBUTING.md">Contributing</a> · <a href="SECURITY.md">Security</a> · <a href="https://github.com/caveman-ai/blocks/issues">Issues</a></sub>

</div>
