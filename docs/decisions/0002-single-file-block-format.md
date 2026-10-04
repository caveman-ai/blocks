# 0002: A block is one file with a `# /// block` header

Date: 2026-10-03. Status: accepted.

## Context

The spec proposed a folder per block with `block.toml`, `run.py`, `test.sh` and a generated SKILL.md. Reusing the Agent Skills folder format was considered and rejected by the founder: the product needs its own identity, and a format only earns that if it does things prose cannot.

## Decision

A block is one executable file. Its manifest is a TOML header in the PEP 723 inline-metadata shape with the type name `block`. The header holds name, summary, typed params, return shape, effects, the example call that doubles as the test, `matches` patterns for the scripts it replaces, provenance and the verification stamp. Full grammar in [FORMAT.md](../FORMAT.md). SKILL.md is an export, produced on demand by `caveman-blocks export`.

## Consequences

- Forty blocks is forty files. No folders, no separate manifest, no test file.
- Agents already know PEP 723, so the header costs nothing to learn.
- The contract is data: the index, lint, verify and the hook's matching all read the header.
- Breaking format changes rename the fence type (`block2`); additive changes add optional keys. No version field.
- Cost: a format to document and maintain, and a compatibility export to keep working.
