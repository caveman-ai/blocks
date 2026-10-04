# Block file format, v0

A block is one executable Python file in `.blocks/` whose header is a TOML document inside a PEP 723
style inline-metadata fence with the type name `block`. The grammar is the PEP 723 grammar verbatim; only
the type differs, and the specification requires tools that do not know a type to ignore it, so `uv run`
and `pipx run` keep working on a block file (checked against uv 0.7.19 and the pipx source, 2026-10-03).

v0 supports Python 3.10 or later only. Shell and JavaScript blocks are later, additive extensions with a
`//` fence variant for JavaScript. First-party blocks use the standard library only.

## Example

```python
#!/usr/bin/env python3
# /// block
# name = "json-peek"
# summary = "Shape of a JSON or JSONL file: keys, row count, one sample. Never the data."
# effects = "read"
# example = ["--path", "$FIXTURES/sample.json", "--depth", "1"]
# matches = ['json\.load\(open', 'json\.loads?\(.*\.read\(\)', 'json\.load\(sys\.stdin']
#
# [returns]
# keys = ["kind", "keys", "rows", "sample", "truncated"]
# doc = "kind is object|array|jsonl; keys are top-level; sample is one element"
#
# [params]
# path  = { type = "path", required = true, help = "JSON or JSONL file inside the repo" }
# depth = { type = "int", default = 2, max = 10, help = "How deep to walk nested keys" }
#
# [provenance]
# created = "2026-10-03"
# source = "registry:json-peek@0.1.0"
#
# [stamp]
# verified = "3f9a1c2b7d4e"
# ///
import argparse
import json
import sys
...
print(json.dumps(answer))
```

## Grammar

- The header starts with the line `# /// block` and ends with the line `# ///`. Every line between is
  `#` followed by a space and content, or a bare `#` for an empty line. Reference regex, from the
  specification: `(?m)^# /// (?P<type>[a-zA-Z0-9-]+)$\s(?P<content>(^#(| .*)$\s)+)^# ///$`.
- One `block` header per file. A second one is an error.
- The header must appear within the first 20 lines, after an optional shebang and encoding line.
- Content is TOML. Unknown keys are an error in lint, so a typo cannot silently disable a feature.

## Fields

| Key | Required | Written by | Rules |
|---|---|---|---|
| `name` | yes | author | 1–64 chars, `a-z`, `0-9`, `-`; no leading, trailing or double hyphen; not a Python standard-library module name (F013); equals the file name without `.py`. Same rules as the Agent Skills `name`, so export is lossless. Style: two or three kebab words, verb-noun or noun-noun (`json-peek`, `test-summary`). |
| `summary` | yes | author | One sentence, at most 100 characters. This is the index line and the exported skill description. Say what comes back. |
| `effects` | yes | author | One of `read`, `write-workspace`, `exec`, `network`, `external`. See below. |
| `example` | yes | author | TOML array of argument strings. `$FIXTURES` expands to `.blocks/fixtures/<name>`. `verify` runs the block with these arguments from the repo root; this is the only test. |
| `[returns]` | yes | author | `keys`: top-level keys the answer always contains; `verify` checks each is present. `doc`: free text. |
| `matches` | no | author | RE2 patterns tested against the body of an inline Python script the agent is about to run, never the shell command around it. A hit produces a hint. Expected on every block that replaces a common script shape. |
| `[params]` | no | author | One table per parameter: `type` (`str`, `int`, `float`, `bool`, `path`, `enum`), `required` or `default`, optional `help`, `min`, `max`, `values` for `enum`. `path` values must resolve inside the repo root; the runner rejects others. |
| `requires` | no | author | Executables the block needs on `PATH`, for example `["pytest"]`. `verify` and `run` fail early with a clear message when one is missing. |
| `[provenance]` | no | author | `created`, `source` (`registry:<name>@<version>` or `candidate:<fp>`), `sessions`. `promote` prints a proposed table for the agent to paste; nothing updates it afterwards. |
| `[stamp]` | no | tool | The only table the tool writes. Always last in the header. `verified`: 12 hex characters of the content hash below. `state`: absent means active; `"quarantined"` is written on failure and removed on the next success. |

The content hash is SHA-256, truncated to 12 hex, over: the block file with CRLF normalized to LF and
the lines from `# [stamp]` to the closing fence removed, together with every consecutive bare `#` line
directly above `# [stamp]` (or directly above the closing fence when there is no stamp), followed by the
sorted list of `(relative path, SHA-256)` for every file under `.blocks/fixtures/<name>/`. A fixture edit therefore
invalidates the stamp too. A block is in the index only when `verified` equals the current hash. The
tool rewrites the `[stamp]` table and nothing else, so verifying unchanged content is a no-op and the
committed file never churns. `blocks.lock` records the same hash for first-party blocks. `add` strips
the table before writing, and the embedded copies carry none.

