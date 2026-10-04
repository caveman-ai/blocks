//go:build !windows

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JuliusBrussee/caveman-blocks/blocks"
	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/export"
	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
)

// symlinkRepo makes <tmp>/repo/.blocks and <tmp>/outside/file ("keep") and returns both paths.
func symlinkRepo(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root, outside = filepath.Join(base, "repo"), filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(root, ".blocks"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "file"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

// snapshot is every file under dir with its content, so a test can assert nothing changed.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(p)
			b.WriteString(p + "=" + string(data) + "\n")
		}
		return err
	})
	return b.String()
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestSyncRefusesSymlinkedAgentsMD(t *testing.T) {
	root, outside := symlinkRepo(t)
	symlink(t, "../outside/file", filepath.Join(root, "AGENTS.md"))
	before := snapshot(t, outside)
	for _, check := range []bool{true, false} {
		if _, err := syncRepo(root, check, false); err == nil {
			t.Errorf("check=%v: no error", check)
		}
	}
	if got := snapshot(t, outside); got != before {
		t.Errorf("outside changed:\n%s", got)
	}
}

func TestSyncSkipsSymlinkedClaudeMD(t *testing.T) {
	root, outside := symlinkRepo(t)
	symlink(t, "../outside/file", filepath.Join(root, "CLAUDE.md"))
	before := snapshot(t, outside)
	files, err := syncRepo(root, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(files, " "), "CLAUDE.md") {
		t.Errorf("wrote CLAUDE.md: %v", files)
	}
	if got := snapshot(t, outside); got != before {
		t.Errorf("outside changed:\n%s", got)
	}
}

func TestSyncSkipsSymlinkedExportDir(t *testing.T) {
	root, outside := symlinkRepo(t)
	// A generated-looking SKILL.md outside that sync would "clean up" if it followed the link.
	stale := filepath.Join(outside, "skills", "old", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte(export.Marker), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink(t, "../outside", filepath.Join(root, ".claude"))
	before := snapshot(t, outside)
	for _, force := range []bool{false, true} {
		files, err := syncRepo(root, false, force)
		if err != nil {
			t.Fatalf("force=%v: %v", force, err)
		}
		if strings.Contains(strings.Join(files, " "), ".claude/") {
			t.Errorf("force=%v touched .claude: %v", force, files)
		}
	}
	if got := snapshot(t, outside); got != before {
		t.Errorf("outside changed:\n%s", got)
	}
}

func TestInitRefusesSymlinkedBlocks(t *testing.T) {
	base := t.TempDir()
	root, outside := filepath.Join(base, "repo"), filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	symlink(t, "../outside", filepath.Join(root, ".blocks"))
	t.Chdir(root)
	c := initCmd()
	c.SetArgs(nil)
	c.SilenceErrors = true
	c.SetOut(new(strings.Builder))
	if err := c.Execute(); err == nil {
		t.Error("no error")
	}
	if got := snapshot(t, outside); got != "" {
		t.Errorf("outside changed:\n%s", got)
	}
}

func TestOversizedFixturesNotIndexed(t *testing.T) {
	root, _ := symlinkRepo(t)
	src, err := fs.ReadFile(blocks.FS, "json-peek.py")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".blocks", "json-peek.py"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	fx := filepath.Join(root, ".blocks", "fixtures", "json-peek", "big.json")
	if err := os.MkdirAll(filepath.Dir(fx), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fx, []byte(strings.Repeat(" ", repo.FixtureMax+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	all, errs := loadBlocks(root)
	if len(errs) != 0 || len(all) != 1 || all[0].indexed {
		t.Errorf("loadBlocks = %+v, %v", all, errs)
	}
	if fd := lintFile(filepath.Join(root, ".blocks", "json-peek.py"), blockfile.LintOptions{}); !slices.ContainsFunc(fd, func(f blockfile.Finding) bool { return f.Code == "F015" }) {
		t.Errorf("lint findings %+v lack F015", fd)
	}
}

func TestNewerSemver(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.2.0", "0.1.9", true}, {"0.1.0", "0.1.0", false}, {"0.1.0", "0.2.0", false},
		{"v1.0.0", "0.9.9", true}, {"1.0.0", "1.0.0-rc.1", true}, {"1.0.0-rc.1", "1.0.0", false},
		{"1.0.0-rc.10", "1.0.0-rc.9", true}, {"1.0.0-rc.2", "1.0.0-rc.10", false},
		{"1.0.0-rc.1", "1.0.0-beta", true}, {"1.0.0-rc.1.1", "1.0.0-rc.1", true}, {"1.0.0+b2", "1.0.0+b1", false},
	} {
		a, okA := semver(c.a)
		b, okB := semver(c.b)
		if !okA || !okB || newer(a, b) != c.want {
			t.Errorf("newer(%s, %s) = %v, want %v", c.a, c.b, newer(a, b), c.want)
		}
	}
	for _, v := range []string{"", "15e4335", "1.2", "1.2.x"} {
		if _, ok := semver(v); ok {
			t.Errorf("semver(%q) ok", v)
		}
	}
}

func TestRefreshBinaryOnlyUpgrades(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })
	for _, c := range []struct {
		mine, installed string
		want            bool
	}{
		{"0.1.0", "0.2.0", false}, {"0.2.0", "0.2.0", false}, {"0.2.0", "0.1.0", true},
		{"0.2.0", "not-a-version", true}, {"0.0.0-dev", "garbage", false}, {"15e4335", "0.1.0", false},
	} {
		t.Setenv("HOME", t.TempDir())
		installed := filepath.Join(repo.BinaryHome(), "caveman-blocks")
		if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(installed, []byte("#!/bin/sh\necho caveman-blocks "+c.installed+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		version = c.mine
		if _, changed := refreshBinary(); changed != c.want {
			t.Errorf("mine %s, installed %s: changed = %v", c.mine, c.installed, changed)
		}
	}
}
