# 0001: Curated first-party blocks first, repo mining later

Date: 2026-10-03. Status: accepted.

## Context

The original spec led with capture, clustering and a merger that synthesizes blocks from repeated agent scripts. The transcript scan ([research](../research/transcript-scan-2026-10-03.md)) found repo-specific exact repeats are rare (about 4% of shell calls, inflated by forked subagents) while shape-level repeats are enormous and generic: load JSON and summarize, run tests and summarize, poll a file, extract the first error. The cited research also found human-written short rules beat agent-synthesized skills by about two to one.

## Decision

Ship the eight authoring rules and a curated registry of first-party blocks written by hand in this repo. Capture still runs from day one so candidates accumulate, and promotion is available, but no clustering model, embeddings or AST fingerprinting is built until first-party blocks show use in dogfood.

## Consequences

- The shadcn analogy becomes true: the registry is the product people see first.
- The repeat table from `caveman-blocks scan` is the first-minute win and also the evidence that decides whether mining is worth building.
- Risk: if first-party blocks are not used, the whole premise is wrong, and we learn that in weeks rather than after building a pipeline.
