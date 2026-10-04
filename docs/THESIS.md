# Thesis

Caveman Blocks turns the scripts coding agents already write into a verified library that lives in the
repo, in `.blocks/`, so agents reuse them instead of re-deriving them. One line installs it. Every session
after that gets cheaper and more reliable for everyone working in that repo. Blocks never leave the repo.

Pitch: code mode, made efficient. Your agents write each script once, write it well, and reuse it.
Two commands install it: `init` in the repo, `hooks install` once per machine.

## The claim

Prose cannot be checked. Code can. A skill or a memory note drifts into confident noise because nothing
runs it. A block has an exit code, a checkable contract, and a content hash it last passed with. That one property is
what lets Blocks capture, verify, rank and retire knowledge without a human curating prose.

## What the data says (this machine, Aug–Oct 2026)

Full numbers and method: [research/transcript-scan-2026-10-03.md](research/transcript-scan-2026-10-03.md).

- Agents write scripts constantly: 30,221 inline scripts in 201,785 shell calls, median 16 lines.
- They write them as heredocs inside the shell call, not as files. Any capture that watches files misses
  99% of them. The shell call is the only place to look.
- They almost never parameterize (3%) or return a structured answer (6%). The scripts are throwaway by
  construction, which is why they get rewritten.
- Half of them are file edits. Those are not procedures and are never captured as candidates. A block
  that makes the edit safe, with an asserted match count, is a different thing and ships in the registry.
- Repo-specific repeats exist but are rare. Shape-level repeats are enormous: "load a JSON file and print
  a summary" alone ran 2,800 times. The reusable unit is a generic parameterized block more often than a
  mined repo procedure.
- Scripts already return small answers (median 276 chars). Plain commands are what flood the context
  (median 3 KB, two thirds over 2 KB). Blocks does not compress that stream; harnesses and Caveman Wrap
  already do. Blocks points a dump of a structured file at the block that returns an answer instead.

## What follows from it

1. Ship the authoring rules and a curated set of first-party blocks first. Mine repo-specific procedures
   second, once there is evidence the blocks get used.
2. Capture at the shell call, with one pre-run hook that never rewrites or denies a command, and
   filter edits out at the door.
3. A block is one file with a header that is data: params, effects, the keys the answer must contain, an
   example that is the test. The index is generated from it.
4. Verification is a content-hash stamp written by running the block's own example, not a replayed
   test suite. CI re-checks and writes nothing.
5. Savings are counted, not inferred: bytes kept out of context, hints followed, scripts avoided. No
   control group until there is team-scale data to make one meaningful.
6. Promotion is done by the agent in the session, with deterministic tools, and the block travels with the
   pull request that needed it.

## Non-goals, for now

- No public hub for user blocks. The only public thing is the first-party registry this repo ships.
- No cross-repo or org-wide sharing. A block is visible to exactly the people who can see the repo.
- No sandbox in v0. Side effects are declared in the header and enforced by the runner from committed policy.
- No model calls inside the CLI. The agent the user already runs does the thinking.

## Origin

Product spec by Julius Brussee, 2026-10-03, revised the same day against the transcript scan. The original
spec's research table (Purdue, VibeMemBench, MSR/UIUC, CUNY/OSU, ContinualSkillBench, RELAI, PointFive,
Live-SWE-agent) is recorded in [research/prior-work.md](research/prior-work.md) with the caveat that
the numbers came from abstracts and vendor pages, not full reproductions.
