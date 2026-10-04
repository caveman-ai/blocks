---
name: caveman-blocks
description: Reuse the verified scripts in this repo's .blocks/ instead of writing a heredoc. Use before writing any inline python3, python -c or node -e script, and when turning a repeated script into a block.
---

# Caveman Blocks

A block is one executable file in `.blocks/` with a typed header: params, the shape of the JSON it
returns, the effects it may have, and an example that doubles as its test. Blocks never leave the repo.

## Before writing a script

1. Read `.blocks/INDEX.md` (or the BLOCKS section of `AGENTS.md`). If a block fits, run it:
   `caveman-blocks run <name> --<param> <value>`.
2. If one almost fits, add a param to it instead of writing a new script.
3. The pre-run hook says `Blocks: <name> covers this. Next time: ...` when a script you are about to
   run matches a block. Follow it next time; the current command still runs.

## When a script repeats

A script you would run twice, or over about 10 lines, becomes a block:

```sh
caveman-blocks promote            # ranked candidates the hook captured, then a brief for one
caveman-blocks lint .blocks/<name>.py
caveman-blocks verify <name>      # runs the example, writes the stamp
caveman-blocks sync               # INDEX.md, AGENTS.md section, exports
```

Rules the header must satisfy: inputs are params, no hardcoded paths or ids; print one small JSON
object, the answer not the data; exit 0 on success, non-zero with `{"error": ...}`; safe to re-run.
Compose by calling other blocks with `caveman-blocks run`, never by copying their code.

## Setup, once

```sh
caveman-blocks init               # .blocks/ and the AGENTS.md section in this repo
caveman-blocks add json-peek      # a first-party block, copied in; you own it
caveman-blocks hooks install      # copies the binary to a stable path the plugin hook finds
caveman-blocks doctor             # binary, hooks, trust notes, python, sync state
```

If `caveman-blocks` is missing, the hook answers nothing and the session continues. Install from
https://github.com/caveman-ai/blocks/releases or with `npx caveman-blocks`.
