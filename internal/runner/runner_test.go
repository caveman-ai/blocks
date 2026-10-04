package runner

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
)

// fakeBlock writes body to <root>/.blocks/<name>.py and returns a hand-built Block for it.
func fakeBlock(t *testing.T, root, name, body string, eff blockfile.Effect, params ...blockfile.Param) *blockfile.Block {
	t.Helper()
	if _, err := Python(); err != nil {
		t.Skip("python not on PATH")
	}
	path := filepath.Join(root, ".blocks", name+".py")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return &blockfile.Block{Path: path, Header: blockfile.Header{Name: name, Effects: eff, Params: params}}
}

func run(t *testing.T, root, state string, b *blockfile.Block, unverified bool, args ...string) Result {
	t.Helper()
	res, err := Run(root, state, b, unverified, nil, args, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestRunJSON(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	b := fakeBlock(t, root, "hello", `import json, os; print(json.dumps({"cwd": os.getcwd()}))`, blockfile.EffectRead)

	res := run(t, root, state, b, false)
	var got map[string]any
	if err := json.Unmarshal(res.Stdout, &got); err != nil {
		t.Fatalf("stdout %q: %v", res.Stdout, err)
	}
	realRoot, _ := filepath.EvalSymlinks(root)
	if got["cwd"] != realRoot {
		t.Errorf("cwd = %v, want %s", got["cwd"], realRoot)
	}
	if _, ok := got["_unverified"]; ok || res.Exit != 0 || res.BytesFull != len(res.Stdout) || res.BytesReturned != len(res.Stdout) {
		t.Errorf("res = %+v", res)
	}

	res = run(t, root, state, b, true)
	if !strings.HasPrefix(string(res.Stdout), `{"_unverified":true,"cwd":`) || !res.Unverified {
		t.Errorf("unverified stdout = %s", res.Stdout)
	}
	if res.BytesReturned != res.BytesFull+len(`"_unverified":true,`) {
		t.Errorf("bytes %d/%d", res.BytesReturned, res.BytesFull)
	}
}

func TestMarkUnverified(t *testing.T) {
	for in, want := range map[string]string{
		"{}\n":         `{"_unverified":true}` + "\n",
		" { }":         ` {"_unverified":true }`,
		`{"a":1}`:      `{"_unverified":true,"a":1}`,
		`[1]`:          `[1]`,
		`not json`:     `not json`,
		`{"a":1}{}`:    `{"a":1}{}`,
		"":             "",
		`{"a":1} junk`: `{"a":1} junk`,
	} {
		if got := string(markUnverified([]byte(in))); got != want {
			t.Errorf("markUnverified(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunTruncates(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	b := fakeBlock(t, root, "big", `import json; print(json.dumps({"x": "é" * 2600}, ensure_ascii=False))`, blockfile.EffectRead)

	res := run(t, root, state, b, true)
	var got struct {
		Truncated  bool   `json:"_truncated"`
		Full       string `json:"_full"`
		Head       string `json:"head"`
		Unverified bool   `json:"_unverified"`
	}
	if err := json.Unmarshal(res.Stdout, &got); err != nil {
		t.Fatalf("stdout %q: %v", res.Stdout, err)
	}
	if !got.Truncated || !got.Unverified || got.Full != res.FullPath || filepath.Dir(got.Full) != filepath.Join(state, "out") {
		t.Errorf("got %+v, res %+v", got, res)
	}
	if !strings.HasPrefix(string(res.Stdout), `{"_truncated":true,"_full":`) {
		t.Errorf("key order: %.40s", res.Stdout)
	}
	if len(got.Head) > HeadBytes || len(got.Head) < HeadBytes-1 || !strings.HasPrefix(got.Head, `{"x": "é`) || strings.ContainsRune(got.Head, '�') {
		t.Errorf("head %d bytes: %.20q", len(got.Head), got.Head)
	}
	full, err := os.ReadFile(res.FullPath)
	if err != nil || len(full) != res.BytesFull || res.BytesFull <= Cap {
		t.Errorf("spill %d bytes, BytesFull %d, err %v", len(full), res.BytesFull, err)
	}
	if fi, _ := os.Stat(res.FullPath); fi.Mode().Perm() != 0o600 {
		t.Errorf("spill mode %v", fi.Mode().Perm())
	}
	if res.BytesReturned != len(res.Stdout) || res.BytesReturned > Cap {
		t.Errorf("returned %d", res.BytesReturned)
	}
}

func TestRunExitAndArgs(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	b := fakeBlock(t, root, "fail", `import json, sys; print(json.dumps({"error": "boom", "args": sys.argv[1:]})); sys.exit(3)`,
		blockfile.EffectRead, blockfile.Param{Name: "n", Type: "int"})
	res := run(t, root, state, b, false, "--n", "7", "--flag")
	if res.Exit != 3 {
		t.Errorf("exit = %d", res.Exit)
	}
	var got struct{ Args []string }
	if err := json.Unmarshal(res.Stdout, &got); err != nil || strings.Join(got.Args, " ") != "--n 7 --flag" {
		t.Errorf("args %v, %v", got.Args, err)
	}
}

func TestEffectGate(t *testing.T) {
	b := &blockfile.Block{Header: blockfile.Header{Name: "fetch", Effects: blockfile.EffectNetwork}}
	_, err := Run(t.TempDir(), "", b, false, nil, nil, nil, nil)
	var ee *EffectError
	if !errors.Is(err, ErrEffectNotAllowed) || !errors.As(err, &ee) || ee.Line != `allow_effects = ["network"]` {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), `allow_effects = ["network"]`) {
		t.Errorf("message lacks the grant line: %v", err)
	}

	b.Header.Effects = blockfile.EffectExternal
	err = CheckEffect(b, []string{"network"})
	if !errors.As(err, &ee) || ee.Line != `allow_effects = ["network", "external"]` {
		t.Errorf("external with network allowed: %v", err)
	}
	if err := CheckEffect(b, []string{"external"}); err != nil {
		t.Errorf("allowed external: %v", err)
	}
	for _, eff := range []blockfile.Effect{blockfile.EffectRead, blockfile.EffectWriteWorkspace, blockfile.EffectExec} {
		b.Header.Effects = eff
		if err := CheckEffect(b, nil); err != nil {
			t.Errorf("%s: %v", eff, err)
		}
	}
	b.Header.Effects = "nuke"
	if err := CheckEffect(b, []string{"nuke"}); err == nil {
		t.Error("unknown effect allowed")
	}
}

func TestCheckPaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	b := &blockfile.Block{Header: blockfile.Header{Name: "p", Params: []blockfile.Param{
		{Name: "path", Type: "path"}, {Name: "pattern", Type: "str"},
	}}}
	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"--path", "data/x.json"}, true},
		{[]string{"--path", filepath.Join(root, "data", "new", "file")}, true},
		{[]string{"--path=."}, true},
		{[]string{"--pattern", "/etc/passwd"}, true},
		{[]string{"--path", "../x"}, false},
		{[]string{"--path", "/etc/passwd"}, false},
		{[]string{"--path=" + outside}, false},
		{[]string{"--path", "escape/secret"}, false},
		{[]string{"--path", "data/../../x"}, false},
		{[]string{"--pat", "/etc/passwd"}, false}, // argparse prefix of --path (and --pattern)
		{[]string{"--pa", "/etc/passwd"}, false},
	} {
		err := CheckPaths(root, b, tc.args)
		if tc.ok && err != nil || !tc.ok && !errors.Is(err, ErrPathOutsideRepo) {
			t.Errorf("CheckPaths(%v) = %v", tc.args, err)
		}
	}
}