## Effects

| Value | Meaning | Allowed by default |
|---|---|---|
| `read` | Reads files under the repo. No writes, no subprocesses, no network. | yes |
| `write-workspace` | Writes under the repo root. | yes |
| `exec` | Runs other programs in the repo: tests, builds, git, formatters. No network. | yes |
| `network` | Opens network connections. | no, until `allow_effects` in `config.toml` includes it |
| `external` | Changes state outside the repo: pushes, deploys, sends, deletes elsewhere. | no, until allowed |

Enforcement is in the runner. `caveman-blocks run` reads `allow_effects` from the committed
`config.toml` at `HEAD` (`git show HEAD:.blocks/config.toml`), never from the working tree, so an
uncommitted edit grants nothing. In CI, `verify --check --policy-ref origin/<base>` reads the policy
from the base branch, so a pull request cannot grant itself an effect either. `verify` applies the same
gate: a block whose effect is not allowed is reported as `skipped (effect not allowed)`, is not stamped,
and is not a failure. `add` prints the one-line grant a skipped block needs. The effect ladder for the
lint floor is `read < write-workspace < exec < network < external`; a block declares the highest rung it
reaches. Blocks write their logs to the state dir, not the repo, so running tests is `exec`, not
`write-workspace`.

Lint infers a floor from the source: importing `urllib` (except `urllib.parse`), `http.client`,
`socket`, `ssl`, `smtplib`, `ftplib`, `xmlrpc`, `requests`, `httpx`, `urllib3`, `aiohttp` or
`websockets`, or calling `asyncio.open_connection`, implies `network`; importing `subprocess`, `pty` or
`multiprocessing`, or calling `os.system`, `os.exec*`, `os.popen`, `os.spawn*` or
`asyncio.create_subprocess_*`, implies at least `exec`; `open` in a writing mode, `write_text`,
`write_bytes`, `shutil.copy*`, `shutil.move`, `shutil.rmtree`, `os.remove`, `os.makedirs` and any
`.unlink`, `.touch`, `.mkdir` or `.rename` call (`os` or `Path`) imply at least `write-workspace`.
`tempfile` does not. A declared effect below the floor fails lint.

Effects are hygiene, not a security boundary. A block run directly with `python3 .blocks/x.py`
bypasses the runner, and a header can lie. The controls for that are `CODEOWNERS` on `.blocks/`, pull
request review, and the fact that nothing in `.blocks/` runs unless an agent or person runs it.

## Calling convention

- Parameters arrive as `--name value` flags. Booleans are bare flags. The block parses them with
  `argparse`; lint checks that every declared parameter appears as `--name` in an `add_argument` call and
  that every `add_argument` name is declared. Flags passed to subprocesses are not inspected.
- stdout is exactly one JSON object and nothing else. Progress and logs go to stderr.
- Exit 0 means the answer is valid. Non-zero means failure, and the JSON object carries an `error`
  string. Usage errors exit 2.
- The answer is small. The runner caps stdout at 2 KB. On overflow the agent receives
  `{"_truncated": true, "_full": "<state-dir>/out/<id>.log", "head": "<first 1 KB>"}` and the full text
  is kept for 7 days. Design the answer so the cap never triggers.
- Idempotent and non-interactive: no prompts, no reliance on being run once.
- Composition: a block calls another block with `caveman-blocks run <name> ...` and parses the JSON.
  There is no shared library.
- Children a block leaves in its process group are killed when the block exits; a block whose child still
  holds stdout keeps its JSON answer after a 5-second wait.
- The runner invokes `python3 <file>` from the repo root with `PYTHONSAFEPATH=1` and `BLOCKS_OUT` set to the
  state dir's `out/` folder, where a block writes any full log it keeps; the file does not need an executable bit.
- The runner executes a block whether or not its stamp is current, because an agent iterates on a
  block before verifying it. When the stamp is missing, stale or quarantined, the answer gains
  `"_unverified": true`. `verify`'s own example runs emit no `run` events.
- `$FIXTURES` expands to the absolute path of `.blocks/fixtures/<name>`, so `file://$FIXTURES/x.json` is
  a valid URL. Fixtures are data only, at most 64 KiB per block, never test sources a user's test runner
  would collect.

## The eight rules

Canonical text of the authoring pack, written verbatim by `sync` at the top of the managed section. 
Changing a word here is a change to every repo's instruction file; it needs a pull request.

