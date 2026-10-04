//go:build !windows

package repo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// outsideFile makes <tmp>/outside/file holding "keep" and returns its path.
func outsideFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "outside", "file")
	write(t, p, "keep")
	return p
}

func assertKept(t *testing.T, p string) {
	t.Helper()
	if b, err := os.ReadFile(p); err != nil || string(b) != "keep" {
		t.Errorf("%s changed: %q, %v", p, b, err)
	}
}

func TestWriteFileSafeRefusesSymlinks(t *testing.T) {
	root := t.TempDir()
	out := outsideFile(t)
	if err := os.Symlink(out, filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(out), filepath.Join(root, ".claude")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"AGENTS.md", ".claude/file", ".claude/skills/x/SKILL.md", "../outside/file", "/etc/x"} {
		if err := WriteFileSafe(root, rel, []byte("pwned"), 0o644); err == nil {
			t.Errorf("WriteFileSafe(%s): no error", rel)
		}
		if err := RemoveAllSafe(root, rel); err == nil {
			t.Errorf("RemoveAllSafe(%s): no error", rel)
		}
	}
	assertKept(t, out)

	if err := WriteFileSafe(root, "a/b/c.txt", []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(filepath.Join(root, "a", "b", "c.txt"))
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Errorf("written file: %v, %v", fi, err)
	}
	if err := RemoveAllSafe(root, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Errorf("a not removed: %v", err)
	}
	if err := RemoveAllSafe(root, "missing/x"); err != nil {
		t.Errorf("missing path: %v", err)
	}
}

func TestInstructionFilesSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "AGENTS.md"), "x")
	if err := os.Symlink(outsideFile(t), filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	if got := InstructionFiles(root); len(got) != 1 || got[0] != "AGENTS.md" {
		t.Errorf("got %v", got)
	}
}

func TestFixtureFilesRefusesSymlinkedDirs(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "a", "secret"), "s")

	root := t.TempDir()
	write(t, filepath.Join(root, ".blocks", "a.py"), "")
	if err := os.Symlink(home, filepath.Join(root, ".blocks", "fixtures")); err != nil {
		t.Fatal(err)
	}
	if _, err := FixtureFiles(root, "a"); err == nil {
		t.Error("symlinked .blocks/fixtures: no error")
	}

	root = t.TempDir()
	write(t, filepath.Join(root, ".blocks", "fixtures", ".keep"), "")
	if err := os.Symlink(filepath.Join(home, "a"), filepath.Join(root, ".blocks", "fixtures", "a")); err != nil {
		t.Fatal(err)
	}
	if _, err := FixtureFiles(root, "a"); err == nil {
		t.Error("symlinked .blocks/fixtures/a: no error")
	}
}

func TestFixtureFilesSkipsFIFOAndCapsSize(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".blocks", "fixtures", "a")
	write(t, filepath.Join(dir, "x.json"), "{}")
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600); err != nil {
		t.Skip("no mkfifo:", err)
	}
	files, err := FixtureFiles(root, "a") // a FIFO opened for reading would block forever
	if err != nil || len(files) != 1 || string(files["x.json"]) != "{}" {
		t.Errorf("FixtureFiles with a FIFO = %v, %v", files, err)
	}

	write(t, filepath.Join(dir, "big1"), strings.Repeat("x", FixtureMax/2))
	if _, err := FixtureFiles(root, "a"); err != nil {
		t.Errorf("under the cap: %v", err)
	}
	write(t, filepath.Join(dir, "big2"), strings.Repeat("x", FixtureMax/2))
	if _, err := FixtureFiles(root, "a"); !errors.Is(err, ErrFixturesTooLarge) {
		t.Errorf("over the cap: %v", err)
	}
}
