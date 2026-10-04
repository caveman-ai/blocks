# Agent-driven promotion

Promotion turns a captured candidate into a committed block. The agent in the session does it. The CLI
supplies deterministic tools and checks; it never calls a model.

## Why the agent, not a merger

The agent that just wrote the script has the task context, knows which literals were incidental, and is a
stronger model than anything the CLI could run locally. A separate merger with a small model would be
slower, worse, and a second thing to trust. Promotion by the session agent also means blocks travel with
the pull request that needed them, so review happens where review already happens.

## Trigger

The hook attaches one line of context when a captured candidate's shape has been seen in two or more
sessions, or when `.candidates/` holds five or more entries:

> Blocks: 2 candidates repeat across sessions. Run `blocks promote` when the task is done.

The agent decides when. Nothing interrupts the task.

## The flow

```
blocks promote                 # lists candidates ranked by repeat count, with shape and first command
blocks promote <fingerprint>   # prints the promotion brief for one candidate
```

The brief is plain text the agent reads:

1. The captured script and the command lines it ran under, with literals that varied across runs marked.
2. The format reference: header fields, the eight rules, the lint rules.
3. A proposed name (verb-noun) and the parameters implied by the varying literals.
4. The exact next steps: write `.blocks/<name>.<ext>`, run `blocks lint <file>`, run `blocks verify
   <name>`, run `blocks sync`, commit.

The agent writes the file. Then:

```
blocks lint .blocks/<name>.py    # header complete, params appear in code, prints JSON, no absolute paths
blocks verify <name>             # runs `example` at HEAD; writes the verified stamp or quarantines
blocks sync                      # regenerates INDEX.md and the managed AGENTS.md section
```

Commit policy comes from `config.toml`:

- `promote = "commit"` (default): the agent commits on the current branch. If the current branch is the
  default branch, the CLI refuses and asks for a branch. The block then rides in whatever pull request the
  branch becomes.
- `promote = "pr"`: the agent commits on a `blocks/<name>` branch and opens a pull request.

The candidate is deleted from `.candidates/` once the block passes verify.

## Headless promotion

For teams that want candidates promoted without a session, `blocks promote --headless --agent <claude|codex>`
runs the same brief through the agent's non-interactive mode and applies the same checks. Same brief,
same lint, same verify, same commit policy. This is a convenience over the interactive flow, not a
different path, and it is out of scope for v0.

## Trust rules

- A block cannot be promoted with undeclared effects. Lint fails if the script imports a network or
  subprocess module without `effects` declaring it.
- A block promoted from a single session is still a block. The header records `provenance.sessions`;
  `blocks stats` surfaces blocks with one session and no later use so they can be retired.
- Verify must pass at promotion time. No stamp, no index entry.
- Retirement is `blocks retire <name>`: removes the file and syncs the index. Git keeps the history.