```
BLOCKS. Scripts in this repo are reusable blocks in .blocks/. Rules:
1. Check the block index below first. If a block fits, run it. If one almost fits, add a param to it.
2. A script you would run twice, or over ~10 lines, becomes a block, not a heredoc or a /tmp file.
3. Inputs are params. No hardcoded paths, ids, ports or dates.
4. Print one small JSON object: the answer, not the data. Filter, count and truncate in code.
5. Exit 0 on success, non-zero with {"error": ...} on failure.
6. Safe to re-run: idempotent, no prompts, cleans up after itself.
7. Header first: name, summary, params, effects, example. caveman-blocks lint checks it.
8. Compose: call existing blocks with caveman-blocks run instead of copying their code.
```

## Index and managed section

`sync` renders the section body, the eight rules followed by one line per indexed block sorted by name,
into `.blocks/INDEX.md` and, byte-identical, between `<!-- caveman-blocks:start -->` and
`<!-- caveman-blocks:end -->` in `AGENTS.md`. Index lines look like:

```
json-peek      --path <path> [--depth 2]       Shape of a JSON or JSONL file: keys, row count, one sample.
```

A block is indexed when it is active (no `state`), its `verified` stamp matches its content, and its
header passes validation (F003, F004, F005, F007, F008).
Sorted by name so the text is deterministic across machines. The whole section, rules included, has a
byte budget of 4 KiB enforced by `sync`; the default `index_max` is 20 lines and each line is cut at 110
characters on a word boundary with `…`. Exceeding the budget fails `sync` with a request to retire blocks
or shorten summaries. Quarantined and unverified blocks are not listed; `caveman-blocks stats` shows them.

`config.toml` key `section = "inline" | "import"` chooses how instruction files carry it. `inline` is the
default described above. `import` writes only the markers and `@.blocks/INDEX.md` into `CLAUDE.md` and
`GEMINI.md`, and the markers plus one line, `Blocks: read .blocks/INDEX.md before writing a script.`,
into `AGENTS.md`, for repos with a hard size cap on their instruction files. Codex has no import syntax,
so in `import` mode it pays one file read per session.

## Lint rules

| Rule | Check |
|---|---|
| F001 | Header present, within the first 20 lines, parses as TOML, one per file |
| F002 | No unknown keys |
| F003 | `name` valid and equal to the file name |
| F004 | `summary` one line, at most 100 characters, no control characters |
| F005 | `effects` is a known value and not below the inferred floor |
| F006 | `example` is a non-empty array of strings |
| F007 | Declared params and `add_argument` names match both ways (`--help` excepted); param names match `^[a-z][a-z0-9_-]{0,31}$` and `type` is one of str, int, float, bool, path, enum |
| F008 | `matches` patterns compile as RE2; at most 16 patterns of at most 200 bytes each |
| F009 | No absolute home paths (`/Users/`, `/home/`, `C:\Users\`) in the source |
| F010 | `json.dumps` appears and no other `print` or `sys.stdout.write` targets stdout |
| F011 | `returns.keys` non-empty |
| F012 | `--first-party` only (used by this repo's CI): stdlib-only imports, one import per line |
| F013 | `name` is not a Python standard-library module name (`json`, `csv`, `glob`, `http`, `time`, …): the runner starts `python3 .blocks/<name>.py`, which puts `.blocks/` first on `sys.path`, so `.blocks/json.py` would break `import json` in every block |
| F015 | Fixtures readable: regular files only, no symlinks, at most 64 KiB per block; an oversized block stays out of the index |
| W001 | Warning: current branch is the repository's default branch (see AGENT-PROMOTION) |
| W002 | Warning: a `matches` pattern looks like shell, not Python (contains a backslash-escaped pipe, `grep `, `tail `, `curl ` or `until `); it is tested only against the Python body, so it never fires |

## Export

`caveman-blocks export` writes a `SKILL.md` per indexed block for agents without the hook or the CLI:
`.claude/skills/<name>/SKILL.md` for Claude Code and `.agents/skills/<name>/SKILL.md` for clients that
read the shared path. Frontmatter is `name` and `description` (the summary); the body is the parameter
table and the call `caveman-blocks run <name> ...`, with `python3 .blocks/<name>.py ...` as the fallback
when the CLI is absent, noted as unenforced. Export output is generated and committed; `sync --check`
fails in CI when it is stale.

## Versioning the format

Additive changes add optional keys. A breaking change renames the fence type (`# /// block2`) and the
tool reads both for one major version. There is no version field.
