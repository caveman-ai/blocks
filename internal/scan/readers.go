package scan

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cmd is one shell command found in a transcript.
type Cmd struct {
	Session string
	Command string
	TS      time.Time // zero when the transcript carries no timestamp
}

// Result is what a reader got out of one transcript file.
type Result struct {
	Cmds    []Cmd
	Skipped int // lines that looked relevant but could not be parsed
}

// Reader reads one harness's transcripts. Formats are internal to each harness, so readers are
// best-effort: they count what they cannot parse instead of failing.
type Reader interface {
	Name() string
	Default() []string // glob patterns; `**` matches any number of directories
	Commands(path string) (Result, error)
}

// Readers returns the readers for every supported harness with default roots.
func Readers() []Reader { return []Reader{Claude{}, Codex{}, Cursor{}} }

func root(r, rel string) string {
	if r != "" {
		return r
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, rel)
}

// Claude reads Claude Code transcripts: <Root>/<slug>/<session>.jsonl and subagent transcripts under
// <Root>/<slug>/<session>/subagents/. Root defaults to ~/.claude/projects.
type Claude struct{ Root string }

func (Claude) Name() string { return "claude" }

func (c Claude) Default() []string {
	r := root(c.Root, ".claude/projects")
	if r == "" {
		return nil
	}
	return []string{filepath.Join(r, "*", "*.jsonl"), filepath.Join(r, "*", "*", "subagents", "**", "*.jsonl")}
}

// Commands returns Bash tool_use commands. A subagent transcript counts toward its parent session.
func (Claude) Commands(path string) (Result, error) {
	return readLines(path, session(path), toolUse("Bash"))
}

// Cursor reads Cursor agent transcripts: <Root>/<slug>/agent-transcripts/<id>/<id>.jsonl. Root
// defaults to ~/.cursor/projects. Cursor lines carry no timestamps.
type Cursor struct{ Root string }

func (Cursor) Name() string { return "cursor" }

func (c Cursor) Default() []string {
	r := root(c.Root, ".cursor/projects")
	if r == "" {
		return nil
	}
	return []string{filepath.Join(r, "*", "agent-transcripts", "*", "*.jsonl")}
}

// Commands returns Shell tool_use commands.
func (Cursor) Commands(path string) (Result, error) {
	return readLines(path, session(path), toolUse("Shell"))
}

// Codex reads Codex CLI rollouts: <Root>/YYYY/MM/DD/rollout-*.jsonl. Root defaults to
// ~/.codex/sessions.
type Codex struct{ Root string }

func (Codex) Name() string { return "codex" }

func (c Codex) Default() []string {
	r := root(c.Root, ".codex/sessions")
	if r == "" {
		return nil
	}
	return []string{filepath.Join(r, "*", "*", "*", "rollout-*.jsonl")}
}

// Commands returns shell commands from function calls: any object whose "name" contains "shell"
// or "exec" and whose "arguments" string is JSON holding "command" or "cmd", as a string or argv.
// Code-mode calls (custom_tool_call with a JavaScript "input") are not read.
func (Codex) Commands(path string) (Result, error) {
	return readLines(path, session(path), codexLine)
}

