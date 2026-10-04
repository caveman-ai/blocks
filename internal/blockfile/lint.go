package blockfile

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var (
	nameRE      = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	homeRE      = regexp.MustCompile(`/Users/|/home/|(?i)[a-z]:\\{1,2}users\\`)
	importRE    = regexp.MustCompile(`^\s*import\s+(.+)$`)
	fromRE      = regexp.MustCompile(`^\s*from\s+(\S+)\s+import\s+(.+)$`)
	printRE     = regexp.MustCompile(`\bprint\(|\bsys\.stdout\.write\(`)
	stderrRE    = regexp.MustCompile(`\bfile\s*=\s*sys\.stderr\b`)
	addArgRE    = regexp.MustCompile(`\badd_argument\(`)
	flagRE      = regexp.MustCompile(`["'](--[A-Za-z0-9][A-Za-z0-9_-]*)["']`)
	openRE      = regexp.MustCompile(`\bopen\(`)
	writeCallRE = regexp.MustCompile(`\.write_(text|bytes)\(|\.(unlink|touch|mkdir|rename)\(|\bos\.(remove|makedirs)\(|\bshutil\.(copy\w*|move|rmtree)\(`)
	execCallRE  = regexp.MustCompile(`\bos\.(system|exec\w*|popen|spawn\w*)\(|\basyncio\.create_subprocess_\w+\(`)
	netCallRE   = regexp.MustCompile(`\basyncio\.open_connection\(`)
	shellRE     = regexp.MustCompile(`\\\||\b(grep|tail|curl|until) `)
	clientRE    = regexp.MustCompile(`\bclient\b`)
)

// pyImport is one imported module and the 1-based line of its statement.
type pyImport struct {
	module string // dotted, as written; relative imports keep their dots
	names  string // the `from x import <names>` part, "" for `import x`
	line   int
	multi  bool // `import a, b` on one line
}

