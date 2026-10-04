# 0010: The hook hints and captures; it never denies in v0. Live capture stays despite the scan's conclusion.

Date: 2026-10-03. Status: accepted. Supersedes the same-day amendment in 0003 and narrows it.

## Context

The first draft gave the hook a deny path for exact repeats of a promoted block and for disallowed
effects. Review showed the exact-repeat match would almost never fire (a promoted block is a rewritten,
parameterized file, so its hash never equals the heredoc that led to it) and would false-deny when
normalization collapsed different string literals. The effects check by string-matching `blocks run` in
the hook was trivially bypassed by any other call form.

The transcript scan ([research](../research/transcript-scan-2026-10-03.md)) concluded that a scan over
transcripts is enough for an MVP and that live capture is unnecessary. The founder ruled that the product
must keep enforceability and in-session learning, which a scan cannot give.

## Decision

- The hook has no deny path. It allows every command and emits at most one hint line and counted events.
- Effects are enforced in the runner from the committed `config.toml` at `HEAD` (decision 0006).
- Live capture stays, because in-session hints need the hook anyway and sightings make promotion briefs
  possible without a transcript reader. The scan's "a scan is enough" is correct for measuring the
  problem and wrong for changing behavior; the research record is amended to say so.
- Cursor, Copilot and Gemini deliver the hint through a post-run event because their pre-run event has no
  context field. One engine, one decision per call.
- Compressing raw command output is a non-goal; harnesses already cap it and Caveman Wrap compresses it.

## Consequences

- Gate 1's "zero false denials" becomes trivially true; the gate's real test is hint override rate.
- The hook can be installed with no risk of blocking work. Copilot's fail-closed pre-tool semantics
  only matter if the binary is missing, which `hooks install` and `doctor` guard.
- A deny for exact repeats can return later with a `replaces` key populated from sighting hashes, if
  dogfood shows agents ignoring hints.