func TestRequires(t *testing.T) {
	b := &blockfile.Block{Header: blockfile.Header{Name: "r", Effects: blockfile.EffectExec,
		Requires: []string{"definitely-not-a-binary-xyz"}}}
	_, err := Run(t.TempDir(), "", b, false, nil, nil, nil, nil)
	if !errors.Is(err, ErrMissingRequires) || !strings.Contains(err.Error(), "definitely-not-a-binary-xyz") {
		t.Errorf("err = %v", err)
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		b.Header.Requires = []string{filepath.Base(sh)}
		if err := CheckRequires(b); err != nil {
			t.Errorf("sh: %v", err)
		}
	}
}

func TestPrune(t *testing.T) {
	state := t.TempDir()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	files := map[string]time.Duration{
		"out/old.log":   8 * 24 * time.Hour,
		"out/new.log":   6 * 24 * time.Hour,
		"cache/old":     2 * time.Hour,
		"cache/new":     30 * time.Minute,
		"cache/olddir/": 2 * time.Hour,
	}
	for name, age := range files {
		p := filepath.Join(state, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(state, name)); return err == nil }

	prune(state, now)
	for name, want := range map[string]bool{"out/old.log": false, "out/new.log": true, "cache/old": false, "cache/new": true, "cache/olddir": false} {
		if exists(name) != want {
			t.Errorf("%s exists = %v, want %v", name, !want, want)
		}
	}

	// Within a day of the last prune nothing is removed, even when stale.
	stale := filepath.Join(state, "cache", "stale")
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stale, now.Add(-5*time.Hour), now.Add(-5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	prune(state, now.Add(23*time.Hour))
	if !exists("cache/stale") {
		t.Error("pruned twice within a day")
	}
	prune(state, now.Add(25*time.Hour))
	if exists("cache/stale") {
		t.Error("not pruned after a day")
	}
}
