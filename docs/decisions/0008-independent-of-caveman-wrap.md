# 0008: Independent of Caveman Wrap

Date: 2026-10-03. Status: accepted.

## Context

The spec positioned Blocks as a hook pack inside Caveman Wrap, which already ships adapters for ten or more agents. The founder decided Blocks is a separate product and repository.

## Decision

This repository owns its installer, its per-agent hook adapters, its CLI and its release. It does not import from or depend on the `caveman` repository. Both can be installed side by side; the hook ordering is irrelevant because Blocks only appends to commands and Wrap documents that it does not change user commands.

## Consequences

- Duplicated adapter knowledge across two repos. Accepted for independence.
- Shared brand and trademark rules (TRADEMARKS.md mirrors the main repo).
- A later integration can be a one-line `caveman blocks` passthrough in Wrap; it is not required.
