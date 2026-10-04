// Package capture extracts inline scripts from shell commands, classifies them, fingerprints them,
// scrubs secrets and stores sightings (docs/HOOK.md).
package capture

import (
	"strings"
	"time"
)

// Script is an inline script extracted from a shell command.
type Script struct {
	Lang      string // "py" in v0
	Body      string // the script text, unscrubbed
	Lines     int
	Edit      bool     // reads a file, replaces, writes the same path
	ScriptSHA string   // sha256 hex of the literal-stripped body
	FP        string   // 12 hex shape fingerprint, "" when the body has no features
	Literals  []string // string/number/path literals in order of appearance
}

// Extract finds an inline Python script in command: a heredoc to python/python3, or python -c.
// A leading `bash -lc '...'` / `sh -c '...'` wrapper is unwrapped first. ok is false when none.
func Extract(command string) (s Script, ok bool) {
	body, ok := extract(command, 0)
	if !ok || strings.TrimSpace(body) == "" {
		return Script{}, false
	}
	s = Script{Lang: "py", Body: body, Lines: strings.Count(strings.TrimRight(body, "\n"), "\n") + 1}
	s.ScriptSHA, s.FP, s.Literals, s.Edit = analyze(body)
	return s, true
}

func extract(command string, depth int) (string, bool) {
	if depth > 3 {
		return "", false
	}
	for _, c := range parseShell(command) {
		w := commandWords(c.words)
		if len(w) == 0 {
			continue
		}
		if isShell(w[0]) {
			inner, ok := shellScript(w)
			if !ok && len(w) == 1 && len(c.docs) > 0 { // bash <<EOF ... EOF
				inner, ok = c.docs[len(c.docs)-1], true
			}
			if ok {
				if b, ok := extract(inner, depth+1); ok {
					return b, true
				}
			}
			continue
		}
		if b, ok := pythonBody(w, c.docs); ok {
			return b, true
		}
	}
	return "", false
}

// pythonBody returns the inline script of one python invocation: -c '<body>' or stdin from a heredoc
// when there is no script file argument (python3 - <<EOF, python3 <<EOF, uv run - <<EOF).
func pythonBody(w, docs []string) (string, bool) {
	var args []string
	switch {
	case isPython(w[0]):
		args = w[1:]
	case w[0] == "uv" && len(w) > 2 && w[1] == "run" && w[2] == "-":
		args = w[2:]
	case w[0] == "uv" && len(w) > 2 && w[1] == "run" && isPython(w[2]):
		args = w[3:]
	default:
		return "", false
	}
args:
	for j := 0; j < len(args); j++ {
		switch a := args[j]; {
		case a == "-c":
			if j+1 < len(args) {
				return args[j+1], true
			}
			return "", false
		case strings.HasPrefix(a, "-c"):
			return a[2:], true
		case a == "-":
			break args
		case a == "-m" || a == "" || a[0] != '-':
			return "", false // module or script file
		case a == "-X" || a == "-W":
			j++
		}
	}
	if len(docs) == 0 {
		return "", false
	}
	return docs[len(docs)-1], true
}

func isPython(w string) bool {
	b := base(w)
	return strings.HasPrefix(b, "python") && strings.Trim(b[6:], "0123456789.") == ""
}

// IsBlockCall reports whether command invokes a block, and which one.
func IsBlockCall(command string) (name string, ok bool) {
	cmds, k := leadCommand(command, 0)
	if k < 0 {
		return "", false
	}
	w := commandWords(cmds[k].words)
	switch {
	case base(w[0]) == "caveman-blocks" && len(w) > 2 && w[1] == "run":
		name = w[2]
	case isPython(w[0]):
		for _, a := range w[1:] {
			if strings.HasPrefix(a, "-") {
				continue
			}
			rest, ok1 := strings.CutPrefix(strings.TrimPrefix(a, "./"), ".blocks/")
			name, ok = strings.CutSuffix(rest, ".py")
			if !ok1 || !ok {
				return "", false
			}
			break
		}
	}
	if !validBlockName(name) {
		return "", false
	}
	return name, true
}

func validBlockName(n string) bool {
	if n == "" {
		return false
	}
	for i := 0; i < len(n); i++ {
		if c := n[i]; !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// StructuredDump reports whether command dumps a structured file whole (cat/head/tail/less of
// .json/.jsonl/.ndjson/.log/.csv) and returns the extension without the dot.
func StructuredDump(command string) (ext string, ok bool) {
	cmds, k := leadCommand(command, 0)
	if k < 0 {
		return "", false
	}
	w := commandWords(cmds[k].words)
	if !dumpers[base(w[0])] || cmds[k].redirOut {
		return "", false
	}
	var files []string
	for j := 1; j < len(w); j++ {
		switch a := w[j]; {
		case (a == "-n" || a == "-c") && (w[0] == "head" || w[0] == "tail"):
			j++
		case strings.HasPrefix(a, "-"):
		default:
			files = append(files, a)
		}
	}
	if len(files) != 1 {
		return "", false
	}
	dot := strings.LastIndexByte(files[0], '.')
	if dot < 0 || !dumpExts[strings.ToLower(files[0][dot+1:])] {
		return "", false
	}
	for j := k; j+1 < len(cmds) && cmds[j].pipe; j++ {
		if nw := commandWords(cmds[j+1].words); len(nw) > 0 && limiters[base(nw[0])] {
			return "", false
		}
	}
	return strings.ToLower(files[0][dot+1:]), true
}

var (
	dumpers  = map[string]bool{"cat": true, "head": true, "tail": true, "less": true, "more": true, "bat": true}
	dumpExts = map[string]bool{"json": true, "jsonl": true, "ndjson": true, "log": true, "csv": true}
	limiters = map[string]bool{"head": true, "tail": true, "jq": true, "wc": true, "grep": true}
)

// Sighting is one line in <fp>.jsonl.
type Sighting struct {
	TS          time.Time `json:"ts"`
	Session     string    `json:"session"`
	ScriptSHA   string    `json:"script_sha"`
	Lines       int       `json:"lines"`
	Literals    []string  `json:"literals"`
	CommandHead string    `json:"command_head"` // first 200 chars, scrubbed
	Script      string    `json:"script"`       // scrubbed body
}

// Shape summarizes one fp's sightings.
type Shape struct {
	FP       string
	Count    int
	Sessions int
	Last     time.Time
	Latest   Sighting
	Literals [][]string // literal vectors across sightings, for the promote brief
}

// Store is the append-only sightings directory (<state>/candidates).
type Store struct{ Dir string }
