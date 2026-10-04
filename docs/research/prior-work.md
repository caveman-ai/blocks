# Prior work cited by the original spec (2026-10-03)

Recorded as cited. Numbers came from paper abstracts and HTML pages, not full PDFs, and several authors
sell products in this space. Treat as motivation, not as verified results.

| Finding | Number | Source | Consequence for Blocks |
|---|---|---|---|
| Near-duplicate script regeneration is a top-three waste in coding agents | waste in 79–98% of tasks, up to 22.75% of cost | Purdue, Sep 2026 | Direct target |
| Auto-synthesized skills pay back slowly | helped in 3 of 8 settings; ~3,000 tasks to break even | Purdue, Sep 2026 | Curated blocks first, mining later |
| Seven short human-written principles cut cost 8–42% at equal pass rate | about 2x agent-synthesized skills | Purdue, Sep 2026 | The 8-rule authoring pack |
| Off-the-shelf memory does not help coding agents | 11 of 12 solver × memory pairs lost to no-memory | VibeMemBench, Sep 2026 | Store verified compact artifacts, not transcripts |
| Relevant-looking skills cause failures | 68.8% of skill-induced failures from relevant-looking skills | MSR/UIUC, Aug 2026 | Hints not denials; exact match only for deny |
| Excessive procedure causes efficiency regressions | 62.6% of skill-caused regressions | MSR/UIUC, Aug 2026 | Short rules, short descriptions |
| Public skills are mostly broken | 91.8% of 138K SKILL.md files have a defect | CUNY/OSU, Aug 2026 | Lint everything; no third-party registry |
| Skills beat plain context only for procedures | 0.602 vs 0.605 overall | ContinualSkillBench, Aug 2026 | Capture procedures (scripts), not wisdom |
| Ungated self-optimization regresses | 54.5% vs 56.8% baseline | RELAI, Jul 2026 | Verify inside promotion, not after |
| Agents that build their own tools do well | 79.2% SWE-bench Verified | Live-SWE-agent | Make in-session tool-making persist |
| Cache is most of the bill | ~87% of cost is cache creation and reads | PointFive, 2026 | Index in a stable prefix, never mid-session |
| Tool output is a large share of prompt tokens | 28% in production; failed builds 7–8x more | Copilot at scale | Output cap |

Registry abuse that justifies "no third-party hub": ClawHavoc (341 of 2,857 ClawHub skills malicious, Feb
2026), Snyk ToxicSkills (36.8% of 3,984 skills flawed, 76 malicious), Claude Code repo-config hook CVEs
(fixed Jan 2026).
