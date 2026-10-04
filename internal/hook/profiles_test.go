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
		if p.Dialect == "" || p.Config == "" || p.ConfigFormat == "" || p.PreEvent.Name == "" ||
			p.Timeout.Value == 0 || p.CommandField == "" || len(p.Capabilities) == 0 || len(p.InstructionFiles) == 0 {
			t.Errorf("%s: incomplete profile %+v", p.Name, p)
		}
		if strings.Contains(strings.Join(p.Capabilities, ","), "post-hint") && p.PostEvent.Name == "" {
			t.Errorf("%s: post-hint without a post event", p.Name)
		}
		if p.Phase == 1 {
			if _, err := os.Stat(filepath.Join("dialect", p.Dialect)); err != nil {
				t.Errorf("%s: phase-1 dialect %q has no package", p.Name, p.Dialect)
			}
		}
	}
	if got := strings.Join(names, ","); got != "claude-code,codex,cursor,copilot,gemini" {
		t.Fatalf("profiles %s", got)
	}
	if ps[4].Timeout.Unit != "ms" || ps[4].Timeout.Value != 5000 {
		t.Errorf("gemini timeout %+v, want 5000 ms", ps[4].Timeout)
	}
}
