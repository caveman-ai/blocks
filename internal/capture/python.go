package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

const (
	tName = iota + 1 // identifier or dotted name, e.g. json.load
	tStr             // string literal, raw text with prefix and quotes
	tNum
	tOp // one punctuation byte
	tNL // end of a physical line
)

type pyTok struct {
	kind uint8
	s    string // a slice of the body
}

var (
	callIndex = map[string]int{} // call name -> position in callNames
	callOrder []int              // callNames positions sorted by name
)

func init() {
	if len(callNames) > 32 {
		panic("capture: callNames outgrew the counts array")
	}
	for i, n := range callNames {
		callIndex[n] = i
		callOrder = append(callOrder, i)
	}
	sort.Slice(callOrder, func(a, b int) bool { return callNames[callOrder[a]] < callNames[callOrder[b]] })
}

// lexPy splits Python source into tokens. Comments and whitespace are dropped.
func lexPy(src string) []pyTok {
	toks := make([]pyTok, 0, len(src)/2+4) // typical code runs one token per 3 bytes
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '\n':
			if len(toks) > 0 && toks[len(toks)-1].kind != tNL {
				toks = append(toks, pyTok{tNL, "\n"})
			}
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			i++
		case c == '\\' && i+1 < n && src[i+1] == '\n':
			i += 2
		case c == '#':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '"' || c == '\'':
			j := strEnd(src, i)
			toks = append(toks, pyTok{tStr, src[i:j]})
			i = j
		case isDigit(c) || c == '.' && i+1 < n && isDigit(src[i+1]):
			hexLit := c == '0' && i+1 < n && src[i+1]|0x20 == 'x'
			j := i + 1
			for j < n && (isIdent(src[j]) || src[j] == '.' ||
				!hexLit && (src[j] == '+' || src[j] == '-') && src[j-1]|0x20 == 'e') {
				j++
			}
			toks = append(toks, pyTok{tNum, src[i:j]})
			i = j
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdent(src[j]) {
				j++
			}
			if j < n && (src[j] == '"' || src[j] == '\'') && j-i <= 2 && strings.Trim(src[i:j], "rRbBuUfF") == "" {
				k := strEnd(src, j)
				toks = append(toks, pyTok{tStr, src[i:k]})
				i = k
				continue
			}
			for j+1 < n && src[j] == '.' && isIdentStart(src[j+1]) {
				j += 2
				for j < n && isIdent(src[j]) {
					j++
				}
			}
			toks = append(toks, pyTok{tName, src[i:j]})
			i = j
		default:
			toks = append(toks, pyTok{tOp, src[i : i+1]})
			i++
		}
	}
	return toks
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || c|0x20 >= 'a' && c|0x20 <= 'z' || c >= 0x80 }
func isIdent(c byte) bool      { return isIdentStart(c) || isDigit(c) }

// strEnd returns the index just past the string literal whose quote is at i.
func strEnd(src string, i int) int {
	q, n := src[i], len(src)
	j := i + 1
	if i+2 < n && src[i+1] == q && src[i+2] == q {
		for j = i + 3; j < n; j++ {
			if src[j] == '\\' {
				j++
			} else if src[j] == q && j+2 < n && src[j+1] == q && src[j+2] == q {
				return j + 3
			}
		}
		return n
	}
	for ; j < n; j++ {
		switch src[j] {
		case '\\':
			j++
		case q:
			return j + 1
		case '\n':
			return j
		}
	}
	return n
}

// strContent strips the prefix and quotes of a raw string literal.
func strContent(raw string) string {
	p := strings.IndexAny(raw, `"'`)
	q := raw[p]
	if len(raw)-p >= 6 && raw[p+1] == q && raw[p+2] == q && strings.HasSuffix(raw, raw[p:p+3]) {
		return raw[p+3 : len(raw)-3]
	}
	if len(raw)-p >= 2 && raw[len(raw)-1] == q {
		return raw[p+1 : len(raw)-1]
	}
	return raw[p+1:]
}

// pathLike reports whether a string literal names a file: /a/b, ./x, ../x, ~/x or a/b.ext.
func pathLike(c string) bool {
	if c == "" || strings.ContainsAny(c, " \t\n") || strings.Contains(c, "://") {
		return false
	}
	for _, pre := range []string{"/", "./", "../", "~/"} {
		if strings.HasPrefix(c, pre) {
			return strings.IndexFunc(c, func(r rune) bool { return r != '/' && r != '.' && r != '~' }) >= 0
		}
	}
	slash := strings.LastIndexByte(c, '/')
	dot := strings.LastIndexByte(c, '.')
	return slash > 0 && dot > slash+1 && dot < len(c)-1
}

