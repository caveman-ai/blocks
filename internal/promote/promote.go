// Package promote ranks captured shapes, renders the promotion brief for the session agent, and
// retires blocks (docs/AGENT-PROMOTION.md). It never calls a model.
package promote

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/capture"
	"github.com/JuliusBrussee/caveman-blocks/internal/index"
	"github.com/JuliusBrussee/caveman-blocks/internal/registry"
)

// Rules is the eight-rule authoring pack, byte-identical to docs/FORMAT.md "The eight rules".
const Rules = index.Rules

// FormatURL points the agent at the block format.
const FormatURL = "https://github.com/JuliusBrussee/caveman-blocks/blob/main/docs/FORMAT.md"

// Steps is the verbatim step block from docs/AGENT-PROMOTION.md.
const Steps = `caveman-blocks lint .blocks/<name>.py
caveman-blocks verify <name>
caveman-blocks sync            # prints every file it changed
git add <those files> .blocks && git commit -m "blocks: add <name>"`

// now is the clock; tests replace it.
var now = time.Now

// Candidate is one shape not yet covered by a block.
type Candidate struct {
	FP       string
	Count    int
	Sessions int
	Last     time.Time
	Preview  string // first non-empty line of the latest script, at most 80 characters
}

// List returns shapes seen within since (0 = no window) whose fp no block covers, ranked by
// sessions then sightings, then fp for a stable order.
func List(shapes []capture.Shape, covered map[string]bool, since time.Duration) []Candidate {
	cutoff := now().Add(-since)
	var out []Candidate
	for _, sh := range shapes {
		if covered[sh.FP] || (since > 0 && sh.Last.Before(cutoff)) {
			continue
		}
		out = append(out, Candidate{FP: sh.FP, Count: sh.Count, Sessions: sh.Sessions, Last: sh.Last,
			Preview: preview(sh.Latest.Script)})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Sessions != b.Sessions {
			return a.Sessions > b.Sessions
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.FP < b.FP
	})
	return out
}

func preview(script string) string {
	for _, l := range strings.Split(script, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if r := []rune(l); len(r) > 80 {
				return string(r[:79]) + "…"
			}
			return l
		}
	}
	return ""
}

// Covered returns the fps named by blocks' provenance.source = "candidate:<fp>".
func Covered(blocks []*blockfile.Block) map[string]bool {
	m := map[string]bool{}
	for _, b := range blocks {
		if b == nil || b.Header.Provenance == nil {
			continue
		}
		if fp, ok := strings.CutPrefix(b.Header.Provenance.Source, "candidate:"); ok && fp != "" {
			m[fp] = true
		}
	}
	return m
}

// param is a literal position that varied across sightings.
type param struct {
	pos    int
	name   string
	typ    string
	values []string // distinct, in order first seen
}

// params compares the literal vectors position-wise; a position with more than one distinct value
// is a likely parameter.
func params(vectors [][]string) []param {
	width := 0
	for _, v := range vectors {
		width = max(width, len(v))
	}
	var out []param
	used := map[string]int{}
	for i := range width {
		var vals []string
		seen := map[string]bool{}
		for _, v := range vectors {
			if i < len(v) && !seen[v[i]] {
				seen[v[i]] = true
				vals = append(vals, v[i])
			}
		}
		if len(vals) < 2 {
			continue
		}
		typ := guessType(vals)
		base := map[string]string{"int": "n", "float": "x", "path": "path", "str": "value"}[typ]
		used[base]++
		name := base
		if used[base] > 1 {
			name += strconv.Itoa(used[base])
		}
		out = append(out, param{pos: i, name: name, typ: typ, values: vals})
	}
	return out
}

func guessType(vals []string) string {
	all := func(ok func(string) bool) bool {
		for _, v := range vals {
			if !ok(v) {
				return false
			}
		}
		return true
	}
	switch {
	case all(func(v string) bool { _, err := strconv.Atoi(v); return err == nil }):
		return "int"
	case all(func(v string) bool { _, err := strconv.ParseFloat(v, 64); return err == nil }):
		return "float"
	case all(func(v string) bool { return strings.ContainsAny(v, "/.") }):
		return "path"
	}
	return "str"
}

var (
	importRe = regexp.MustCompile(`(?m)^\s*(?:import|from)\s+([A-Za-z_]\w*)`)
	// verbs maps the first matching call in the body to a verb, in priority order.
	// ponytail: naive keyword table; the agent renames freely.
	verbs = []struct {
		re   *regexp.Regexp
		verb string
	}{
		{regexp.MustCompile(`\btime\.sleep\(`), "wait"},
		{regexp.MustCompile(`\b(urllib|requests|httpx)\b`), "fetch"},
		{regexp.MustCompile(`\bsubprocess\b`), "run"},
		{regexp.MustCompile(`\b(Counter|statistics|defaultdict)\b`), "count"},
		{regexp.MustCompile(`\bre\.(findall|search|finditer|match)\(`), "find"},
		{regexp.MustCompile(`\b(json\.loads?|csv\.|yaml\.|open)\(|\bsqlite3\b`), "read"},
	}
	skipNoun = map[string]bool{"sys": true, "os": true, "re": true, "argparse": true, "typing": true,
		"collections": true, "pathlib": true, "time": true, "subprocess": true, "__future__": true,
		"statistics": true, "glob": true, "itertools": true, "functools": true}
)