func lint(b *Block, opts LintOptions) []Finding {
	var out []Finding
	add := func(code string, line int, format string, args ...any) {
		out = append(out, Finding{Code: code, Message: fmt.Sprintf(format, args...), Line: line})
	}
	h := b.Header
	lines := strings.Split(string(normalize(b.Source)), "\n")
	hline := func(key string) int { // 1-based line of the first header line that sets key, or 0
		re := regexp.MustCompile(`(^|[\s{,])` + regexp.QuoteMeta(key) + `\s*=`)
		for i := b.FenceStart + 1; i < b.FenceEnd && i < len(lines); i++ {
			if re.MatchString(headerLine(lines[i])) {
				return i + 1
			}
		}
		return 0
	}

	raw, err := rawHeader(b)
	if err != nil {
		add("F001", b.FenceStart+1, "header does not parse: %v", err)
		return out
	}
	for _, k := range unknownKeys(raw) {
		add("F002", hline(k[strings.LastIndex(k, ".")+1:]), "unknown key %q", k)
	}

	switch {
	case !nameRE.MatchString(h.Name) || len(h.Name) > 64:
		add("F003", hline("name"), "name %q must be 1-64 chars of a-z, 0-9 and single inner hyphens", h.Name)
	case b.Path != "" && strings.TrimSuffix(filepath.Base(b.Path), ".py") != h.Name:
		add("F003", hline("name"), "name %q must equal the file name %q", h.Name, filepath.Base(b.Path))
	}
	if stdlib[h.Name] { // python3 .blocks/x.py puts .blocks/ first on sys.path, so json.py breaks `import json`
		add("F013", hline("name"), "name %q shadows a Python standard-library module", h.Name)
	}

	if strings.TrimSpace(h.Summary) == "" {
		add("F004", hline("summary"), "summary is required")
	} else if err := checkSummary(h.Summary); err != nil {
		add("F004", hline("summary"), "%v", err)
	}

	code := codeText(lines, b.FenceStart, b.FenceEnd)
	imports := parseImports(code)
	if h.Effects.Rank() < 0 {
		add("F005", hline("effects"), "effects %q must be one of read, write-workspace, exec, network, external", h.Effects)
	} else if floor, why, line := effectFloor(code, imports); h.Effects.Rank() < floor.Rank() {
		add("F005", line, "effects %q is below the inferred floor %q (%s)", h.Effects, floor, why)
	}

	if ex, ok := raw["example"].([]any); !ok || len(ex) == 0 || len(strs(ex)) != len(ex) {
		add("F006", hline("example"), "example must be a non-empty array of strings")
	}

	lintParams(h, code, hline, add)

	if err := checkMatches(h.Matches); err != nil {
		add("F008", hline("matches"), "%v", err)
	}
	for _, p := range h.Matches {
		if _, err := regexp.Compile(p); err != nil {
			add("F008", hline("matches"), "matches pattern %q is not valid RE2: %v", p, err)
		}
		if shellRE.MatchString(p) { // the hook tests only the Python body, so a shell pattern never fires
			add("W002", hline("matches"), "matches pattern %q looks like shell, not Python", p)
		}
	}

	for i, l := range lines {
		if homeRE.MatchString(l) {
			add("F009", i+1, "absolute home path; use a param or a path relative to the repo")
		}
	}

	if !strings.Contains(code, "json.dumps") {
		add("F010", 0, "no json.dumps: stdout must be exactly one JSON object")
	}
	for _, loc := range printRE.FindAllStringIndex(code, -1) {
		if loc[0] > 0 && code[loc[0]-1] == '.' && code[loc[0]:loc[0]+5] == "print" {
			continue // a method named print, not the builtin
		}
		args := callText(code, loc[1]-1)
		if strings.Contains(args, "json.dumps") || (code[loc[0]] == 'p' && stderrRE.MatchString(args)) {
			continue
		}
		add("F010", lineAt(code, loc[0]), "stdout write that is not json.dumps; send logs to stderr")
	}

	if len(h.Returns.Keys) == 0 {
		add("F011", hline("keys"), "returns.keys must list the keys every answer contains")
	}

	if opts.FirstParty {
		for _, im := range imports {
			if im.multi {
				add("F012", im.line, "one import per line")
			}
			if top := strings.Split(im.module, ".")[0]; top == "" || !stdlib[top] {
				add("F012", im.line, "import %q is not in the Python standard library", im.module)
			}
		}
	}

	if opts.OnDefaultBranch {
		add("W001", 0, "on the default branch; promote blocks on a branch (docs/AGENT-PROMOTION.md)")
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// lintParams is F007: declared params and add_argument("--name") flags match both ways.
func lintParams(h Header, code string, hline func(string) int, add func(string, int, string, ...any)) {
	flags := map[string]int{} // name -> line
	var order []string
	for _, loc := range addArgRE.FindAllStringIndex(code, -1) {
		for _, m := range flagRE.FindAllStringSubmatch(callText(code, loc[1]-1), -1) {
			name := strings.TrimPrefix(m[1], "--")
			if _, seen := flags[name]; !seen {
				flags[name] = lineAt(code, loc[0])
				order = append(order, name)
			}
		}
	}
	var declared []string
	for _, p := range h.Params {
		if err := checkParam(p); err != nil {
			add("F007", hline(p.Name), "%v", err)
		}
		declared = append(declared, p.Name)
		if _, ok := flags[p.Name]; !ok {
			add("F007", hline(p.Name), "param %q has no add_argument(\"--%s\")", p.Name, p.Name)
		}
	}
	for _, f := range order {
		if f != "help" && !slices.Contains(declared, f) {
			add("F007", flags[f], "add_argument(\"--%s\") is not declared in [params]", f)
		}
	}
}

// effectFloor infers the lowest effect the code needs, with a reason and its 1-based line.
func effectFloor(code string, imports []pyImport) (Effect, string, int) {
	floor, why, line := EffectRead, "", 0
	raise := func(e Effect, reason string, at int) {
		if e.Rank() > floor.Rank() {
			floor, why, line = e, reason, at
		}
	}
	for _, im := range imports {
		top := strings.Split(im.module, ".")[0]
		switch {
		case top == "socket" || top == "requests" || top == "httpx" || top == "smtplib" || top == "ftplib" ||
			top == "ssl" || top == "xmlrpc" || top == "urllib3" || top == "aiohttp" || top == "websockets",
			im.module == "http.client",
			im.module == "http" && clientRE.MatchString(im.names),
			// ponytail: urllib.parse does no I/O, so it alone does not imply network.
			top == "urllib" && im.module != "urllib.parse" && !(im.module == "urllib" && strings.TrimSpace(im.names) == "parse"):
			raise(EffectNetwork, "imports "+im.module, im.line)
		case top == "subprocess" || top == "pty" || top == "multiprocessing":
			raise(EffectExec, "imports "+top, im.line)
		}
	}
	if loc := netCallRE.FindStringIndex(code); loc != nil {
		raise(EffectNetwork, "calls asyncio.open_connection", lineAt(code, loc[0]))
	}
	if loc := execCallRE.FindStringIndex(code); loc != nil {
		raise(EffectExec, "calls "+strings.TrimSuffix(code[loc[0]:loc[1]], "("), lineAt(code, loc[0]))
	}
	if loc := writeCallRE.FindStringIndex(code); loc != nil {
		raise(EffectWriteWorkspace, "calls "+strings.TrimPrefix(strings.TrimSuffix(code[loc[0]:loc[1]], "("), "."), lineAt(code, loc[0]))
	}
	for _, loc := range openRE.FindAllStringIndex(code, -1) {
		if openMode(callText(code, loc[1]-1)) {
			raise(EffectWriteWorkspace, "opens a file for writing", lineAt(code, loc[0]))
			break
		}
	}
	return floor, why, line
}

// openMode reports whether open(...) arguments ask for a writing mode.
func openMode(args string) bool {
	parts := splitArgs(args)
	mode := ""
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if v, ok := strings.CutPrefix(p, "mode"); ok && strings.HasPrefix(strings.TrimSpace(v), "=") {
			mode = strings.TrimSpace(strings.TrimSpace(v)[1:])
		} else if i == 1 && !strings.Contains(p, "=") {
			mode = p
		}
	}
	return len(mode) >= 2 && (mode[0] == '"' || mode[0] == '\'') && strings.ContainsAny(mode, "wax+")
}

// codeText is the source with the header and comment-only lines blanked, line numbers preserved.
func codeText(lines []string, start, end int) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if (i < start || i > end) && !strings.HasPrefix(strings.TrimSpace(l), "#") {
			out[i] = l
		}
	}
	return strings.Join(out, "\n")
}

func lineAt(s string, off int) int { return strings.Count(s[:off], "\n") + 1 }

// callText returns the text inside the parentheses opening at s[open], balancing nested brackets
// and skipping string literals. ponytail: no triple-quote or f-string nesting awareness.
func callText(s string, open int) string {
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
			if depth == 0 {
				return s[open+1 : i]
			}
		}
	}
	return s[open+1:]
}

// splitArgs splits call arguments at top-level commas.
func splitArgs(s string) []string {
	var parts []string
	depth, last := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, s[last:i])
			last = i + 1
		}
	}
	return append(parts, s[last:])
}

func parseImports(code string) []pyImport {
	var out []pyImport
	for i, l := range strings.Split(code, "\n") {
		if m := fromRE.FindStringSubmatch(l); m != nil {
			out = append(out, pyImport{module: m[1], names: m[2], line: i + 1})
		} else if m := importRE.FindStringSubmatch(l); m != nil {
			mods := strings.Split(m[1], ",")
			for j, mod := range mods {
				name, _, _ := strings.Cut(strings.TrimSpace(mod), " ")
				out = append(out, pyImport{module: name, line: i + 1, multi: j == 0 && len(mods) > 1})
			}
		}
	}
	return out
}
