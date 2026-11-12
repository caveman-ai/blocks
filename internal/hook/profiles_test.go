package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfiles(t *testing.T) {
	ps, err := Profiles()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
		if p.Dialect == "" || p.Config == "" || p.ConfigFormat == "" || p.PreEvent.Name == "" || p.Timeout.Value == 0 ||
			(p.Timeout.Unit != "s" && p.Timeout.Unit != "ms") {
			t.Errorf("%s: incomplete profile %+v", p.Name, p)
		}
		if p.Phase == 1 {
			if _, err := os.Stat(filepath.Join("dialect", p.Dialect)); err != nil {
				t.Errorf("%s: phase-1 dialect %q has no package", p.Name, p.Dialect)
			}
		}
	}
	if got := strings.Join(names, ","); got != "claude-code,codex,cursor,opencode,pi,copilot,gemini" {
		t.Fatalf("profiles %s", got)
	}
	if ps[6].Timeout.Unit != "ms" || ps[6].Timeout.Value != 5000 {
		t.Errorf("gemini timeout %+v, want 5000 ms", ps[6].Timeout)
	}
}

// TestProfileConfigEnv: Claude Code, Codex and pi move their config with CLAUDE_CONFIG_DIR, CODEX_HOME and PI_CODING_AGENT_DIR;
// unset or empty, the documented default stands. The other harnesses have no such variable.
func TestProfileConfigEnv(t *testing.T) {
	paths := func() map[string][2]string {
		ps, err := Profiles()
		if err != nil {
			t.Fatal(err)
		}
		m := map[string][2]string{}
		for _, p := range ps {
			m[p.Name] = [2]string{p.Config, p.Detect[0]}
		}
		return m
	}
	for _, unset := range []bool{true, false} { // unset, then set but empty
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		t.Setenv("CODEX_HOME", "")
		t.Setenv("PI_CODING_AGENT_DIR", "")
		if unset {
			os.Unsetenv("CLAUDE_CONFIG_DIR")
			os.Unsetenv("CODEX_HOME")
			os.Unsetenv("PI_CODING_AGENT_DIR")
		}
		got := paths()
		for name, want := range map[string][2]string{
			"claude-code": {"~/.claude/settings.json", "~/.claude"},
			"codex":       {"~/.codex/hooks.json", "~/.codex"},
			"cursor":      {"~/.cursor/hooks.json", "~/.cursor"},
			"opencode":    {"~/.config/opencode/plugins/caveman-blocks.js", "~/.config/opencode"},
			"pi":          {"~/.pi/agent/extensions/caveman-blocks.js", "~/.pi/agent"},
		} {
			if got[name] != want {
				t.Errorf("unset=%v %s: %v, want %v", unset, name, got[name], want)
			}
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "/home/u/.claude-max5")
	t.Setenv("CODEX_HOME", "/srv/codex")
	t.Setenv("PI_CODING_AGENT_DIR", "/srv/pi")
	got := paths()
	for name, want := range map[string][2]string{
		"claude-code": {"/home/u/.claude-max5/settings.json", "/home/u/.claude-max5"},
		"codex":       {"/srv/codex/hooks.json", "/srv/codex"},
		"cursor":      {"~/.cursor/hooks.json", "~/.cursor"},
		"pi":          {"/srv/pi/extensions/caveman-blocks.js", "/srv/pi"},
	} {
		if got[name] != want {
			t.Errorf("set: %s: %v, want %v", name, got[name], want)
		}
	}
	if got := resolveEnv("~/${X:-y}/z"); got != "~/${X:-y}/z" {
		t.Errorf("only a leading ${VAR:-default} resolves; got %s", got)
	}
}
