package blockfile

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// fenceRE is the PEP 723 reference regex, verbatim. It runs on the LF-normalized source, so CRLF
// files parse too (the reference regex alone would reject them).
var fenceRE = regexp.MustCompile(`(?m)^# /// (?P<type>[a-zA-Z0-9-]+)$\s(?P<content>(^#(| .*)$\s)+)^# ///$`)

// maxHeaderLine is the last 1-based line the opening fence may sit on.
const maxHeaderLine = 20

// Error renders a finding as an error. Parse returns F001 findings wrapped with the path, so
// callers can recover the code with errors.As.
func (f Finding) Error() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s line %d: %s", f.Code, f.Line, f.Message)
	}
	return f.Code + ": " + f.Message
}

func parse(path string, src []byte) (*Block, error) {
	fail := func(line int, format string, args ...any) error {
		name := path
		if name == "" {
			name = "<block>"
		}
		return fmt.Errorf("%s: %w", name, Finding{Code: "F001", Message: fmt.Sprintf(format, args...), Line: line})
	}
	if path != "" && filepath.Ext(path) != ".py" {
		return nil, fail(0, "v0 blocks are Python files ending in .py")
	}
	norm := normalize(src)
	lines := strings.Split(string(norm), "\n")
	if strings.HasPrefix(lines[0], "#!") && !strings.Contains(lines[0], "python") {
		return nil, fail(1, "v0 blocks are Python only; shebang %q is not python", lines[0])
	}

	var found [][]int
	for _, m := range fenceRE.FindAllSubmatchIndex(norm, -1) {
		if string(norm[m[2]:m[3]]) == "block" {
			found = append(found, m)
		}
	}
	switch len(found) {
	case 0:
		return nil, fail(0, "no block header: expected a `# /// block` ... `# ///` fence")
	case 1:
	default:
		return nil, fail(lineOf(norm, found[1][0])+1, "second block header; a file has exactly one")
	}
	start, end := lineOf(norm, found[0][0]), lineOf(norm, found[0][1])
	if start+1 > maxHeaderLine {
		return nil, fail(start+1, "block header starts on line %d; it must be within the first %d lines", start+1, maxHeaderLine)
	}

	doc := headerTOML(lines, start, end)
	raw, err := decode(doc)
	if err != nil {
		line := 0
		var de *toml.DecodeError
		if errors.As(err, &de) {
			row, _ := de.Position()
			line = start + 1 + row
		}
		return nil, fail(line, "header is not valid TOML: %v", err)
	}

	stampLine := -1
	for i := start + 1; i < end; i++ {
		c := headerLine(lines[i])
		if c == "[stamp]" {
			stampLine = i
		} else if stampLine >= 0 && strings.HasPrefix(c, "[") {
			return nil, fail(i+1, "[stamp] must be the last table in the header")
		}
	}
	if _, ok := raw["stamp"]; ok && stampLine < 0 {
		return nil, fail(start+1, "stamp must be written as a `# [stamp]` table")
	}

	return &Block{
		Path:       path,
		Header:     toHeader(raw, paramOrder(doc)),
		Source:     src,
		FenceStart: start,
		FenceEnd:   end,
		StampLine:  stampLine,
	}, nil
}

