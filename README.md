# Caveman Blocks

Your agents write the same scripts every session. Blocks makes them write each one once, well, and reuse it.

A block is one executable file in your repo's `.blocks/` folder with a typed header: parameters, the
shape of the JSON answer it returns, the side effects it is allowed, an example call that doubles as its
test, and the commit it last passed on. Agents call blocks instead of rewriting them. Blocks never
leave the repo.

```
npx caveman-blocks init          # .blocks/ in this repo, rules + index in your AGENTS.md
caveman-blocks hooks install     # one pre-run hook per agent on this machine, once
caveman-blocks scan              # what your agents rewrote last month, from their own transcripts
caveman-blocks add json-peek     # a first-party block, copied into your repo; you own it
```

Status: foundation. The design is written; the binary is not. Start with [docs/THESIS.md](docs/THESIS.md).

## How it works

- **Rules.** Eight short lines in your instruction file teach agents to write scripts that take
  parameters, return a small JSON answer, and import existing blocks.
- **Index.** One line per block, in the same file, in the cached prompt prefix. At most forty.
- **Hook.** One pre-run hook on the shell tool. It captures scripts agents write, hints when one matches a
  block, and denies only an exact repeat of a block that already exists. It never rewrites a command.
- **Promotion.** The agent in the session turns a captured script into a block with `blocks promote`,
  then `lint`, `verify`, `sync`. The block rides in the pull request that needed it.
- **Verification.** `blocks verify` runs each block's example at HEAD and stamps the commit into the file.
  A failure quarantines the block out of the index. One line in CI.
- **Counting.** Scripts captured, hints followed, block calls, bytes kept out of context. Measured,
  labelled, local. No dollar figures.

## Documents

| | |
|---|---|
| [docs/THESIS.md](docs/THESIS.md) | Why, with the transcript data that shaped the design |
| [docs/FORMAT.md](docs/FORMAT.md) | The block file format |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Components, data flow, security boundaries |
| [docs/HOOK.md](docs/HOOK.md) | Decision engine and per-harness adapters |
| [docs/INTEGRATION.md](docs/INTEGRATION.md) | Generic protocol, dialects, profiles: adding an agent in an afternoon |
| [docs/AGENT-PROMOTION.md](docs/AGENT-PROMOTION.md) | How an agent promotes a candidate |
| [docs/REPOSITORY.md](docs/REPOSITORY.md) | Repo layout and tech stack |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Phases and gates |
| [docs/decisions/](docs/decisions/README.md) | Decision records |
| [docs/research/](docs/research/) | Transcript scan, harness contracts, prior work |

## Supported agents

Claude Code, Codex CLI, Cursor, GitHub Copilot CLI, Gemini CLI through hooks. Any agent that reads
`AGENTS.md` or Agent Skills gets the rules, the index and exported skills without the hook.

## License

Apache-2.0. "Caveman" is a trademark; see [TRADEMARKS.md](TRADEMARKS.md).
