package hook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/internal/capture"
)

// goldenCase is one row of testdata/decisions/*.json.
type goldenCase struct {
	Name string `json:"name"`
	Cfg  struct {
		RepoRoot     string   `json:"repo_root"`
		HintDisabled bool     `json:"hint_disabled"`
		CoveredFPs   []string `json:"covered_fps"`
		Blocks       []struct {
			Name    string   `json:"name"`
			Effects string   `json:"effects"`
			Matches []string `json:"matches"`
			Hint    string   `json:"hint"`
		} `json:"blocks"`
	} `json:"cfg"`
	Input struct {
		Phase   string `json:"phase"`
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
		Session string `json:"session"`
		CallID  string `json:"call_id"`
	} `json:"input"`
	Stub struct {
		BlockCall string `json:"block_call"`
		Extract   *struct {
			Body      string   `json:"body"`
			Lines     int      `json:"lines"`
			Edit      bool     `json:"edit"`
			FP        string   `json:"fp"`
			ScriptSHA string   `json:"script_sha"`
			Literals  []string `json:"literals"`
		} `json:"extract"`
		Dump      string `json:"dump"`
		AppendErr bool   `json:"append_err"`
		Shapes    []struct {
			FP       string `json:"fp"`
			Sessions int    `json:"sessions"`
		} `json:"shapes"`
	} `json:"stub"`
	Cache struct {
		Cmd           *Decision `json:"cmd"`
		CmdRecent     bool      `json:"cmd_recent"`
		Call          *Decision `json:"call"`
		PromoteHinted []string  `json:"promote_hinted"` // fps already hinted today
		AddHinted     []string  `json:"add_hinted"`
	} `json:"cache"`
	Expect struct {
		Hint     string  `json:"hint"`
		Events   []Event `json:"events"`
		Sighting *struct {
			FP          string   `json:"fp"`
			ScriptSHA   string   `json:"script_sha"`
			Lines       int      `json:"lines"`
			Script      string   `json:"script"`
			CommandHead string   `json:"command_head"`
			Literals    []string `json:"literals"`
		} `json:"sighting"`
	} `json:"expect"`
}

// memCache is an in-memory Cache; recent holds keys written within the dedupe window.
type memCache struct {
	entries map[string]Decision
	recent  map[string]bool
	flags   map[string]bool
}

func newMemCache() *memCache {
	return &memCache{entries: map[string]Decision{}, recent: map[string]bool{}, flags: map[string]bool{}}
}

func (m *memCache) Get(k string) (Decision, bool) { d, ok := m.entries[k]; return d, ok }
func (m *memCache) Put(k string, d Decision)      { m.entries[k] = d; m.recent[k] = true }
func (m *memCache) SeenRecently(k string, _ time.Duration) bool {
	return m.recent[k]
}
func (m *memCache) PromoteHinted(fp, day string) bool { return m.flags["promote:"+fp+":"+day] }
func (m *memCache) MarkPromoteHinted(fp, day string)  { m.flags["promote:"+fp+":"+day] = true }
func (m *memCache) AddHinted(s, b string) bool        { return m.flags["add:"+s+":"+b] }
func (m *memCache) MarkAddHinted(s, b string)         { m.flags["add:"+s+":"+b] = true }

var testNow = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func TestGoldenDecisions(t *testing.T) {
	files, err := filepath.Glob("testdata/decisions/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden files: %v", err)
	}
	total := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var cases []goldenCase
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cases); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, c := range cases {
			total++
			t.Run(filepath.Base(f)+"/"+c.Name, func(t *testing.T) { runGolden(t, c) })
		}
	}
	if total < 20 {
		t.Fatalf("golden tables hold %d cases, want at least 20", total)
	}
}

