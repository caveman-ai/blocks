# Roadmap

Gates are go/no-go decisions on the next phase. Failing a gate reshapes the product; it does not get
worked around.

## Phase 0: foundation (this)

Docs, format, decisions, repo layout, CI skeleton. Done when a reviewer can build the v0 from the docs
without asking a question that the docs should have answered.

## Phase 1: v0, local only

- `init`, `add`, `run`, `lint`, `verify` (with `--check`), `sync` (with `--check`), `scan`, `stats`, `promote` (brief only), `retire`, `export`, `doctor`
- `hooks install|uninstall|status` for Claude Code, Codex CLI and Cursor
- Eight first-party blocks with fixtures, Python 3.10+ stdlib: `json-peek`, `jsonl-stats`, `test-summary`, `first-error`,
  `wait-for`, `grep-defs`, `http-json`, `replace-in-file`
- Hook engine with golden decision tables; hint and capture only; rule 1 under 5 ms
- State dir outside the repo; scrub list with golden tests
- Release: GitHub releases, Homebrew tap, `curl | sh`, `npx caveman-blocks`

**Gate 1, dogfood on two repositories for two weeks** (candidates: the `caveman` repo and this one; a repo
with a hard byte cap on its instruction file needs the import form). Pass if: `run` events on most
sessions, at least three first-party blocks used unprompted, heredocs per session down against the
pre-install scan, hints followed (a `run` of the hinted block in the same session) above 50%. Fail on:
hints ignored on most sessions, hook latency complaints, any report of the hook breaking a session.

## Phase 2: adoption surface

- Copilot CLI and Gemini CLI adapters
- Headless promotion through an agent's non-interactive mode; `promote = "pr"` policy
- `diff` and `update` against newer registry versions, using the lock file; first network code
- A deny for exact repeats via a `replaces` key, only if dogfood shows hints being ignored
- More first-party blocks driven by the scan data of design partners
- Windows: PowerShell extraction in the hook

**Gate 2, twenty design-partner repos.** Pass if the scan finds repeat groups in most repos and the
first-party registry covers the top groups. The kill signal from the original spec stays: under five
repeat groups per active repo per month means mining is not worth building.

## Phase 3: repo mining

- Shape clustering across candidates, parameter inference from varying literals, promotion briefs that
  merge several candidates into one block
- Only if gate 2 shows repo-specific repeats worth the machinery

## Not scheduled

- Team layer, dashboards, CI verification service, receipts, holdout measurement
- Cross-repo sharing
- Sandbox