// session is the file name without .jsonl, or the parent session for a subagent transcript.
func session(path string) string {
	dir := filepath.Dir(path)
	for d := dir; d != "." && d != string(filepath.Separator); d = filepath.Dir(d) {
		if filepath.Base(d) == "subagents" {
			return filepath.Base(filepath.Dir(d))
		}
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

// parseFn parses one line. ok is false when the line was relevant but unparsable.
type parseFn func(line []byte) (cmds []string, ts time.Time, ok bool)

const maxLine = 4 << 20

// readLines streams path line by line. Lines over 4 MB are counted as skipped and discarded.
func readLines(path, sess string, parse parseFn) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	var res Result
	br := bufio.NewReaderSize(f, maxLine)
	for {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			res.Skipped++
			for errors.Is(err, bufio.ErrBufferFull) {
				_, err = br.ReadSlice('\n')
			}
			if err == io.EOF {
				return res, nil
			}
			if err != nil {
				return res, err
			}
			continue
		}
		if len(bytes.TrimSpace(line)) > 0 {
			cmds, ts, ok := parse(line)
			if !ok {
				res.Skipped++
			}
			for _, c := range cmds {
				res.Cmds = append(res.Cmds, Cmd{Session: sess, Command: c, TS: ts})
			}
		}
		if err == io.EOF {
			return res, nil
		}
		if err != nil {
			return res, err
		}
	}
}

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// toolUse parses Claude Code and Cursor lines: message.content[] items of type tool_use with the
// given tool name and a string input.command.
func toolUse(tool string) parseFn {
	needle := []byte(`"tool_use"`)
	return func(line []byte) ([]string, time.Time, bool) {
		if !bytes.Contains(line, needle) {
			return nil, time.Time{}, true
		}
		var o struct {
			Timestamp string `json:"timestamp"`
			Message   struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &o) != nil {
			return nil, time.Time{}, false
		}
		var items []struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Input struct {
				Command any `json:"command"`
			} `json:"input"`
		}
		if json.Unmarshal(o.Message.Content, &items) != nil {
			return nil, time.Time{}, true // string content mentioning tool_use, not a call
		}
		var cmds []string
		for _, it := range items {
			if s, _ := it.Input.Command.(string); it.Type == "tool_use" && it.Name == tool && s != "" {
				cmds = append(cmds, s)
			}
		}
		return cmds, parseTS(o.Timestamp), true
	}
}

func codexLine(line []byte) ([]string, time.Time, bool) {
	if !bytes.Contains(line, []byte(`"arguments"`)) {
		return nil, time.Time{}, true
	}
	var o any
	if json.Unmarshal(line, &o) != nil {
		return nil, time.Time{}, false
	}
	var ts time.Time
	if m, ok := o.(map[string]any); ok {
		s, _ := m["timestamp"].(string)
		ts = parseTS(s)
	}
	var cmds []string
	ok := true
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, x := range v {
				walk(x)
			}
		case map[string]any:
			name, _ := v["name"].(string)
			args, isStr := v["arguments"].(string)
			if isStr && (strings.Contains(name, "shell") || strings.Contains(name, "exec")) {
				var a map[string]any
				if json.Unmarshal([]byte(args), &a) != nil {
					ok = false
					return
				}
				c := a["command"]
				if c == nil {
					c = a["cmd"]
				}
				if s := commandString(c); s != "" {
					cmds = append(cmds, s)
				}
				return
			}
			for _, x := range v {
				walk(x)
			}
		}
	}
	walk(o)
	return cmds, ts, ok
}

// commandString turns a command given as a string or argv into one shell string. A
// `bash -lc <script>` style argv yields the script itself, since joining would lose its quoting.
func commandString(c any) string {
	switch c := c.(type) {
	case string:
		return c
	case []any:
		argv := make([]string, 0, len(c))
		for _, x := range c {
			s, ok := x.(string)
			if !ok {
				return ""
			}
			argv = append(argv, s)
		}
		if len(argv) == 3 && (argv[1] == "-c" || argv[1] == "-lc") {
			switch filepath.Base(argv[0]) {
			case "sh", "bash", "zsh":
				return argv[2]
			}
		}
		return strings.Join(argv, " ")
	}
	return ""
}

// expand returns files matching pattern; one `**` segment matches any depth below its prefix and
// the part after it is matched against the remaining relative path's base name.
func expand(pattern string) ([]string, error) {
	sep := string(filepath.Separator) + "**" + string(filepath.Separator)
	prefix, rest, deep := strings.Cut(pattern, sep)
	if !deep {
		return filepath.Glob(pattern)
	}
	dirs, err := filepath.Glob(prefix)
	if err != nil {
		return nil, err
	}
	if _, err := filepath.Match(rest, ""); err != nil {
		return nil, err
	}
	var out []string
	for _, d := range dirs {
		filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() {
				if ok, _ := filepath.Match(rest, e.Name()); ok {
					out = append(out, p)
				}
			}
			return nil
		})
	}
	return out, nil
}
