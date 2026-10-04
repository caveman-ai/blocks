# Security

Report vulnerabilities to security@caveman.so. Do not open a public issue. Expect an acknowledgement
within three business days.

Scope: the `caveman-blocks` binary, its hook adapters, the installer scripts, the npm packages and the
first-party blocks in `blocks/`.

Design boundaries are described in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#security-boundaries) and
decisions 0006 and 0009. In short: hooks install at user level only, never from repo configuration; the hook writes only to a
state directory outside the repo and refuses symlinks under `.blocks/`; declared effects are enforced in
the runner from committed config and `network` and `external` are denied by default; user blocks are
never uploaded anywhere; there is no network code and no telemetry in v0.

What this does NOT protect against: a malicious repository. A repo author writes the blocks, their
`effects`, their stamps and `allow_effects`, so in an untrusted clone "verified" means nothing and a
block is arbitrary code the same way a Makefile is. The hook never runs a block; it only hints. Treat
`caveman-blocks run` in a repo you do not trust like running that repo's scripts.
