package registry

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/caveman-ai/blocks/internal/blockfile"
)

const stamped = `#!/usr/bin/env python3
# /// block
# name = "json-peek"
# effects = "read"
#
# [stamp]
# verified = "3f9a1c2b7d4e"
# ///
import json
`

const unstamped = `#!/usr/bin/env python3
# /// block
# name = "json-peek"
# effects = "read"
# ///
import json
`

func testRegistry() Registry {
	return Registry{
		FS: fstest.MapFS{
			"json-peek.py":                   {Data: []byte(stamped)},
			"http-json.py":                   {Data: []byte("x")},
			"README.md":                      {Data: []byte("not a block")},
			"VERSION":                        {Data: []byte("0.1.0\n")},
			"fixtures/json-peek/sample.json": {Data: []byte(`{"a":1}`)},
			"fixtures/json-peek/sub/b.jsonl": {Data: []byte("{}\n")},
		},
		Parse: func(_ string, src []byte) (*blockfile.Block, error) {
			return &blockfile.Block{Header: blockfile.Header{Effects: blockfile.EffectRead}}, nil
		},
	}
}

func TestList(t *testing.T) {
	got, err := testRegistry().List()
	if err != nil || !reflect.DeepEqual(got, []string{"http-json", "json-peek"}) {
		t.Fatalf("List = %v, %v", got, err)
	}
}

func TestRead(t *testing.T) {
	r := testRegistry()
	_, fx, err := r.Read("json-peek")
	if err != nil {
		t.Fatal(err)
	}
	if len(fx) != 2 || string(fx["sub/b.jsonl"]) != "{}\n" {
		t.Fatalf("fixtures = %v", fx)
	}
	if _, fx, err := r.Read("http-json"); err != nil || len(fx) != 0 {
		t.Fatalf("no fixtures dir: %v %v", fx, err)
	}
	for _, bad := range []string{"missing", "../etc", "A"} {
		if _, _, err := r.Read(bad); err == nil {
			t.Errorf("Read(%q) succeeded", bad)
		}
	}
}

func TestStripStamp(t *testing.T) {
	if got := string(StripStamp([]byte(stamped))); got != unstamped {
		t.Fatalf("got\n%s", got)
	}
	if got := string(StripStamp([]byte(unstamped))); got != unstamped {
		t.Fatalf("unstamped changed:\n%s", got)
	}
	crlf := strings.ReplaceAll(stamped, "\n", "\r\n")
	if got := string(StripStamp([]byte(crlf))); got != strings.ReplaceAll(unstamped, "\n", "\r\n") {
		t.Fatalf("crlf:\n%q", got)
	}
}

func TestAdd(t *testing.T) {
	root := t.TempDir()
	r := testRegistry()
	var hashed string
	hash := func(src []byte, fx map[string][]byte) string {
		hashed = string(src)
		return "abcdef123456"
	}
	res, err := r.Add(root, "json-peek", false, hash)
	if err != nil {
		t.Fatal(err)
	}
	if hashed != unstamped {
		t.Errorf("hash saw stamped source:\n%s", hashed)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".blocks", "json-peek.py"))
	if string(b) != unstamped {
		t.Errorf("written:\n%s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".blocks/fixtures/json-peek/sub/b.jsonl")); string(b) != "{}\n" {
		t.Errorf("fixture = %q", b)
	}
	if res.Effect != blockfile.EffectRead || !reflect.DeepEqual(res.Fixtures, []string{"sample.json", "sub/b.jsonl"}) {
		t.Errorf("result = %+v", res)
	}

	want := LockEntry{Source: "registry:json-peek@0.1.0", Version: "0.1.0", Hash: "abcdef123456"}
	lock, err := Lock(root)
	if err != nil || !reflect.DeepEqual(lock, map[string]LockEntry{"json-peek": want}) {
		t.Fatalf("lock = %v, %v", lock, err)
	}

	if _, err := r.Add(root, "json-peek", false, hash); err == nil {
		t.Fatal("second Add without force succeeded")
	}
	stale := filepath.Join(root, ".blocks/fixtures/json-peek/stale.txt")
	os.WriteFile(stale, []byte("x"), 0o644)
	if _, err := r.Add(root, "json-peek", true, hash); err != nil {
		t.Fatalf("force: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("force kept a stale fixture")
	}

	// A second block keeps the first lock entry; no VERSION means 0.0.0.
	delete(r.FS.(fstest.MapFS), "VERSION")
	if _, err := r.Add(root, "http-json", false, hash); err != nil {
		t.Fatal(err)
	}
	lock, _ = Lock(root)
	if len(lock) != 2 || lock["http-json"].Source != "registry:http-json@0.0.0" {
		t.Fatalf("lock = %v", lock)
	}
	text, _ := os.ReadFile(filepath.Join(root, ".blocks", "blocks.lock"))
	if strings.Index(string(text), "[http-json]") > strings.Index(string(text), "[json-peek]") {
		t.Errorf("lock not sorted:\n%s", text)
	}
}

func TestLockMissing(t *testing.T) {
	lock, err := Lock(t.TempDir())
	if err != nil || len(lock) != 0 {
		t.Fatalf("%v %v", lock, err)
	}
}

// TestAddRefusesSymlinks: a cloned repo whose .blocks/fixtures or .blocks/fixtures/<name> points
// outside (say at ~) must not get files written or deleted there, even with --force.
func TestAddRefusesSymlinks(t *testing.T) {
	hash := func([]byte, map[string][]byte) string { return "h" }
	for _, link := range []string{".blocks/fixtures", ".blocks/fixtures/json-peek", ".blocks/blocks.lock"} {
		outside := t.TempDir()
		victim := filepath.Join(outside, "keep")
		if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		target := outside
		if strings.HasSuffix(link, ".lock") {
			target = victim
		}
		root := t.TempDir()
		p := filepath.Join(root, filepath.FromSlash(link))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
		for _, force := range []bool{false, true} {
			if _, err := testRegistry().Add(root, "json-peek", force, hash); err == nil {
				t.Errorf("%s force=%v: no error", link, force)
			}
		}
		ents, _ := os.ReadDir(outside)
		if b, _ := os.ReadFile(victim); len(ents) != 1 || string(b) != "keep" {
			t.Errorf("%s: outside dir changed: %d entries, victim %q", link, len(ents), b)
		}
	}
}
