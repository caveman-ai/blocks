# 0004: The session agent promotes; the CLI never calls a model

Date: 2026-10-03. Status: accepted.

## Context

The spec had a local merger with a small model writing canonical scripts and manifests, then a human PR gate. The founder asked for promotion to be agent-driven.

## Decision

`blocks promote` produces a brief; the agent in the session writes the block file; `blocks lint`, `blocks verify` and `blocks sync` check it; the agent commits under the repo's policy (default: commit on the current non-default branch so the block rides in the existing pull request). The CLI contains no model calls. A headless mode drives the same brief through an agent's non-interactive CLI later.

## Consequences

- Same tools for every agent; the quality of promotion scales with the agent the team already uses.
- No API key, no model dependency, no cost inside the tool.
- Human review happens in the pull request that needed the block, not in a separate gate.
- Trust rests on lint and verify being strict. See [AGENT-PROMOTION.md](../AGENT-PROMOTION.md).
