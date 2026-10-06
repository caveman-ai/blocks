package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/caveman-ai/blocks/internal/hook"
	"github.com/caveman-ai/blocks/internal/hook/install"
)

// cli runs caveman-blocks with args and returns its stdout.
func cli(t *testing.T, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	root := newRoot()
	root.SetArgs(args)
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.Bytes()
}

func TestHooksStatusJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	type status struct {
		Version   string          `json:"version"`
		Harnesses []harnessStatus `json:"harnesses"`
	}
	read := func() harnessStatus {
		t.Helper()
		var s status
		if err := json.Unmarshal(cli(t, "hooks", "status", "--json"), &s); err != nil {
			t.Fatal(err)
		}
		if s.Version != version || len(s.Harnesses) != 1 || s.Harnesses[0].Name != "claude-code" {
			t.Fatalf("status = %+v, want version %s and only claude-code", s, version)
		}
		return s.Harnesses[0]
	}

	if h := read(); h.Installed || h.BinaryPresent || h.Config != "~/.claude/settings.json" {
		t.Fatalf("before install: %+v", h)
	}

	all, err := hook.Profiles()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(p hook.Profile) bool { return p.Name == "claude-code" })
	f, err := install.For(all[i].ConfigFormat)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin", "caveman-blocks")
	if _, err := f.Install(expandHome(all[i].Config), bin); err != nil {
		t.Fatal(err)
	}
	if h := read(); !h.Installed || h.Binary != bin || h.BinaryPresent {
		t.Fatalf("installed, binary missing: %+v", h)
	}

	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if h := read(); !h.Installed || !h.BinaryPresent {
		t.Fatalf("installed, binary present: %+v", h)
	}
}

func TestHooksStatusJSONNoHarness(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got := string(cli(t, "hooks", "status", "--json")); got != `{"version":"`+version+`","harnesses":[]}`+"\n" {
		t.Fatalf("got %q", got)
	}
}

func TestVersionJSON(t *testing.T) {
	var v struct {
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(cli(t, "version", "--json"), &v); err != nil {
		t.Fatal(err)
	}
	if v.Version != version || !slices.Contains(v.Capabilities, "hooks_status_json") {
		t.Fatalf("version --json = %+v", v)
	}
}
