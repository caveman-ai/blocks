package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bin = "/opt/cb/caveman-blocks"

var formats = []string{"claude-settings", "codex-hooks", "cursor-hooks"}

func fixture(t *testing.T, format, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../testdata/install", format, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func expect(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s\n got:\n%s\nwant:\n%s", path, got, want)
	}
}

func mustChange(t *testing.T, want bool, changed bool, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if changed != want {
		t.Fatalf("changed = %v, want %v", changed, want)
	}
}

func TestInstallCycle(t *testing.T) {
	for _, format := range formats {
		f, err := For(format)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(format+"/missing file", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", "config.json")
			if ok, _, err := f.Status(path); ok || err != nil {
				t.Fatalf("status on a missing file: %v %v", ok, err)
			}
			c, err := f.Install(path, bin)
			mustChange(t, true, c, err)
			expect(t, path, fixture(t, format, "empty.installed.json"))
			if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
				t.Errorf("new file mode %v", fi.Mode().Perm())
			}
			c, err = f.Install(path, bin)
			mustChange(t, false, c, err)
			if ok, b, err := f.Status(path); !ok || b != bin || err != nil {
				t.Fatalf("status: %v %q %v", ok, b, err)
			}
			c, err = f.Uninstall(path)
			mustChange(t, true, c, err)
			expect(t, path, fixture(t, format, "empty.uninstalled.json"))
			c, err = f.Uninstall(path)
			mustChange(t, false, c, err)
		})
		t.Run(format+"/empty file", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			os.WriteFile(path, nil, 0o644)
			c, err := f.Install(path, bin)
			mustChange(t, true, c, err)
			expect(t, path, fixture(t, format, "empty.installed.json"))
			if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
				t.Errorf("mode not kept: %v", fi.Mode().Perm())
			}
		})
		t.Run(format+"/other hooks kept", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			others := fixture(t, format, "others.json")
			os.WriteFile(path, others, 0o600)
			c, err := f.Install(path, bin)
			mustChange(t, true, c, err)
			expect(t, path, fixture(t, format, "others.installed.json"))
			c, err = f.Install(path, bin)
			mustChange(t, false, c, err)
			c, err = f.Uninstall(path)
			mustChange(t, true, c, err)
			expect(t, path, others)
		})
		t.Run(format+"/stale install replaced", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			os.WriteFile(path, fixture(t, format, "stale.json"), 0o600)
			if ok, b, _ := f.Status(path); !ok || !strings.HasPrefix(b, "/old/") {
				t.Fatalf("stale status: %v %q", ok, b)
			}
			c, err := f.Install(path, bin)
			mustChange(t, true, c, err)
			expect(t, path, fixture(t, format, "stale.installed.json"))
		})
	}
}

