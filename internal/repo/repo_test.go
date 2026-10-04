package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// gitRepo creates a git repository in a temp dir with hermetic git config.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, msg string) {
	t.Helper()
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false",
		"commit", "-q", "--allow-empty", "-m", msg)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".blocks"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{root, deep} {
		got, ok := FindRoot(cwd)
		if !ok || got != root {
			t.Errorf("FindRoot(%s) = %q, %v; want %q", cwd, got, ok, root)
		}
	}

	none := t.TempDir()
	if _, ok := FindRoot(none); ok {
		t.Error("FindRoot without .blocks: ok")
	}

	linked := t.TempDir()
	if err := os.Symlink(filepath.Join(root, ".blocks"), filepath.Join(linked, ".blocks")); err != nil {
		t.Fatal(err)
	}
	if _, ok := FindRoot(linked); ok {
		t.Error("FindRoot through a symlinked .blocks: ok")
	}
}

func TestLoadConfig(t *testing.T) {
	root := t.TempDir()
	c, err := LoadConfig(root)
	if err != nil || !reflect.DeepEqual(c, DefaultConfig()) {
		t.Fatalf("missing file: %+v, %v", c, err)
	}

	write(t, filepath.Join(root, ".blocks", "config.toml"), "allow_effects = [\"network\"]\nhint = false\n")
	c, err = LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultConfig()
	want.AllowEffects = []string{"network"}
	want.Hint = false
	if !reflect.DeepEqual(c, want) {
		t.Errorf("got %+v want %+v", c, want)
	}

	write(t, filepath.Join(root, ".blocks", "config.toml"), "allow_efects = [\"network\"]\n")
	if _, err := LoadConfig(root); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("unknown key: err = %v", err)
	}
}

func TestLoadConfigAt(t *testing.T) {
	root := gitRepo(t)
	commit(t, root, "empty")
	c, err := LoadConfigAt(root, "HEAD")
	if err != nil || !reflect.DeepEqual(c, DefaultConfig()) {
		t.Fatalf("missing at HEAD: %+v, %v", c, err)
	}

	write(t, filepath.Join(root, ".blocks", "config.toml"), "index_max = 5\n")
	commit(t, root, "config")
	// An uncommitted edit must grant nothing.
	write(t, filepath.Join(root, ".blocks", "config.toml"), "index_max = 5\nallow_effects = [\"external\"]\n")
	c, err = LoadConfigAt(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if c.IndexMax != 5 || len(c.AllowEffects) != 0 {
		t.Errorf("at HEAD: %+v", c)
	}

	if _, err := LoadConfigAt(root, "no-such-ref"); err == nil {
		t.Error("bad ref: no error")
	}
	if _, err := LoadConfigAt(root, "--output=x"); err == nil {
		t.Error("option-shaped ref: no error")
	}
}

func TestStateDir(t *testing.T) {
	root := gitRepo(t)
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	dir, err := StateDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != filepath.Join(state, "caveman-blocks") || len(filepath.Base(dir)) != 16 {
		t.Errorf("dir = %s", dir)
	}
	for _, sub := range []string{"candidates", "out", "cache"} {
		fi, err := os.Stat(filepath.Join(dir, sub))
		if err != nil || !fi.IsDir() {
			t.Errorf("%s missing: %v", sub, err)
		}
	}
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}

	// A subdirectory of the same repository shares the id.
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if again, _ := StateDir(sub); again != dir {
		t.Errorf("subdir id %s != %s", again, dir)
	}

	// Outside git, the id comes from the path and differs per directory.
	a, _ := StateDir(t.TempDir())
	b, _ := StateDir(t.TempDir())
	if a == b || a == dir {
		t.Errorf("non-git ids collide: %s %s", a, b)
	}
}

