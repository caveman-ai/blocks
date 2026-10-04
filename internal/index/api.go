// Package index renders INDEX.md and the managed instruction-file section (docs/FORMAT.md).
package index

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

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
// A line is the name padded to 14 columns, the params padded to 34, then the summary; a column that
// overflows keeps one separating space. With no entries the body is Rules alone. No trailing newline.
func Body(entries []Entry, max, budget int) (string, error) {
	if len(entries) > max {
		return "", fmt.Errorf("index has %d blocks, index_max is %d: retire %d", len(entries), max, len(entries)-max)
	}
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	body := Rules
	if len(sorted) > 0 {
		lines := make([]string, len(sorted))
		for i, e := range sorted {
			lines[i] = cut(strings.TrimRight(pad(e.Name, 14)+pad(e.Params, 34)+e.Summary, " "))
		}
		body += "\n\n" + strings.Join(lines, "\n")
	}
	if len(body) > budget {
		return "", fmt.Errorf("index section is %d bytes, budget is %d: shorten summaries or retire blocks to cut %d bytes",
			len(body), budget, len(body)-budget)
	}
	return body, nil
}

// pad right-pads s to width runes, keeping at least one trailing space.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(1, width-utf8.RuneCountInString(s)))
}

// cut shortens line to LineWidth runes at the last word boundary, ending in "…".
func cut(line string) string {
	r := []rune(line)
	if len(r) <= LineWidth {
		return line
	}
	head := string(r[:LineWidth-1])
	if i := strings.LastIndex(head, " "); i > 0 {
		head = head[:i]
	}
	return strings.TrimRight(head, " ") + "…"
}

// Upsert returns file with body placed between the markers, replacing an existing section or
// appending a new one after a blank line. Text outside the markers is untouched.
// The section uses the file's line ending (CRLF when the file has any). Upsert is idempotent.
func Upsert(file []byte, body string) []byte {
	eol := "\n"
	if bytes.Contains(file, []byte("\r\n")) {
		eol = "\r\n"
	}
	inner := eol + strings.ReplaceAll(body, "\n", eol) + eol
	if s, e, ok := locate(file); ok {
		out := slices.Clone(file[:s])
		out = append(out, inner...)
		return append(out, file[e:]...)
	}
	out := slices.Clone(file)
	switch {
	case len(out) == 0:
	case bytes.HasSuffix(out, []byte("\n")):
		out = append(out, eol...)
	default:
		out = append(out, eol+eol...)
	}
	return append(out, MarkerStart+inner+MarkerEnd+eol...)
}

// Section returns the current text between the markers, and false when absent.
// The text is LF-normalized, without the line breaks that follow the start marker and precede the
// end marker, so Section(Upsert(f, body)) == body.
func Section(file []byte) (string, bool) {
	s, e, ok := locate(file)
	if !ok {
		return "", false
	}
	text := strings.ReplaceAll(string(file[s:e]), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\n")
	return strings.TrimSuffix(text, "\n"), true
}

// locate returns the byte range strictly between the markers: the first end marker and the last
// start marker before it, so a stray start marker earlier in the file is left alone.
func locate(file []byte) (start, end int, ok bool) {
	end = bytes.Index(file, []byte(MarkerEnd))
	if end < 0 {
		return 0, 0, false
	}
	start = bytes.LastIndex(file[:end], []byte(MarkerStart))
	if start < 0 {
		return 0, 0, false
	}
	return start + len(MarkerStart), end, true
}

// ImportBody is the body written to CLAUDE.md and GEMINI.md: the import line.
const ImportBody = "@.blocks/INDEX.md"

// PointerBody is the body written to AGENTS.md in import mode.
const PointerBody = "Blocks: read .blocks/INDEX.md before writing a script."
