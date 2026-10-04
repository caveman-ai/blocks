// Package index renders INDEX.md and the managed instruction-file section (docs/FORMAT.md).
package index

// Markers bound the managed section in instruction files.
const (
	MarkerStart = "<!-- caveman-blocks:start -->"
	MarkerEnd   = "<!-- caveman-blocks:end -->"
	// DefaultMax is the default index_max. DefaultBudget is the section byte budget.
	DefaultMax    = 20
	DefaultBudget = 4096
	LineWidth     = 110
)

// Rules is the canonical eight-rule authoring pack (docs/FORMAT.md, "The eight rules"). Verbatim.
const Rules = `BLOCKS. Scripts in this repo are reusable blocks in .blocks/. Rules:
1. Check the block index below first. If a block fits, run it. If one almost fits, add a param to it.
2. A script you would run twice, or over ~10 lines, becomes a block, not a heredoc or a /tmp file.
3. Inputs are params. No hardcoded paths, ids, ports or dates.
4. Print one small JSON object: the answer, not the data. Filter, count and truncate in code.
5. Exit 0 on success, non-zero with {"error": ...} on failure.
6. Safe to re-run: idempotent, no prompts, cleans up after itself.
7. Header first: name, summary, params, effects, example. caveman-blocks lint checks it.
8. Compose: call existing blocks with caveman-blocks run instead of copying their code.`

// Entry is one indexed block.
type Entry struct {
	Name    string
	Params  string // from blockfile.ParamsColumn
	Summary string
}

// Mode is the section mode from config.toml.
type Mode string

const (
	ModeInline Mode = "inline"
	ModeImport Mode = "import"
)

// Body renders the full section body: Rules, a blank line, then one line per entry sorted by name,
// each cut at LineWidth on a word boundary with "…". Errors when len(entries) > max or the body
// exceeds budget bytes.
func Body(entries []Entry, max, budget int) (string, error) { panic("index: not implemented") }

// Upsert returns file with body placed between the markers, replacing an existing section or
// appending a new one after a blank line. Text outside the markers is untouched.
func Upsert(file []byte, body string) []byte { panic("index: not implemented") }

// Section returns the current text between the markers, and false when absent.
func Section(file []byte) (string, bool) { panic("index: not implemented") }

// ImportBody is the body written to CLAUDE.md and GEMINI.md: the import line.
const ImportBody = "@.blocks/INDEX.md"

// PointerBody is the body written to AGENTS.md in import mode.
const PointerBody = "Blocks: read .blocks/INDEX.md before writing a script."
