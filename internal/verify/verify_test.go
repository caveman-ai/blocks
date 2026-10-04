package verify

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/runner"
)

// Stubs for the blockfile functions: the hash is fixed and the stamp is a trailing comment line.
const testHash = "0123456789ab"

func stubHash(*blockfile.Block, map[string][]byte) string { return testHash }

func stubStamp(b *blockfile.Block, s blockfile.Stamp) []byte {
	return []byte(string(b.Source) + "# stamp " + s.Verified + " " + s.State + "\n")
}

var stubs = Options{Hash: stubHash, Stamp: stubStamp}

func fakeBlock(t *testing.T, root, name, body string, keys ...string) *blockfile.Block {
	t.Helper()
	if _, err := runner.Python(); err != nil {
		t.Skip("python not on PATH")
	}
	path := filepath.Join(root, ".blocks", name+".py")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return &blockfile.Block{Path: path, Source: []byte(body), Header: blockfile.Header{
		Name: name, Effects: blockfile.EffectRead, Example: []string{"--path", "$FIXTURES/in.json"},
		Returns: blockfile.Returns{Keys: keys},
	}}
}

func read(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const echoPath = `import json, os, sys
p = sys.argv[2]
print(json.dumps({"abs": os.path.isabs(p), "exists": os.path.exists(p), "path": p}))
`

func TestVerifyPassWritesStamp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".blocks", "fixtures", "peek"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".blocks", "fixtures", "peek", "in.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := fakeBlock(t, root, "peek", echoPath, "abs", "exists")

	o := Verify(root, b, nil, nil, stubs)
	if o != (Outcome{Name: "peek", Status: Pass, Hash: testHash}) {
		t.Fatalf("outcome %+v", o)
	}
	if got := read(t, b.Path); got != echoPath+"# stamp "+testHash+" \n" {
		t.Errorf("file after pass:\n%s", got)
	}

	// Already stamped with the current hash: no rewrite.
	b.Source = []byte(read(t, b.Path))
	b.Header.Stamp = &blockfile.Stamp{Verified: testHash}
	before := read(t, b.Path)
	if o := Verify(root, b, nil, nil, stubs); o.Status != Pass || read(t, b.Path) != before {
		t.Errorf("re-verify rewrote: %+v", o)
	}
}

func TestVerifyFixturesExpansion(t *testing.T) {
	root := t.TempDir()
	alt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(alt, "peek"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alt, "peek", "in.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := fakeBlock(t, root, "peek", echoPath+`sys.exit(0 if os.path.exists(p) and os.path.isabs(p) else 4)`+"\n", "exists")
	opts := stubs
	opts.FixturesRoot = alt
	opts.Check = true
	b.Header.Stamp = &blockfile.Stamp{Verified: testHash}
	if o := Verify(root, b, nil, nil, opts); o.Status != Pass {
		t.Errorf("FixturesRoot override: %+v", o)
	}
	opts.FixturesRoot = ""
	if o := Verify(root, b, nil, nil, opts); o.Status != Fail || !strings.HasPrefix(o.Reason, "exit 4") {
		t.Errorf("default fixtures dir has no in.json: %+v", o)
	}
}

func TestVerifyFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, reason string
	}{
		{"exit", `import json, sys; print(json.dumps({"error": "no input"})); sys.exit(3)`, "exit 3: no input"},
		{"stderr", `import sys; print("boom", file=sys.stderr); sys.exit(1)`, "exit 1: boom"},
		{"big", `import json; print(json.dumps({"k": "x" * 3000}))`, "output over 2 KB"},
		{"text", `print("hello")`, "stdout is not exactly one JSON object"},
		{"two", `print('{"k": 1}'); print('{"k": 2}')`, "stdout is not exactly one JSON object"},
		{"array", `print('[{"k": 1}]')`, "stdout is not exactly one JSON object"},
		{"keys", `print('{"other": 1}')`, "answer lacks returns keys: k"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			b := fakeBlock(t, root, tc.name, tc.body, "k")
			b.Header.Stamp = &blockfile.Stamp{Verified: "old000000000"}
			o := Verify(root, b, nil, nil, stubs)
			if o.Status != Fail || o.Reason != tc.reason {
				t.Errorf("outcome %+v, want reason %q", o, tc.reason)
			}
			if got := read(t, b.Path); got != tc.body+"# stamp old000000000 quarantined\n" {
				t.Errorf("file after fail:\n%s", got)
			}
		})
	}
}

func TestVerifyCheckNeverWrites(t *testing.T) {
	root := t.TempDir()
	const ok = `print('{"k": 1}')`
	opts := stubs
	opts.Check = true
	for _, tc := range []struct {
		name   string
		body   string
		stamp  *blockfile.Stamp
		status string
		reason string
	}{
		{"pass", ok, &blockfile.Stamp{Verified: testHash}, Pass, ""},
		{"missing", ok, nil, Fail, "stamp stale"},
		{"stale", ok, &blockfile.Stamp{Verified: "fff000000000"}, Fail, "stamp stale"},
		{"quarantined", ok, &blockfile.Stamp{Verified: testHash, State: "quarantined"}, Fail, "committed quarantined block"},
		{"failing", `print("x")`, &blockfile.Stamp{Verified: testHash}, Fail, "stdout is not"},
	} {
		b := fakeBlock(t, root, tc.name, tc.body, "k")
		b.Header.Stamp = tc.stamp
		o := Verify(root, b, nil, nil, opts)
		if o.Status != tc.status || !strings.HasPrefix(o.Reason, tc.reason) {
			t.Errorf("%s: %+v", tc.name, o)
		}
		if read(t, b.Path) != tc.body {
			t.Errorf("%s: check mode wrote the file", tc.name)
		}
	}
}

