package scan

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/caveman-ai/blocks/internal/capture"
)

var update = flag.Bool("update", false, "rewrite golden files")

// extractStub stands in for capture.Extract: a python3 heredoc body, edit when it writes, and a
// fingerprint keyed on Counter or json.load.
func extractStub(cmd string) (capture.Script, bool) {
	_, rest, ok := strings.Cut(cmd, "<<'PY'\n")
	if !ok {
		return capture.Script{}, false
	}
	body := strings.TrimSuffix(rest, "\nPY")
	s := capture.Script{Lang: "py", Body: body, Lines: strings.Count(body, "\n") + 1, Edit: strings.Contains(body, ".write(")}
	switch {
	case strings.Contains(body, "Counter"):
		s.FP = "bbbbbbbbbbbb"
	case strings.Contains(body, "json.load"):
		s.FP = "aaaaaaaaaaaa"
	}
	return s, true
}

func testReaders() []Reader {
	return []Reader{
		Claude{Root: "testdata/claude"},
		Codex{Root: "testdata/codex"},
		Cursor{Root: "testdata/cursor"},
	}
}

func commands(t *testing.T, r Reader) ([]string, []string, int) {
	t.Helper()
	var cmds, sessions []string
	skipped := 0
	for _, p := range r.Default() {
		files, err := expand(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			res, err := r.Commands(f)
			if err != nil {
				t.Fatal(err)
			}
			skipped += res.Skipped
			for _, c := range res.Cmds {
				first, _, _ := strings.Cut(c.Command, "\n")
				cmds = append(cmds, first)
				sessions = append(sessions, c.Session)
			}
		}
	}
	return cmds, sessions, skipped
}

func TestReaders(t *testing.T) {
	cases := []struct {
		r        Reader
		cmds     []string
		sessions []string
		skipped  int
	}{
		{Claude{Root: "testdata/claude"},
			[]string{"python3 - <<'PY'", "ls -la", "caveman-blocks run json-peek --path runs/a.json", "python3 - <<'PY'", "python3 - <<'PY'", "python3 - <<'PY'", "python3 - <<'PY'", "python3 - <<'PY'"},
			[]string{"s1", "s1", "s1", "s1", "s1", "s2", "s2", "s1"}, 2},
		{Codex{Root: "testdata/codex"},
			[]string{"python3 - <<'PY'", "git status --short", "python3 - <<'PY'", "git diff --stat"},
			[]string{"rollout-2026-10-03T10-00-00-abc", "rollout-2026-10-03T10-00-00-abc", "rollout-2026-10-03T10-00-00-abc", "rollout-2026-10-03T10-00-00-abc"}, 1},
		{Cursor{Root: "testdata/cursor"},
			[]string{"python3 - <<'PY'", "npm test"},
			[]string{"c1", "c1"}, 1},
	}
	for _, c := range cases {
		cmds, sessions, skipped := commands(t, c.r)
		if !reflect.DeepEqual(cmds, c.cmds) || !reflect.DeepEqual(sessions, c.sessions) || skipped != c.skipped {
			t.Errorf("%s:\n cmds %q\n sessions %q\n skipped %d", c.r.Name(), cmds, sessions, skipped)
		}
	}
}

func TestCodexTimestampAndArgv(t *testing.T) {
	res, err := Codex{}.Commands("testdata/codex/2026/10/03/rollout-2026-10-03T10-00-00-abc.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 3, 10, 1, 0, 0, time.UTC); !res.Cmds[0].TS.Equal(want) {
		t.Errorf("ts = %v", res.Cmds[0].TS)
	}
	if !strings.HasPrefix(res.Cmds[0].Command, "python3 - <<'PY'\nimport json") {
		t.Errorf("bash -lc argv not unwrapped: %q", res.Cmds[0].Command)
	}
}

func TestOddInput(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "odd.jsonl")
	long := `{"message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"` + strings.Repeat("x", maxLine+10) + `"}}]}}`
	ok := `{"message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"echo hi"}}]}}`
	odd := []string{long, "", "\x00\xff \"tool_use\"", `{"message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":42}}]}}`, `{"message":null}`, `[1,2,"tool_use"]`, ok}
	os.WriteFile(p, []byte(strings.Join(odd, "\n")), 0o644) // no trailing newline
	res, err := Claude{}.Commands(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Cmds) != 1 || res.Cmds[0].Command != "echo hi" || res.Skipped != 3 {
		t.Fatalf("res = %+v", res)
	}
	for _, line := range odd {
		codexLine([]byte(line))
	}
	if commandString([]any{"a", 1}) != "" || commandString(nil) != "" {
		t.Fatal("commandString accepted a bad argv")
	}
}

