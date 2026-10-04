# 0006: Side effects are declared in the header and enforced at the call site, no sandbox

Date: 2026-10-03. Status: accepted.

## Context

The spec enforced `side_effects` with bubblewrap on Linux, sandbox-exec on macOS and a container in CI. sandbox-exec is deprecated, bubblewrap is Linux-only, and both are heavy for a tool that must install in one line.

## Decision

Every block declares `effects` as one of `read`, `write-workspace`, `exec`, `network`, `external`. The first three are allowed by default because they stay inside the repo; the last two are denied until `config.toml` allows them. Lint fails when the script imports network or subprocess facilities without declaring them. The runner refuses a `network` or `external` block unless the `config.toml` committed at `HEAD` allows that effect, so a grant must be committed and reviewed before it works. `verify` applies the same gate, in CI too. No local sandbox in v0. Effects are hygiene, not a security boundary: a direct `python3 .blocks/x.py` bypasses the runner and a header can lie.

## Consequences

- The promise "blocks do what they declare" is enforced at promotion (lint) and at call (hook), not by isolation.
- A malicious block that lies in its header and hides its imports is not caught by v0. CODEOWNERS on `.blocks/` and PR review are the mitigation; a sandbox can be added behind the same `effects` field later.