func TestVerifySkips(t *testing.T) {
	root := t.TempDir()
	b := fakeBlock(t, root, "fetch", `print('{"k": 1}')`, "k")
	b.Header.Effects = blockfile.EffectNetwork
	o := Verify(root, b, nil, nil, stubs)
	if o.Status != Skip || !strings.Contains(o.Reason, `allow_effects = ["network"]`) {
		t.Errorf("effect: %+v", o)
	}
	if o := Verify(root, b, nil, []string{"network"}, stubs); o.Status != Pass {
		t.Errorf("allowed network: %+v", o)
	}

	b = fakeBlock(t, root, "tool", `print('{"k": 1}')`, "k")
	b.Header.Requires = []string{"definitely-not-a-binary-xyz"}
	opts := stubs
	opts.Check = true
	b.Header.Stamp = &blockfile.Stamp{Verified: testHash} // a current stamp: the skip stands
	if o := Verify(root, b, nil, nil, opts); o.Status != Skip || !strings.Contains(o.Reason, "definitely-not-a-binary-xyz") {
		t.Errorf("requires: %+v", o)
	}
	if read(t, b.Path) != `print('{"k": 1}')` {
		t.Error("skip wrote the file")
	}
}

// TestVerifyCheckJudgesStampBeforeSkip: in CI a pull request cannot land a hand-stamped or
// quarantined block whose effect or tool is unavailable as a passing skip (docs/CI.md).
func TestVerifyCheckJudgesStampBeforeSkip(t *testing.T) {
	root := t.TempDir()
	b := fakeBlock(t, root, "fetch", `print('{"k": 1}')`, "k")
	b.Header.Effects = blockfile.EffectNetwork
	opts := stubs
	opts.Check = true
	for _, st := range []*blockfile.Stamp{nil, {Verified: "deadbeef0000"}, {Verified: testHash, State: "quarantined"}} {
		b.Header.Stamp = st
		if o := Verify(root, b, nil, nil, opts); o.Status != Fail {
			t.Errorf("stamp %+v: %+v", st, o)
		}
	}
	b.Header.Stamp = &blockfile.Stamp{Verified: testHash}
	if o := Verify(root, b, nil, nil, opts); o.Status != Skip {
		t.Errorf("current stamp, effect not allowed: %+v", o)
	}
	// Outside Check mode a skip still comes first.
	b.Header.Stamp = nil
	if o := Verify(root, b, nil, nil, stubs); o.Status != Skip {
		t.Errorf("non-check: %+v", o)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func put(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSelect(t *testing.T) {
	root := gitRepo(t)
	for _, n := range []string{"a", "b", "c", "d"} {
		put(t, root, ".blocks/"+n+".py", n)
	}
	put(t, root, ".blocks/fixtures/b/x.json", "{}")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base")
	git(t, root, "checkout", "-q", "-b", "feat")

	p := func(names ...string) []string {
		var out []string
		for _, n := range names {
			out = append(out, filepath.Join(root, ".blocks", n+".py"))
		}
		return out
	}

	if got, err := Select(root, "changed", "", "main"); err != nil || len(got) != 0 {
		t.Errorf("nothing changed: %v, %v", got, err)
	}

	put(t, root, ".blocks/a.py", "a2")               // committed edit
	put(t, root, ".blocks/fixtures/b/y.json", "[]")  // untracked fixture
	put(t, root, ".blocks/e.py", "e")                // untracked block
	put(t, root, ".blocks/fixtures/cc/z.json", "{}") // fixture of a block that is not c
	git(t, root, "add", ".blocks/a.py")
	git(t, root, "commit", "-q", "-m", "edit a")
	put(t, root, ".blocks/d.py", "d2") // uncommitted edit

	got, err := Select(root, "changed", "", "main")
	if want := p("a", "b", "d", "e"); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("changed = %v, %v; want %v", got, err, want)
	}

	// No merge base: everything.
	got, err = Select(root, "changed", "", "no-such-branch")
	if want := p("a", "b", "c", "d", "e"); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("fallback = %v, %v", got, err)
	}
	if got, err := Select(root, "all", "", ""); err != nil || len(got) != 5 {
		t.Errorf("all = %v, %v", got, err)
	}
	if got, err := Select(root, "name", "c", ""); err != nil || !reflect.DeepEqual(got, p("c")) {
		t.Errorf("name = %v, %v", got, err)
	}
	for _, bad := range []string{"zz", "../x", ""} {
		if _, err := Select(root, "name", bad, ""); err == nil {
			t.Errorf("name %q: no error", bad)
		}
	}
	if _, err := Select(root, "bogus", "", ""); err == nil {
		t.Error("unknown mode: no error")
	}
}