// analyze computes the script_sha, fp and edit flag of a Python body.
func analyze(body string) (sha, fp string, edit bool) {
	toks := lexPy(body)

	norm := make([]byte, 0, len(body))
	for _, t := range toks {
		if t.kind == tNL {
			continue
		}
		if len(norm) > 0 {
			norm = append(norm, ' ')
		}
		switch t.kind {
		case tStr:
			// Paths in Python are always quoted, so they reduce to S like any string: a script that
			// switches 'a.json' for 'runs/b.json' is still the same script.
			norm = append(norm, 'S')
		case tNum:
			norm = append(norm, 'N')
		default:
			norm = append(norm, t.s...)
		}
	}
	sum := sha256.Sum256(norm)
	sha = hex.EncodeToString(sum[:])

	var imports []string
	var counts [32]int // len(callNames) <= 32
	hits := false
	start := true
	for j := 0; j < len(toks); j++ {
		t := toks[j]
		switch {
		case t.kind == tNL || t.kind == tOp && (t.s == ";" || t.s == ":"):
			start = true
			continue
		case t.kind == tName && start && (t.s == "import" || t.s == "from"):
			j, imports = parseImport(toks, j, imports)
		case t.kind == tName:
			hits = countCalls(t.s, &counts) || hits
		}
		start = false
	}
	if len(imports) > 0 || hits {
		sort.Strings(imports)
		var b strings.Builder
		b.WriteString("py|")
		prev := ""
		for i, m := range imports {
			if i > 0 && m == prev {
				continue
			}
			if prev != "" {
				b.WriteByte(',')
			}
			b.WriteString(m)
			prev = m
		}
		b.WriteByte('|')
		first := true
		for _, ix := range callOrder {
			if counts[ix] == 0 {
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteString(callNames[ix])
			b.WriteString([...]string{":1", ":2", ":3+"}[min(counts[ix], 3)-1])
		}
		h := sha256.Sum256([]byte(b.String()))
		fp = hex.EncodeToString(h[:6])
	}
	return sha, fp, isEdit(toks)
}

// safeLiterals is the literal vector of body: string contents and numbers in order of appearance,
// taken from the scrubbed body. A literal that holds the scrub marker, or that sits inside or
// contains a span Scrub replaced anywhere in the body, becomes the marker, so a secret repeated
// outside an assignment cannot leak. Positions are kept for promote's position-wise comparison.
func safeLiterals(body string) []string {
	clean, spans := scrub(body)
	var lits []string
	for _, t := range lexPy(clean) {
		var l string
		switch t.kind {
		case tStr:
			l = strContent(t.s)
		case tNum:
			l = t.s
		default:
			continue
		}
		if strings.Contains(l, scrubbed) || inSpan(l, spans) {
			l = scrubbed
		}
		lits = append(lits, l)
	}
	return lits
}

// inSpan reports a literal that contains a scrubbed span, or is a 4+ character piece of one. Shorter
// pieces (a 3 that also occurs in a key) are too common to be a leak and are kept.
func inSpan(l string, spans []string) bool {
	for _, sp := range spans {
		if len(l) >= 4 && strings.Contains(sp, l) || strings.Contains(l, sp) {
			return true
		}
	}
	return false
}

// parseImport reads `import a.b as c, d` or `from a.b import x` at toks[j] and returns the index of
// the last token it consumed plus the top-level module names appended to mods.
func parseImport(toks []pyTok, j int, mods []string) (int, []string) {
	top := func(s string) string {
		if i := strings.IndexByte(s, '.'); i >= 0 {
			return s[:i]
		}
		return s
	}
	end := func(k int) bool {
		return k >= len(toks) || toks[k].kind == tNL || toks[k].kind == tOp && toks[k].s == ";"
	}
	if toks[j].s == "from" {
		if j+1 < len(toks) && toks[j+1].kind == tName {
			mods = append(mods, top(toks[j+1].s)) // relative imports (from . import x) are skipped
		}
		for j+1 < len(toks) && !end(j+1) {
			j++
		}
		return j, mods
	}
	for k := j + 1; k < len(toks); k++ {
		if toks[k].kind != tName {
			return k - 1, mods
		}
		mods = append(mods, top(toks[k].s))
		if k+2 < len(toks) && toks[k+1].s == "as" {
			k += 2
		}
		if k+1 >= len(toks) || toks[k+1].kind != tOp || toks[k+1].s != "," {
			return k, mods
		}
		k++
	}
	return len(toks) - 1, mods
}

// countCalls counts table names that occur in a dotted name as whole segments: "glob.glob" counts
// glob twice, "json.loads" counts json.loads but not json.load.
func countCalls(name string, counts *[32]int) bool {
	hit := false
	for start := 0; ; {
		end1 := len(name)
		if i := strings.IndexByte(name[start:], '.'); i >= 0 {
			end1 = start + i
		}
		if ix, ok := callIndex[name[start:end1]]; ok {
			counts[ix]++
			hit = true
		}
		if end1 == len(name) {
			return hit
		}
		end2 := len(name)
		if i := strings.IndexByte(name[end1+1:], '.'); i >= 0 {
			end2 = end1 + 1 + i
		}
		if ix, ok := callIndex[name[start:end2]]; ok {
			counts[ix]++
			hit = true
		}
		start = end1 + 1
	}
}

// isEdit reports whether the body reads a file and writes the same path: open(p) ... open(p, "w"),
// Path(p).read_text() ... Path(p).write_text(), p.read_text() ... p.write_text(), or
// fileinput with inplace=True. Paths compare as source text after resolving `name = "literal"`.
func isEdit(toks []pyTok) bool {
	var reads, writes []string
	var vars map[string]string
	at := func(k int, kind uint8, s string) bool {
		return k < len(toks) && toks[k].kind == kind && toks[k].s == s
	}
	for j, t := range toks {
		if t.kind != tName {
			continue
		}
		switch {
		case t.s == "open" || t.s == "io.open" || t.s == "codecs.open":
			if !at(j+1, tOp, "(") {
				continue
			}
			key, mode := callArgs(toks, j+2)
			if key == "" {
				continue
			}
			if mode == "" || strings.ContainsAny(mode, "r+") {
				reads = append(reads, key)
			}
			if strings.ContainsAny(mode, "w+") {
				writes = append(writes, key)
			}
		case t.s == "Path" || t.s == "pathlib.Path":
			if !at(j+1, tOp, "(") {
				continue
			}
			key, close := firstArg(toks, j+2)
			if key == "" || !at(close+1, tOp, ".") || close+2 >= len(toks) {
				continue
			}
			reads, writes = pathMethod(toks[close+2].s, key, reads, writes)
		case t.s == "inplace" && at(j+1, tOp, "=") && at(j+2, tName, "True"):
			return true
		default:
			if dot := strings.LastIndexByte(t.s, '.'); dot > 0 {
				reads, writes = pathMethod(t.s[dot+1:], t.s[:dot], reads, writes)
			}
			if at(j+1, tOp, "=") && j+2 < len(toks) && toks[j+2].kind == tStr &&
				(j+3 == len(toks) || toks[j+3].kind == tNL || at(j+3, tOp, ";")) {
				if vars == nil {
					vars = map[string]string{}
				}
				vars[t.s] = strContent(toks[j+2].s)
			}
		}
	}
	resolve := func(k string) string {
		if v, ok := vars[k]; ok {
			return v
		}
		return k
	}
	for _, w := range writes {
		for _, r := range reads {
			if resolve(w) == resolve(r) {
				return true
			}
		}
	}
	return false
}

func pathMethod(method, key string, reads, writes []string) ([]string, []string) {
	switch method {
	case "read_text", "read_bytes":
		reads = append(reads, key)
	case "write_text", "write_bytes":
		writes = append(writes, key)
	}
	return reads, writes
}

// firstArg returns the first call argument starting at toks[k] as source text (a lone string literal
// is reduced to its content) and the index of the token that ends it.
func firstArg(toks []pyTok, k int) (string, int) {
	depth := 0
	var b strings.Builder
	start := k
	for ; k < len(toks); k++ {
		t := toks[k]
		if t.kind == tOp {
			switch t.s {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth == 0 {
					return argKey(toks[start:k], &b), k
				}
				depth--
			case ",":
				if depth == 0 {
					return argKey(toks[start:k], &b), k
				}
			}
		}
	}
	return "", k
}

func argKey(ts []pyTok, b *strings.Builder) string {
	if len(ts) == 1 && ts[0].kind == tStr {
		return strContent(ts[0].s)
	}
	for _, t := range ts {
		if t.kind != tNL {
			b.WriteString(t.s)
		}
	}
	return b.String()
}

// callArgs returns open()'s path argument and its mode: the second positional string or mode=.
func callArgs(toks []pyTok, k int) (key, mode string) {
	key, end := firstArg(toks, k)
	if end >= len(toks) || toks[end].s != "," {
		return key, ""
	}
	depth := 0
	for k = end + 1; k < len(toks); k++ {
		t := toks[k]
		switch {
		case t.kind == tOp && (t.s == "(" || t.s == "[" || t.s == "{"):
			depth++
		case t.kind == tOp && (t.s == ")" || t.s == "]" || t.s == "}"):
			if depth == 0 {
				return key, ""
			}
			depth--
		case t.kind == tStr && depth == 0:
			prev := toks[k-1]
			if prev.s == "," && k-1 == end || prev.s == "=" && toks[k-2].s == "mode" {
				return key, strContent(t.s)
			}
		}
	}
	return key, ""
}