// ProposeName derives a verb-noun kebab name from the script's calls and imports, falling back to
// "block-<fp[:6]>".
func ProposeName(sh capture.Shape) string {
	body := sh.Latest.Script
	verb := ""
	for _, v := range verbs {
		if v.re.MatchString(body) {
			verb = v.verb
			break
		}
	}
	noun := ""
	for _, m := range importRe.FindAllStringSubmatch(body, -1) {
		if n := strings.ToLower(m[1]); !skipNoun[n] {
			noun = strings.ReplaceAll(n, "_", "-")
			break
		}
	}
	if name := verb + "-" + noun; verb != "" && noun != "" && registry.ValidName(name) {
		return name
	}
	fp := sh.FP
	if len(fp) > 6 {
		fp = fp[:6]
	}
	return "block-" + fp
}

// Brief renders the plain-text promotion brief for one shape. An empty proposedName is derived.
func Brief(sh capture.Shape, proposedName string) string {
	if proposedName == "" {
		proposedName = ProposeName(sh)
	}
	ps := params(sh.Literals)
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("Promotion brief for shape %s: %d sightings in %d sessions, last %s (measured).\n\n",
		sh.FP, sh.Count, sh.Sessions, sh.Last.UTC().Format("2006-01-02"))

	w("1. Latest sighting (scrubbed)\n\n")
	w("Command: %s\n\n", sh.Latest.CommandHead)
	w("```python\n%s\n```\n\n", strings.Trim(sh.Latest.Script, "\n"))
	if len(ps) == 0 {
		w("No literal differs across sightings.\n\n")
	} else {
		w("Literals that differ across sightings (likely parameters):\n")
		for _, p := range ps {
			w("- literal %d -> --%s (%s): %s\n", p.pos+1, p.name, p.typ, quoteList(p.values, 10))
		}
		w("\n")
	}

	w("2. Proposal\n\n")
	w("Name: %s (two or three kebab words, verb-noun or noun-noun; rename if a better one fits)\n\n", proposedName)
	if len(ps) > 0 {
		w("# [params]\n")
		for _, p := range ps {
			w("# %s = { type = %q, required = true, help = %s }\n", p.name, p.typ,
				tomlString("seen: "+strings.Join(trimValues(p.values, 3), ", ")))
		}
		w("#\n")
	}
	w("# [provenance]\n")
	w("# created = %q\n", now().Format("2006-01-02"))
	w("# source = %q\n", "candidate:"+sh.FP)
	w("# sessions = %d\n\n", sh.Sessions)

	w("3. Rules (format: %s)\n\n%s\n\n", FormatURL, Rules)

	w("4. Steps\n\n")
	w("Write .blocks/%s.py, add a fixture under .blocks/fixtures/%s/ if the example needs one, then, with <name> = %s:\n\n",
		proposedName, proposedName, proposedName)
	w("%s\n\n", Steps)
	w("Commit on the current branch, which must not be the repository's default branch.\n")
	return b.String()
}

func trimValues(vals []string, n int) []string {
	out := make([]string, 0, n+1)
	for i, v := range vals {
		if i == n {
			out = append(out, "…")
			break
		}
		if r := []rune(v); len(r) > 40 {
			v = string(r[:39]) + "…"
		}
		out = append(out, v)
	}
	return out
}

func quoteList(vals []string, n int) string {
	q := make([]string, 0, len(vals))
	for _, v := range trimValues(vals, n) {
		q = append(q, strconv.Quote(v))
	}
	return strings.Join(q, ", ")
}

// tomlString renders s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Retire removes .blocks/<name>.py, .blocks/fixtures/<name>/ and the blocks.lock entry if present.
// The caller runs sync afterwards.
func Retire(root, name string) error {
	if !registry.ValidName(name) {
		return fmt.Errorf("invalid block name %q", name)
	}
	blocks := filepath.Join(root, ".blocks")
	if err := os.Remove(filepath.Join(blocks, name+".py")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("no block %q in %s: %w", name, blocks, err)
		}
		return err
	}
	if err := os.RemoveAll(filepath.Join(blocks, "fixtures", name)); err != nil {
		return err
	}
	lock, err := registry.Lock(root)
	if err != nil {
		return err
	}
	if _, ok := lock[name]; !ok {
		return nil
	}
	delete(lock, name)
	return registry.WriteLock(root, lock)
}
