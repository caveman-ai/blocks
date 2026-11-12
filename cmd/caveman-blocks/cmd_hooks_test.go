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

// The developer's own harness config dirs must not steer the unit tests. The e2e build runs this test
// binary as the CLI, where a scenario sets them on purpose.
func init() {
	if filepath.Base(os.Args[0]) != "caveman-blocks" {
		os.Unsetenv("CLAUDE_CONFIG_DIR")
		os.Unsetenv("CODEX_HOME")
		os.Unsetenv("PI_CODING_AGENT_DIR")
	}
}

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

// TestHooksConfigDirEnv: with CLAUDE_CONFIG_DIR set, detection, install, status and uninstall all use
// that directory and ~/.claude is never created.
func TestHooksConfigDirEnv(t *testing.T) {
	home, dir := t.TempDir(), filepath.Join(t.TempDir(), ".claude-max5")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ps, err := harnesses(nil)
	if err != nil || len(ps) != 1 || ps[0].Name != "claude-code" {
		t.Fatalf("detected %+v %v, want claude-code from CLAUDE_CONFIG_DIR", ps, err)
	}
	settings := filepath.Join(dir, "settings.json")
	if ps[0].Config != settings {
		t.Fatalf("config %s, want %s", ps[0].Config, settings)
	}
	f, _ := install.For(ps[0].ConfigFormat)
	if _, err := f.Install(expandHome(ps[0].Config), filepath.Join(home, "bin", "caveman-blocks")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(settings); err != nil {
		t.Fatal(err)
	}
	if s, err := harnessState(ps[0]); err != nil || !s.Installed || s.Config != settings {
		t.Fatalf("status %+v %v", s, err)
	}
	if out := string(cli(t, "hooks", "uninstall")); out != "claude-code: removed from "+settings+"\n" {
		t.Fatalf("uninstall: %q", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
		t.Fatal("~/.claude was created")
	}
}

// TestDropLegacyBinary: the extensionless Windows copy goes once no hook entry runs it, and never
// on a host whose binary has no .exe.
func TestDropLegacyBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	legacy := filepath.Join(home, "bin", "caveman-blocks")
	touch := func() {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacy, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exists := func() bool { _, err := os.Stat(legacy); return err == nil }

	touch()
	if dropLegacyBinary(legacy); !exists() {
		t.Fatal("removed the binary itself on a host without .exe")
	}
	f, _ := install.For("codex-hooks")
	codex := filepath.Join(home, ".codex", "hooks.json")
	if _, err := f.Install(codex, legacy); err != nil {
		t.Fatal(err)
	}
	if dropLegacyBinary(legacy + ".exe"); !exists() {
		t.Fatal("removed a copy the codex hook still runs")
	}
	if _, err := f.Install(codex, legacy+".exe"); err != nil {
		t.Fatal(err)
	}
	if dropLegacyBinary(legacy + ".exe"); exists() {
		t.Fatal("legacy copy left behind with no hook running it")
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
