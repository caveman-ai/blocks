# 0005: Savings are counted events, not inferred lift

Date: 2026-10-03. Status: accepted.

## Context

The spec measured lift with a 10% holdout of sessions and a per-block holdout. Holdouts degrade one session in ten on purpose and need hundreds of sessions for a confidence interval. Individuals and small teams never get there.

## Decision

Count what the hook and runner see: scripts captured, hints shown, hints followed, exact repeats denied, block calls, and for each block call the bytes of full output written to `.out/` against the bytes returned. Report them as measured counts with those labels. Never print a dollar figure or a percentage saving in v0. A control-group method may return with a team layer that has the scale for it.

## Consequences

- Honest numbers from day one, consistent with Caveman's rule that measured, inferred and verified stay separate.
- The north star for dogfood is heredocs per session before versus after install, from `blocks scan`.
- Weaker marketing claim than "saved $1,840". Accepted.