func TestBranches(t *testing.T) {
	root := gitRepo(t)
	commit(t, root, "one")
	if got := DefaultBranch(root); got != "main" {
		t.Errorf("DefaultBranch = %q", got)
	}
	if got := CurrentBranch(root); got != "main" {
		t.Errorf("CurrentBranch = %q", got)
	}
	base := run(t, root, "git", "rev-parse", "HEAD")
	run(t, root, "git", "checkout", "-q", "-b", "feat/x")
	commit(t, root, "two")
	if got := CurrentBranch(root); got != "feat/x" {
		t.Errorf("CurrentBranch = %q", got)
	}
	mb, err := MergeBase(root, "main")
	if err != nil || mb != base {
		t.Errorf("MergeBase = %q, %v; want %q", mb, err, base)
	}
	if _, err := MergeBase(root, "nope"); err == nil {
		t.Error("MergeBase of a missing branch: no error")
	}

	run(t, root, "git", "config", "init.defaultBranch", "trunk")
	if got := DefaultBranch(root); got != "trunk" {
		t.Errorf("DefaultBranch with init.defaultBranch = %q", got)
	}
	run(t, root, "git", "update-ref", "refs/remotes/origin/develop", "HEAD")
	run(t, root, "git", "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
	if got := DefaultBranch(root); got != "develop" {
		t.Errorf("DefaultBranch with origin/HEAD = %q", got)
	}
}

func TestInstructionFiles(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "GEMINI.md"), "x")
	write(t, filepath.Join(root, "AGENTS.md"), "x")
	if got := InstructionFiles(root); !reflect.DeepEqual(got, []string{"AGENTS.md", "GEMINI.md"}) {
		t.Errorf("got %v", got)
	}
}

func TestIsCachePath(t *testing.T) {
	for p, want := range map[string]bool{
		"/Users/a/.npm/_npx/abc/node_modules/.bin/caveman-blocks": true,
		"/home/a/.npm/caveman-blocks":                              true,
		"/x/node_modules/caveman-blocks/bin":                       true,
		"/usr/local/bin/caveman-blocks":                            false,
		"/Users/a/.local/share/caveman-blocks/bin/caveman-blocks":  false,
	} {
		if got := IsCachePath(p); got != want {
			t.Errorf("IsCachePath(%s) = %v", p, got)
		}
	}
}

func TestInstallBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := filepath.Join(t.TempDir(), "caveman-blocks")
	write(t, src, "#!/bin/sh\necho caveman-blocks 1.2.3\n")

	path, changed, err := InstallBinary(src, "1.2.3")
	if err != nil || !changed || path != filepath.Join(home, ".local/share/caveman-blocks/bin/caveman-blocks") {
		t.Fatalf("first install: %s %v %v", path, changed, err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm()&0o100 == 0 {
		t.Error("installed binary not executable")
	}
	if _, changed, err = InstallBinary(src, "1.2.3"); err != nil || changed {
		t.Errorf("same version: changed=%v err=%v", changed, err)
	}
	write(t, src, "#!/bin/sh\necho caveman-blocks 1.2.4\n")
	if _, changed, err = InstallBinary(src, "1.2.4"); err != nil || !changed {
		t.Errorf("new version: changed=%v err=%v", changed, err)
	}
	if got := installedVersion(path); got != "1.2.4" {
		t.Errorf("installed version %q", got)
	}
}

func TestListBlocksAndFixtures(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".blocks", "b.py"), "")
	write(t, filepath.Join(root, ".blocks", "a.py"), "")
	write(t, filepath.Join(root, ".blocks", "config.toml"), "")
	write(t, filepath.Join(root, ".blocks", "fixtures", "a", "x.json"), "{}")
	write(t, filepath.Join(root, ".blocks", "fixtures", "a", "sub", "y.txt"), "y")
	if err := os.Symlink(filepath.Join(root, ".blocks", "a.py"), filepath.Join(root, ".blocks", "c.py")); err != nil {
		t.Fatal(err)
	}

	got, err := ListBlocks(root)
	want := []string{filepath.Join(root, ".blocks", "a.py"), filepath.Join(root, ".blocks", "b.py")}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ListBlocks = %v, %v", got, err)
	}

	files, err := FixtureFiles(root, "a")
	if err != nil || len(files) != 2 || string(files["x.json"]) != "{}" || string(files["sub/y.txt"]) != "y" {
		t.Errorf("FixtureFiles(a) = %v, %v", files, err)
	}
	if files, err := FixtureFiles(root, "b"); err != nil || len(files) != 0 {
		t.Errorf("FixtureFiles(b) = %v, %v", files, err)
	}
	if _, err := FixtureFiles(root, "../a"); err == nil {
		t.Error("traversal name: no error")
	}
	if err := os.Symlink("/etc/hosts", filepath.Join(root, ".blocks", "fixtures", "a", "h")); err != nil {
		t.Fatal(err)
	}
	if _, err := FixtureFiles(root, "a"); err == nil {
		t.Error("symlinked fixture: no error")
	}
}
