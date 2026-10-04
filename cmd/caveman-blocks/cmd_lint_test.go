package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/verify"
)

// TestLintSymlinkedFixtures: a symlinked .blocks/fixtures is F015 in lint, as it fails verify and sync.
func TestLintSymlinkedFixtures(t *testing.T) {
	root := hookRepo(t)
	hookBlock(t, root, "json-view.py", "json-view", "")
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "json-view"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".blocks", "fixtures")); err != nil {
		t.Fatal(err)
	}
	has := func(fs []blockfile.Finding) bool {
		for _, f := range fs {
			if f.Code == "F015" {
				return true
			}
		}
		return false
	}
	if fs := lintFile(filepath.Join(root, ".blocks", "json-view.py"), blockfile.LintOptions{}); !has(fs) {
		t.Errorf("no F015 for a symlinked .blocks/fixtures: %+v", fs)
	}
}

func TestPrintOutcome(t *testing.T) {
	for o, want := range map[verify.Outcome]string{
		{Status: verify.Fail, Name: "json-peek", Reason: "no header"}:                   "fail json-peek: no header\n",
		{Status: verify.Pass, Name: "json-peek", Hash: "abcdefabcdef"}:                  "pass json-peek abcdefabcdef\n",
		{Status: verify.Fail, Name: "json-peek", Hash: "abcdefabcdef", Reason: "stale"}: "fail json-peek abcdefabcdef: stale\n",
	} {
		var b strings.Builder
		printOutcome(&b, o)
		if b.String() != want {
			t.Errorf("printOutcome = %q, want %q", b.String(), want)
		}
	}
}