func runGolden(t *testing.T, c goldenCase) {
	cfg := Config{RepoRoot: c.Cfg.RepoRoot, StateDir: "/state", HintEnabled: !c.Cfg.HintDisabled,
		CoveredFPs: map[string]bool{}, Now: func() time.Time { return testNow }}
	for _, fp := range c.Cfg.CoveredFPs {
		cfg.CoveredFPs[fp] = true
	}
	for _, b := range c.Cfg.Blocks {
		bi := BlockInfo{Name: b.Name, Effects: b.Effects, Hint: b.Hint}
		for _, m := range b.Matches {
			bi.Matches = append(bi.Matches, regexp.MustCompile(m))
		}
		cfg.Blocks = append(cfg.Blocks, bi)
	}
	in := Input{Phase: c.Input.Phase, Command: c.Input.Command, Cwd: c.Input.Cwd, Session: c.Input.Session, CallID: c.Input.CallID}
	if in.Phase == "" {
		in.Phase = PhasePre
	}

	cache := newMemCache()
	if c.Cache.Cmd != nil {
		cache.entries[cmdKey(in)] = *c.Cache.Cmd
		cache.recent[cmdKey(in)] = c.Cache.CmdRecent
	}
	if c.Cache.Call != nil {
		cache.entries[callKey(in.CallID)] = *c.Cache.Call
	}
	for _, fp := range c.Cache.PromoteHinted {
		cache.MarkPromoteHinted(fp, testNow.Format(time.DateOnly))
	}
	for _, b := range c.Cache.AddHinted {
		cache.MarkAddHinted(in.Session, b)
	}

	var appended []capture.Sighting
	var appendedFP string
	deps := Deps{
		Extract: func(string) (capture.Script, bool) {
			x := c.Stub.Extract
			if x == nil {
				return capture.Script{}, false
			}
			return capture.Script{Lang: "py", Body: x.Body, Lines: x.Lines, Edit: x.Edit, FP: x.FP,
				ScriptSHA: x.ScriptSHA, Literals: x.Literals}, true
		},
		IsBlockCall:    func(string) (string, bool) { return c.Stub.BlockCall, c.Stub.BlockCall != "" },
		StructuredDump: func(string) (string, bool) { return c.Stub.Dump, c.Stub.Dump != "" },
		Scrub:          func(s string) string { return strings.ReplaceAll(s, "sk-SECRET", "«scrubbed»") },
		Append: func(fp string, sg capture.Sighting) error {
			if c.Stub.AppendErr {
				return errors.New("disk full")
			}
			appendedFP = fp
			appended = append(appended, sg)
			return nil
		},
		Shapes: func(since time.Duration) ([]capture.Shape, error) {
			if since != 14*24*time.Hour {
				t.Errorf("Shapes window %v, want 14 days", since)
			}
			var out []capture.Shape
			for _, s := range c.Stub.Shapes {
				out = append(out, capture.Shape{FP: s.FP, Count: s.Sessions, Sessions: s.Sessions})
			}
			return out, nil
		},
		Cache: cache,
	}

	d := Decide(cfg, in, deps)

	if d.Hint != c.Expect.Hint {
		t.Errorf("hint\n got %q\nwant %q", d.Hint, c.Expect.Hint)
	}
	var got []Event
	for _, e := range d.Events {
		if !e.TS.Equal(testNow) || e.Session != in.Session {
			t.Errorf("event %+v: ts/session not stamped", e)
		}
		got = append(got, Event{Kind: e.Kind, Block: e.Block, FP: e.FP, Lines: e.Lines})
	}
	if !reflect.DeepEqual(got, c.Expect.Events) {
		t.Errorf("events\n got %+v\nwant %+v", got, c.Expect.Events)
	}

	want := c.Expect.Sighting
	switch {
	case want == nil && len(appended) > 0:
		t.Errorf("unexpected sighting %+v", appended[0])
	case want != nil && len(appended) != 1:
		t.Errorf("want one sighting, got %d", len(appended))
	case want != nil:
		sg := appended[0]
		if appendedFP != want.FP || sg.ScriptSHA != want.ScriptSHA || sg.Lines != want.Lines || sg.Script != want.Script ||
			sg.CommandHead != want.CommandHead || !reflect.DeepEqual(sg.Literals, want.Literals) ||
			sg.Session != in.Session || !sg.TS.Equal(testNow) {
			t.Errorf("sighting\n got %+v\nwant %+v", sg, *want)
		}
	}

	// Every pre call inside a repo leaves its decision for rule 2 and the post-run replay.
	if in.Phase == PhasePre && cfg.RepoRoot != "" {
		if cd, ok := cache.Get(cmdKey(in)); !ok || cd.Hint != d.Hint {
			t.Errorf("cmd key not cached: %+v %v", cd, ok)
		}
		if in.CallID != "" {
			if cd, ok := cache.Get(callKey(in.CallID)); !ok || cd.Hint != d.Hint {
				t.Errorf("call key not cached: %+v %v", cd, ok)
			}
		}
	}
}

func TestFileCache(t *testing.T) {
	dir := t.TempDir()
	now := testNow
	c := &FileCache{Dir: dir, FlagDir: t.TempDir(), Now: func() time.Time { return now }}

	if _, ok := c.Get("k"); ok {
		t.Fatal("empty cache hit")
	}
	c.Put("k", Decision{Hint: "h", Events: []Event{{Kind: KindHint}}})
	d, ok := c.Get("k")
	if !ok || d.Hint != "h" || len(d.Events) != 1 {
		t.Fatalf("get after put: %+v %v", d, ok)
	}
	if !c.SeenRecently("k", 2*time.Second) {
		t.Fatal("not seen recently right after put")
	}
	if fi, _ := os.Stat(c.path("k")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", fi.Mode().Perm())
	}

	now = now.Add(3 * time.Second)
	if c.SeenRecently("k", 2*time.Second) {
		t.Fatal("seen after the window")
	}
	if _, ok := c.Get("k"); !ok {
		t.Fatal("replay lost inside the TTL")
	}

	if c.PromoteHinted("aaa", "2026-10-03") || c.AddHinted("s", "json-peek") {
		t.Fatal("flags set on a fresh cache")
	}
	c.MarkPromoteHinted("aaa", "2026-10-03")
	c.MarkAddHinted("s", "json-peek")
	if !c.PromoteHinted("aaa", "2026-10-03") || c.PromoteHinted("bbb", "2026-10-03") || c.PromoteHinted("aaa", "2026-10-04") ||
		!c.AddHinted("s", "json-peek") || c.AddHinted("s", "first-error") {
		t.Fatal("flags not scoped to shape and day, or session and block")
	}

	now = now.Add(2 * time.Hour)
	if _, ok := c.Get("k"); ok {
		t.Fatal("entry outlived the TTL")
	}
	// The promote flag outlives the cache TTL: it is per day, not per hour.
	if !c.PromoteHinted("aaa", "2026-10-03") {
		t.Fatal("promote flag expired with the cache TTL")
	}
	c.MarkPromoteHinted("aaa", "2026-10-04")
	if c.PromoteHinted("aaa", "2026-10-03") || !c.PromoteHinted("aaa", "2026-10-04") {
		t.Fatal("a new day does not replace the flag")
	}
}

func TestFileCacheRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	c := &FileCache{Dir: dir}
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte(`{"hint":"planted"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, c.path("k")); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("k"); ok {
		t.Fatal("read through a symlink")
	}
	c.Put("k", Decision{Hint: "mine"})
	if b, _ := os.ReadFile(target); string(b) != `{"hint":"planted"}` {
		t.Fatal("wrote through a symlink")
	}
}

// stubDeps returns Deps with no script, no block call and no dump; shapes and extract are set by tests.
func stubDeps(c Cache) Deps {
	return Deps{
		Extract:        func(string) (capture.Script, bool) { return capture.Script{}, false },
		IsBlockCall:    func(string) (string, bool) { return "", false },
		StructuredDump: func(string) (string, bool) { return "", false },
		Scrub:          func(s string) string { return s },
		Append:         func(string, capture.Sighting) error { return nil },
		Shapes:         func(time.Duration) ([]capture.Shape, error) { return nil, nil },
		Cache:          c,
	}
}

func TestPromoteHintOncePerShapePerDay(t *testing.T) {
	now := testNow
	cfg := Config{RepoRoot: "/repo", HintEnabled: true, Now: func() time.Time { return now }}
	deps := stubDeps(newMemCache())
	deps.Shapes = func(time.Duration) ([]capture.Shape, error) {
		return []capture.Shape{{FP: "aaaaaaaaaaaa", Sessions: 2}}, nil
	}
	hint := func(session string) string {
		return Decide(cfg, Input{Phase: PhasePre, Command: "ls", Session: session}, deps).Hint
	}
	if hint("s1") != PromoteHint {
		t.Fatal("no promote hint for a due shape")
	}
	if hint("s2") != "" {
		t.Fatal("a new session the same day hinted the same shape again")
	}
	now = now.Add(24 * time.Hour)
	if hint("s3") != PromoteHint {
		t.Fatal("no promote hint on the next day")
	}
}

func TestSightingScriptCapped(t *testing.T) {
	body := strings.Repeat("print('é')\n", 4000) // 48 KB, multi-byte runes
	var got capture.Sighting
	deps := stubDeps(newMemCache())
	deps.Extract = func(string) (capture.Script, bool) {
		return capture.Script{Body: body, Lines: 4000, FP: "aaaaaaaaaaaa"}, true
	}
	deps.Append = func(_ string, sg capture.Sighting) error { got = sg; return nil }
	Decide(Config{RepoRoot: "/repo", Now: func() time.Time { return testNow }}, Input{Phase: PhasePre, Command: "python3 -"}, deps)
	if len(got.Script) == 0 || len(got.Script) > 16<<10 || !strings.HasPrefix(body, got.Script) {
		t.Fatalf("script %d bytes, want a valid prefix of at most 16 KiB", len(got.Script))
	}
}

// TestSightingHasNoSecrets runs rule 7 with the real capture package: the reviewer's secrets must
// not reach any field of the stored sighting.
func TestSightingHasNoSecrets(t *testing.T) {
	body := "import json, urllib.request\nhexkey = \"abcd1234efgh5678\"\nh = {\"X-Api-Key\": \"zzzz9999yyyy8888\"}\n" +
		"print(\"abcd1234efgh5678\")\n" + strings.Repeat("print(json.dumps(h))\n", 8)
	var got capture.Sighting
	deps := stubDeps(newMemCache())
	deps.Extract, deps.Scrub = capture.Extract, capture.Scrub
	deps.Append = func(_ string, sg capture.Sighting) error { got = sg; return nil }
	Decide(Config{RepoRoot: "/repo", Now: func() time.Time { return testNow }},
		Input{Phase: PhasePre, Command: "python3 - <<'EOF'\n" + body + "EOF", Session: "s1"}, deps)
	raw, _ := json.Marshal(got)
	if got.Lines < 10 || len(got.Literals) == 0 {
		t.Fatalf("no sighting captured: %s", raw)
	}
	for _, secret := range []string{"abcd1234efgh5678", "zzzz9999yyyy8888"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("secret %s stored: %s", secret, raw)
		}
	}
}