// normalize converts CRLF to LF. Line indexes are unchanged by it.
func normalize(src []byte) []byte { return bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")) }

func lineOf(b []byte, off int) int { return bytes.Count(b[:off], []byte("\n")) }

// headerLine returns a header line's content without the `#` prefix, trimmed.
func headerLine(line string) string { return strings.TrimSpace(strings.TrimPrefix(line, "#")) }

// headerTOML strips the PEP 723 prefix (`# ` or `#`) from the lines strictly between the fences.
func headerTOML(lines []string, start, end int) []byte {
	var b strings.Builder
	for _, l := range lines[start+1 : end] {
		if strings.HasPrefix(l, "# ") {
			l = l[2:]
		} else {
			l = strings.TrimPrefix(l, "#")
		}
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func decode(doc []byte) (map[string]any, error) {
	raw := map[string]any{}
	err := toml.Unmarshal(doc, &raw)
	return raw, err
}

// rawHeader re-decodes the header of a parsed block for rules that need the untyped values.
func rawHeader(b *Block) (map[string]any, error) {
	lines := strings.Split(string(normalize(b.Source)), "\n")
	if b.FenceStart < 0 || b.FenceEnd >= len(lines) || b.FenceEnd <= b.FenceStart {
		return nil, errors.New("fence lines out of range")
	}
	return decode(headerTOML(lines, b.FenceStart, b.FenceEnd))
}

// paramOrder returns [params] entry names in document order. The decoded map loses order, so this
// walks the same TOML with go-toml's expression parser and records the second key segment of
// every table header or key under `params`. It covers `[params]` with inline tables,
// `[params.x]` sub-tables and dotted keys alike.
func paramOrder(doc []byte) []string {
	var p unstable.Parser
	p.Reset(doc)
	var table, out []string
	add := func(path []string) {
		if len(path) >= 2 && path[0] == "params" && !slices.Contains(out, path[1]) {
			out = append(out, path[1])
		}
	}
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = keyParts(e.Key())
			add(table)
		case unstable.KeyValue:
			add(append(slices.Clone(table), keyParts(e.Key())...))
		}
	}
	return out
}

func keyParts(it unstable.Iterator) []string {
	var parts []string
	for it.Next() {
		parts = append(parts, string(it.Node().Data))
	}
	return parts
}

// toHeader converts decoded TOML leniently: a value of the wrong type becomes the zero value and
// Lint reports it, so a malformed header still lints.
func toHeader(raw map[string]any, order []string) Header {
	h := Header{
		Name:     str(raw["name"]),
		Summary:  str(raw["summary"]),
		Effects:  Effect(str(raw["effects"])),
		Example:  strs(raw["example"]),
		Matches:  strs(raw["matches"]),
		Requires: strs(raw["requires"]),
	}
	if r, ok := raw["returns"].(map[string]any); ok {
		h.Returns = Returns{Keys: strs(r["keys"]), Doc: str(r["doc"])}
	}
	if p, ok := raw["provenance"].(map[string]any); ok {
		n, _ := p["sessions"].(int64)
		h.Provenance = &Provenance{Created: str(p["created"]), Source: str(p["source"]), Sessions: int(n)}
	}
	if s, ok := raw["stamp"].(map[string]any); ok {
		h.Stamp = &Stamp{Verified: str(s["verified"]), State: str(s["state"])}
	}
	params, _ := raw["params"].(map[string]any)
	for _, name := range order {
		t, _ := params[name].(map[string]any)
		req, _ := t["required"].(bool)
		h.Params = append(h.Params, Param{
			Name:     name,
			Type:     str(t["type"]),
			Required: req,
			Default:  t["default"],
			Help:     str(t["help"]),
			Min:      num(t["min"]),
			Max:      num(t["max"]),
			Values:   strs(t["values"]),
		})
	}
	return h
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func strs(v any) []string {
	a, _ := v.([]any)
	var out []string
	for _, e := range a {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func num(v any) *float64 {
	var f float64
	switch n := v.(type) {
	case int64:
		f = float64(n)
	case float64:
		f = n
	default:
		return nil
	}
	return &f
}

var knownKeys = map[string][]string{
	"":           {"name", "summary", "effects", "example", "matches", "returns", "params", "requires", "provenance", "stamp"},
	"returns":    {"keys", "doc"},
	"provenance": {"created", "source", "sessions"},
	"stamp":      {"verified", "state"},
	"params.*":   {"type", "required", "default", "help", "min", "max", "values"},
}

// unknownKeys lists dotted paths of keys the format does not define, sorted.
func unknownKeys(raw map[string]any) []string {
	var out []string
	check := func(prefix string, m map[string]any, allowed []string) {
		for k := range m {
			if !slices.Contains(allowed, k) {
				out = append(out, prefix+k)
			}
		}
	}
	check("", raw, knownKeys[""])
	for _, t := range []string{"returns", "provenance", "stamp"} {
		if m, ok := raw[t].(map[string]any); ok {
			check(t+".", m, knownKeys[t])
		}
	}
	if params, ok := raw["params"].(map[string]any); ok {
		for name, v := range params {
			if m, ok := v.(map[string]any); ok {
				check("params."+name+".", m, knownKeys["params.*"])
			}
		}
	}
	sort.Strings(out)
	return out
}
