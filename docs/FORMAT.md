# Block file format, v0

A block is one executable file in `.blocks/` whose header is a TOML document inside a PEP 723 style
inline-metadata fence with the type name `block`. The grammar is the PEP 723 grammar verbatim; only the
type differs, and the specification requires tools that do not know a type to ignore it, so `uv run` and
`pipx run` keep working on a block file (checked against uv 0.7.19 and the pipx source, 2026-10-03).

## Example

```python
#!/usr/bin/env python3
# /// block
# name = "json-peek"
# summary = "Shape of a JSON or JSONL file: keys, row count, one sample. Never the data."
# effects = "read"
# example = "--path testdata/blocks/json-peek/sample.json --depth 1"
# returns = "{ keys: [str], rows: int, sample: object, truncated: bool }"
# matches = ['json\.loads?\(', 'JSON\.parse\(']
#
# [params]
# path  = { type = "path", required = true, help = "JSON or JSONL file" }
# depth = { type = "int", default = 2, max = 10, help = "How deep to walk nested keys" }
#
# [provenance]
# created = "2026-10-03"
# source = "registry:json-peek@0.1.0"
#
# verified = "a1b2c3d 2026-10-03"
# ///
import argparse, json, sys
...
print(json.dumps(answer))
```

## Grammar

- Header starts with the line `# /// block` and ends with the line `# ///`. Every line between is `#`
  followed by a space and content, or a bare `#` for an empty line. Reference regex, from the
  specification: `(?m)^# /// (?P<type>[a-zA-Z0-9-]+)$\s(?P<content>(^#(| .*)$\s)+)^# ///$`.
- One `block` header per file. A second one is an error.
- The header must appear within the first 20 lines, after an optional shebang and encoding line.
- v0 supports languages whose comment leader is `#`: Python and POSIX shell. A `//` leader for
  JavaScript is a later, additive extension.
- Content is TOML. Unknown keys are an error in lint, so typos cannot silently disable a feature.

## Fields

| Key | Required | Written by | Rules |
|---|---|---|---|
| `name` | yes | author | 1–64 chars, `a-z`, `0-9`, `-`; no leading, trailing or double hyphen; equals the file name without extension. Same rules as the Agent Skills `name`, so export is lossless. |
| `summary` | yes | author | One sentence, at most 120 characters. This is the index line and the exported skill description. Say what comes back, not what the script does internally. |
| `effects` | yes | author | One of `read`, `write-workspace`, `exec`, `network`, `external`. See below. |
| `example` | yes | author | Arguments only, relative to the repo root. `blocks verify` runs the block with these arguments and this is the only test. |
| `returns` | yes | author | Free-text shape of the JSON answer. Shown in the brief and the export. |
| `matches` | no | author | RE2 patterns tested against the body of an inline script the agent is about to run. A hit produces a hint. At least one pattern is expected for blocks that replace a common script shape. |
| `[params]` | no | author | One table per parameter: `type` (`str`, `int`, `float`, `bool`, `path`, `enum`), `required` or `default`, optional `help`, `min`, `max`, `values` for `enum`. |
| `requires` | no | author | Executables the block needs on `PATH`, for example `["pytest"]`. Verify fails early with a clear message when one is missing. |
| `[provenance]` | no | author or `promote` | `created`, `source` (`registry:<name>@<version>` or `session`), `sessions`, `runs`. |
| `verified` | no | tool | `"<short-sha> <date>"`. Written by `blocks verify` on success. |
| `state` | no | tool | Absent means active. `"quarantined"` is written by `blocks verify` on failure and removes the block from the index. |

The tool writes exactly two keys, `verified` and `state`, always as the last lines of the header, and
never rewrites any other line. A diff of a verify run is therefore one or two lines.

## Effects

| Value | Meaning | Allowed by default |
|---|---|---|
| `read` | Reads files under the repo. No writes, no subprocesses, no network. | yes |
| `write-workspace` | Writes under the repo root. | yes |
| `exec` | Runs other programs in the repo: tests, builds, git, formatters. No network. | yes |
| `network` | Opens network connections. | no, until `allow_effects` in `config.toml` includes it |
| `external` | Changes state outside the repo: pushes, deploys, sends, deletes elsewhere. | no, until allowed |

Lint infers a minimum effect from the source. For Python: `urllib`, `http.client`, `socket`, `requests`,
`httpx` imply `network`; `subprocess`, `os.system`, `os.exec*` imply at least `exec`; writes imply at
least `write-workspace`. A declared effect below the inferred one fails lint. The inference is a floor,
not a proof; review and `CODEOWNERS` on `.blocks/` remain the control for a header that lies.

## Calling convention

- Parameters arrive as `--name value` flags. Booleans are bare flags. The block parses them itself with
  the language's standard library; lint checks that each declared parameter name appears in the source
  as `--name`.
- stdout is exactly one JSON object and nothing else. Progress and logs go to stderr.
- Exit 0 means the answer is valid. Non-zero means failure, and the JSON object carries an `error`
  string. Usage errors exit 2.
- The answer is small. `blocks run` caps stdout at 2 KB, spills the full text to `.blocks/.out/<id>.log`
  and appends `"_full": "<path>"` to what the agent sees. Design the answer so the cap never triggers.
- Idempotent and non-interactive: no prompts, no reliance on being run once.
- Composition: a block calls another block with `blocks run <name> ...` and parses the JSON. There is no
  shared library.

## Index line

`blocks sync` renders one line per active block, sorted by name, into `.blocks/INDEX.md` and into the
managed section of each instruction file:

```
json-peek   --path <path> [--depth 2]      Shape of a JSON or JSONL file: keys, row count, one sample.
```

Sorted by name so the text is deterministic across machines and the committed file never churns. More
than `index_max` (default 40) active blocks fails `sync` with a request to retire some. Quarantined
blocks are listed in a separate short section so the agent knows they exist but will not call them.

## Lint rules

| Rule | Check |
|---|---|
| F001 | Header present, within the first 20 lines, parses as TOML, one per file |
| F002 | No unknown keys |
| F003 | `name` valid and equal to the file name |
| F004 | `summary` one line, at most 120 characters |
| F005 | `effects` is a known value and not below the inferred floor |
| F006 | `example` present and non-empty |
| F007 | Every declared parameter appears as `--name` in the source; every `--flag` in the source is declared |
| F008 | `matches` patterns compile as RE2 |
| F009 | No absolute home paths (`/Users/`, `/home/`, `C:\Users\`) in the source |
| F010 | Source prints JSON: `json.dumps` or an equivalent appears; nothing else writes to stdout |
| F011 | Shebang present and the file is executable |
| F012 | First-party blocks only: imports are Python standard library |

## Export

`blocks export` writes a `SKILL.md` per active block for agents without the hook or the CLI:
`.claude/skills/<name>/SKILL.md` for Claude Code and `.agents/skills/<name>/SKILL.md` for the clients that
read the shared path. The frontmatter is `name` and `summary`; the body is the parameter table and the
two call forms, `blocks run <name> ...` and `python3 .blocks/<name>.py ...`. Export output is generated
and may be committed; `sync --check` in CI fails when it is stale.

## Versioning the format

Additive changes add optional keys. A breaking change renames the fence type (`# /// block2`) and the tool
reads both for one major version. There is no version field.