func TestInstallRefuses(t *testing.T) {
	f, _ := For("claude-settings")
	dir := t.TempDir()
	for _, b := range []string{"caveman-blocks", "/usr/local/bin/other"} {
		if _, err := f.Install(filepath.Join(dir, "s.json"), b); err == nil {
			t.Errorf("installed bin %q", b)
		}
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"hooks": [`), 0o600)
	if _, err := f.Install(bad, bin); err == nil {
		t.Error("installed into invalid JSON")
	}
	if _, err := f.Uninstall(bad); err == nil {
		t.Error("uninstalled from invalid JSON")
	}
	if b, _ := os.ReadFile(bad); string(b) != `{"hooks": [` {
		t.Error("invalid file was rewritten")
	}
	notObj := filepath.Join(dir, "hooks-array.json")
	os.WriteFile(notObj, []byte(`{"hooks": []}`), 0o600)
	if _, err := f.Install(notObj, bin); err == nil {
		t.Error("installed into a non-object hooks value")
	}
	if _, err := For("vim-hooks"); err == nil {
		t.Error("unknown format accepted")
	}
}

func TestInstallFollowsSymlinkAndQuotes(t *testing.T) {
	f, _ := For("cursor-hooks")
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "hooks.json")
	os.MkdirAll(filepath.Dir(target), 0o700)
	os.WriteFile(target, []byte("{}"), 0o600)
	link := filepath.Join(dir, "hooks.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	spaced := "/Users/Jane Doe/.local/share/caveman-blocks/bin/caveman-blocks"
	c, err := f.Install(link, spaced)
	mustChange(t, true, c, err)
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced by a file")
	}
	b, _ := os.ReadFile(target)
	if !strings.Contains(string(b), `"'/Users/Jane Doe/.local/share/caveman-blocks/bin/caveman-blocks' hook --harness cursor --phase pre"`) {
		t.Fatalf("path not shell-quoted:\n%s", b)
	}
	if ok, got, _ := f.Status(link); !ok || got != spaced {
		t.Fatalf("status bin %q", got)
	}
	c, err = f.Uninstall(link)
	mustChange(t, true, c, err)
	if b, _ := os.ReadFile(target); string(b) != "{\n  \"version\": 1\n}\n" {
		t.Fatalf("after uninstall:\n%s", b)
	}
}

// TestInstallFile covers the opencode-plugin format: a whole file, written once, removed only when ours.
func TestInstallFile(t *testing.T) {
	f, err := For("opencode-plugin")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plugins", "caveman-blocks.js")
	if ok, _, err := f.Status(path); ok || err != nil {
		t.Fatalf("status on a missing file: %v %v", ok, err)
	}
	c, err := f.Install(path, bin)
	mustChange(t, true, c, err)
	b, _ := os.ReadFile(path)
	for _, want := range []string{"// /opt/cb/caveman-blocks hook --harness generic\n", `const INSTALLED = "/opt/cb/caveman-blocks";`, `"tool.execute.after"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in:\n%s", want, b)
		}
	}
	if strings.Contains(string(b), `const INSTALLED = "";`) {
		t.Error("placeholder left in place")
	}
	c, err = f.Install(path, bin)
	mustChange(t, false, c, err)
	if ok, got, err := f.Status(path); !ok || got != bin || err != nil {
		t.Fatalf("status: %v %q %v", ok, got, err)
	}
	c, err = f.Uninstall(path)
	mustChange(t, true, c, err)
	if _, err := os.Stat(path); err == nil {
		t.Fatal("file not removed")
	}
	c, err = f.Uninstall(path)
	mustChange(t, false, c, err)

	theirs := []byte("export const Mine = async () => ({})\n")
	os.WriteFile(path, theirs, 0o600)
	if ok, _, _ := f.Status(path); ok {
		t.Error("a user's plugin taken as ours")
	}
	c, err = f.Uninstall(path)
	mustChange(t, false, c, err)
	if b, _ := os.ReadFile(path); string(b) != string(theirs) {
		t.Error("a user's plugin was touched")
	}
}

// TestMarker pins which commands uninstall treats as ours: only what entries writes.
func TestMarker(t *testing.T) {
	ours := []string{
		"/opt/cb/caveman-blocks hook --harness claude",
		"/old/path/caveman-blocks hook --harness codex",
		"/caveman-blocks hook --harness cursor --phase post",
		"'/Users/Jane Doe/bin/caveman-blocks' hook --harness cursor --phase pre",
		"'/Users/O'\\''Brien/bin/caveman-blocks' hook --harness claude",
		`C:\Users\me\caveman-blocks.exe hook --harness claude`,
		`'C:\Program Files\cb\caveman-blocks.exe' hook --harness claude`,
	}
	notOurs := []string{
		"caveman-blocks hook --harness claude",                     // relative: a user's own entry
		"~/bin/caveman-blocks hook --harness claude",               // not absolute
		"/opt/cb/not-caveman-blocks hook --harness claude",         // another binary
		"/opt/cb/caveman-blocks hook",                              // no harness
		"/opt/cb/caveman-blocks hook --harness claude && say done", // a wrapper
		"/opt/cb/caveman-blocks hook --harness claude --phase mid",
		"/opt/cb/caveman-blocks.exe hook --harness claude", // .exe only after a backslash
		"bash -c '/opt/cb/caveman-blocks hook --harness claude'",
		"/opt/cb/caveman-blocks run json-peek",
	}
	for _, c := range ours {
		if !marker.MatchString(c) {
			t.Errorf("not recognised as ours: %s", c)
		}
	}
	for _, c := range notOurs {
		if marker.MatchString(c) {
			t.Errorf("taken as ours: %s", c)
		}
	}
}

// TestUninstallKeepsLookalikes installs next to user entries that mention caveman-blocks and checks
// uninstall leaves them.
func TestUninstallKeepsLookalikes(t *testing.T) {
	f, _ := For("cursor-hooks")
	path := filepath.Join(t.TempDir(), "hooks.json")
	others := "{\n  \"version\": 1,\n  \"hooks\": {\n    \"beforeShellExecution\": [\n      {\n        \"command\": \"caveman-blocks hook --harness cursor --phase pre\"\n      },\n      {\n        \"command\": \"/opt/cb/caveman-blocks hook --harness cursor --phase pre | tee /tmp/log\"\n      }\n    ]\n  }\n}\n"
	os.WriteFile(path, []byte(others), 0o600)
	c, err := f.Install(path, bin)
	mustChange(t, true, c, err)
	c, err = f.Uninstall(path)
	mustChange(t, true, c, err)
	expect(t, path, []byte(others))
}
