# Security

Report vulnerabilities to security@caveman.so. Do not open a public issue. Expect an acknowledgement
within three business days.

Scope: the `caveman-blocks` binary, its hook adapters, the installer scripts, the npm packages and the
first-party blocks in `blocks/`.

Design boundaries are described in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#security-boundaries) and
decisions 0006 and 0009. In short: hooks install at user level only, never from repo configuration; a
block's declared effects are enforced at the call site and `network` and `external` are denied by
default; user blocks are never uploaded anywhere; there is no telemetry.
