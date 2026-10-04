# 0009: User blocks never leave the repo; only the first-party registry is public

Date: 2026-10-03. Status: accepted.

## Context

Public skill registries were abused within months (ClawHavoc, ToxicSkills). The spec ruled out any registry. The shadcn model, which the product follows, depends on a curated set of first-party primitives.

## Decision

The only public registry is the set of blocks in this repository's `blocks/` folder, written and reviewed here, embedded in the binary and installed with `blocks add`. There is no upload, publish or sync command for user blocks. A block a team writes is visible to exactly the people who can see the repo and moves only through branches and pull requests.

## Consequences

- Supply-chain exposure is limited to this repository's own review process.
- Cross-repo sharing inside an organization is a later, opt-in feature, not a registry.
- Contributions to the first-party registry are pull requests here and pass lint, verify and review.