func TestSessionOfSubagent(t *testing.T) {
	if got := session("/h/.claude/projects/slug/sess-1/subagents/x/agent-a.jsonl"); got != "sess-1" {
		t.Fatalf("session = %s", got)
	}
	if got := session("/h/.claude/projects/slug/sess-1.jsonl"); got != "sess-1" {
		t.Fatalf("session = %s", got)
	}
}

func blocks() []MatchInfo {
	return []MatchInfo{
		{Name: "json-peek", Matches: []*regexp.Regexp{regexp.MustCompile(`json\.loads?\(`)}},
		{Name: "jsonl-stats", Matches: []*regexp.Regexp{regexp.MustCompile(`Counter\(`), regexp.MustCompile(`json\.loads\(`)}},
	}
}

func TestScanFormatGolden(t *testing.T) {
	rep, err := Scan(testReaders(), 0, extractStub, blocks())
	if err != nil {
		t.Fatal(err)
	}
	got := Format(rep)
	golden := filepath.Join("testdata", "format.golden")
	if *update {
		os.WriteFile(golden, []byte(got), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("Format differs from %s (go test -update):\n%s", golden, got)
	}
}

func TestScanCounts(t *testing.T) {
	rep, err := Scan(testReaders(), 0, extractStub, blocks())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Commands != 14 || rep.Scripts != 9 || rep.Edits != 1 || rep.Unshaped != 1 || len(rep.Groups) != 2 {
		t.Fatalf("report = %+v", rep)
	}
	g := rep.Groups[0]
	if g.FP != "aaaaaaaaaaaa" || g.Count != 5 || g.Sessions != 4 || g.Lines != 3 ||
		!reflect.DeepEqual(g.CoveredBy, []string{"json-peek"}) {
		t.Fatalf("group = %+v", g)
	}
	// Latest by timestamp: the s2 sighting at 11:00, not the untimed Cursor one read later.
	if !strings.Contains(g.Example, "runs/a.json") {
		t.Fatalf("example = %q", g.Example)
	}

	// A window that starts 2026-10-02 drops the 2026-10-01 commands but keeps untimed ones.
	since := time.Since(time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
	rep, _ = Scan(testReaders(), since, extractStub, blocks())
	if rep.Commands != 12 {
		t.Fatalf("windowed commands = %d", rep.Commands)
	}
}

func TestFormatTopN(t *testing.T) {
	var r Report
	for range TopN + 3 {
		r.Groups = append(r.Groups, Group{FP: "f", Count: 1, Example: "x"})
	}
	if out := Format(r); strings.Count(out, "1x  ") != TopN || !strings.Contains(out, "… 3 more shapes") {
		t.Fatalf("out:\n%s", out)
	}
	if out := Format(Report{}); !strings.Contains(out, "No repeated script shapes found.") {
		t.Fatalf("empty:\n%s", out)
	}
}

// oneFile is a Reader over one fixed transcript path that returns cmds.
type oneFile struct {
	path string
	cmds []Cmd
}

func (oneFile) Name() string                      { return "fake" }
func (r oneFile) Default() []string               { return []string{r.path} }
func (r oneFile) Commands(string) (Result, error) { return Result{Cmds: r.cmds}, nil }

func TestScanScrubsExample(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	os.WriteFile(p, nil, 0o644)
	secret := "sk-live-0123456789abcdefghijklmnopqrstuv"
	r := oneFile{p, []Cmd{
		{Session: "s", Command: "python3 - <<'PY'\nimport json\nkey = \"" + secret + "\"\njson.load(open('a'))\nPY"},
		{Session: "s", Command: "caveman-blocks run json-peek --path a.json"},
	}}
	rep, err := Scan([]Reader{r}, 0, extractStub, blocks())
	if err != nil || len(rep.Groups) != 1 {
		t.Fatalf("rep = %+v, %v", rep, err)
	}
	if out := Format(rep); strings.Contains(out, secret) || strings.Contains(rep.Groups[0].Example, secret) {
		t.Errorf("secret in scan output:\n%s", out)
	}
	if rep.Scripts != 1 || !reflect.DeepEqual(rep.Groups[0].CoveredBy, []string{"json-peek"}) {
		t.Errorf("rep = %+v", rep)
	}
}
