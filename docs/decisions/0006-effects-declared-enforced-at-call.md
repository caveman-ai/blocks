# 0006: Side effects are declared in the header and enforced at the call site, no sandbox

Date: 2026-10-03. Status: accepted.

## Context

The spec enforced `side_effects` with bubblewrap on Linux, sandbox-exec on macOS and a container in CI. sandbox-exec is deprecated, bubblewrap is Linux-only, and both are heavy for a tool that must install in one line.

## Decision

Every block declares `effects` as one of `read`, `write-workspace`, `exec`, `network`, `external`. The first three are allowed by default because they stay inside the repo; the last two are denied until `config.toml` allows them. Lint fails when the script imports network or subprocess facilities without declaring them. The hook denies `blocks run` of a `network` or `external` block unless `config.toml` allows that effect. CI verify runs in the CI container. No local sandbox in v0.

## Consequences

- The promise "blocks do what they declare" is enforced at promotion (lint) and at call (hook), not by isolation.
- A malicious block that lies in its header and hides its imports is not caught by v0. CODEOWNERS on `.blocks/` and PR review are the mitigation; a sandbox can be added behind the same `effects` field later.
