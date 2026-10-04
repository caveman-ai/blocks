# Agent-driven promotion

Promotion turns captured sightings into a committed block. The agent in the session does it. The CLI
supplies deterministic tools and checks; it never calls a model.

## Why the agent, not a merger

The agent that just wrote the script has the task context, knows which literals were incidental, and is a
stronger model than anything the CLI could run locally. Promotion by the session agent also means blocks
travel with the pull request that needed them, so review happens where review already happens.

## Trigger

The hook appends one sentence to a hint when a shape has sightings from two or more sessions, or when
five or more shapes exist, at most once per shape per day:

> Run caveman-blocks promote when the task is done.

Nothing interrupts the task.

## The flow

```
caveman-blocks promote           # shapes ranked by sightings and sessions, with a one-line preview
caveman-blocks promote <fp>      # the brief for one shape
```

The brief is plain text for the agent:

1. The most recent sighting's script, scrubbed, and the command lines it ran under. Literals that
   differ across sightings are listed as the likely parameters, with the values seen.
2. A proposed name, a proposed `[params]` table from those literals, and a proposed `[provenance]`
   table (`source = "candidate:<fp>"`, `sessions`, `created`).
3. A link to [FORMAT.md](FORMAT.md) and the eight rules.
4. The steps below, verbatim.

The agent writes `.blocks/<name>.py`, adds a fixture under `.blocks/fixtures/<name>/` if the example
needs one, then:

```
caveman-blocks lint .blocks/<name>.py
caveman-blocks verify <name>
caveman-blocks sync            # prints every file it changed
git add <those files> .blocks && git commit -m "blocks: add <name>"
```

In v0 the commit policy is text in the brief: commit on the current branch, which must not be the
repository's default branch. `lint` emits `W001` on the default branch, found from `origin/HEAD`, then
`init.defaultBranch`, then `main` or `master`. The block rides in whatever pull request the branch
becomes. A `promote = "pr"` policy that branches and opens a pull request is phase 2.

`verify` records a `verify{block, ok}` event; `promote` stops listing a shape once a block carries its
`fp` in `provenance.source`.

## Headless promotion

`promote --headless --agent <claude|codex>` drives the same brief through an agent's non-interactive
mode and applies the same checks. Same brief, same lint, same verify. Phase 2.

## Trust rules

- Lint fails on undeclared effects below the inferred floor, so a block cannot be promoted with a
  `read` header and a network import.
- Verify must pass at promotion time. No stamp, no index entry.
- `provenance.sessions = 1` is allowed. `stats` lists blocks with one source session and no later runs
  so they can be retired.
- Retirement is `caveman-blocks retire <name>`: removes the file and its fixtures and runs `sync`. Git
  keeps the history.
