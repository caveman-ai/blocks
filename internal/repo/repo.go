// Package repo locates the repo root and .blocks/, reads config.toml, owns the per-machine state
// directory and the installed binary, and answers the git questions other packages need
// (docs/ARCHITECTURE.md, docs/HOOK.md#state-directory).
package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// gitTimeout bounds every git call so a wedged repository cannot hang the CLI.
const gitTimeout = 10 * time.Second

// FindRoot returns the nearest directory at or above cwd that contains a .blocks directory.
// A .blocks that is a symlink is refused: ok is false and the search stops.
func FindRoot(cwd string) (root string, ok bool) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", false
	}
	for {
		fi, err := os.Lstat(filepath.Join(dir, ".blocks"))
		if err == nil {
			if fi.Mode()&fs.ModeSymlink != 0 {
				return "", false
			}
			if fi.IsDir() {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Config is .blocks/config.toml. Every key has a default; the file is optional.
type Config struct {
	Promote      string   `toml:"promote"`
	AllowEffects []string `toml:"allow_effects"`
	IndexMax     int      `toml:"index_max"`
	Section      string   `toml:"section"`
	Hint         bool     `toml:"hint"`
}

// DefaultConfig returns the values used for every key the file does not set.
func DefaultConfig() Config {
	return Config{Promote: "commit", AllowEffects: []string{}, IndexMax: 20, Section: "inline", Hint: true}
}

// ParseConfig decodes config.toml content over the defaults. Unknown keys are an error.
func ParseConfig(data []byte) (Config, error) {
	c := DefaultConfig()
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return Config{}, fmt.Errorf(".blocks/config.toml: unknown key:\n%s", strict.String())
		}
		return Config{}, fmt.Errorf(".blocks/config.toml: %w", err)
	}
	return c, nil
}

// LoadConfig reads .blocks/config.toml from the working tree. A missing file yields the defaults;
// anything other than a regular file (a symlink, say) is refused.
func LoadConfig(root string) (Config, error) {
	p := filepath.Join(root, ".blocks", "config.toml")
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return Config{}, err
	}
	if !fi.Mode().IsRegular() {
		return Config{}, fmt.Errorf("%s: not a regular file", p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(data)
}

// LoadConfigAt reads .blocks/config.toml as committed at ref (for example HEAD or origin/main),
// never from the working tree. Missing at ref yields the defaults; a git failure is an error.
func LoadConfigAt(root, ref string) (Config, error) {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return Config{}, fmt.Errorf("invalid ref %q", ref)
	}
	// ls-tree fails on a bad ref and prints nothing when the path is absent at a good one.
	entry, err := Git(root, "ls-tree", ref, "--", ".blocks/config.toml")
	if err != nil {
		return Config{}, fmt.Errorf("read policy at %s: %w", ref, err)
	}
	if entry == "" {
		return DefaultConfig(), nil
	}
	if !strings.HasPrefix(entry, "100644 blob ") && !strings.HasPrefix(entry, "100755 blob ") {
		return Config{}, fmt.Errorf(".blocks/config.toml at %s: not a regular file", ref)
	}
	data, err := Git(root, "show", ref+":.blocks/config.toml")
	if err != nil {
		return Config{}, fmt.Errorf("read policy at %s: %w", ref, err)
	}
	return ParseConfig([]byte(data))
}

// Git runs `git -C root args...` under a timeout and returns trimmed stdout. A failure carries
// git's stderr.
func Git(root string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// StateDir returns, creating it 0700 with candidates/, out/ and cache/, the per-machine state dir:
// $XDG_STATE_HOME (default ~/.local/state) /caveman-blocks/<repo-id>. repo-id is the first 16 hex of
// SHA-256 of the absolute git common dir, so worktrees share state, or of the absolute root outside git.
func StateDir(root string) (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	key, err := Git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || key == "" {
		if key, err = filepath.Abs(root); err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256([]byte(key))
	dir := filepath.Join(base, "caveman-blocks", hex.EncodeToString(sum[:])[:16])
	for _, sub := range []string{"candidates", "out", "cache"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// DefaultBranch names the repository's default branch: origin/HEAD's target, else
// init.defaultBranch, else main when that branch exists, else master.
func DefaultBranch(root string) string {
	if ref, err := Git(root, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil && ref != "" {
		return strings.TrimPrefix(ref, "refs/remotes/origin/")
	}
	if b, err := Git(root, "config", "init.defaultBranch"); err == nil && b != "" {
		return b
	}
	if _, err := Git(root, "rev-parse", "--verify", "--quiet", "refs/heads/main"); err == nil {
		return "main"
	}
	return "master"
}

// CurrentBranch returns the checked-out branch, or "" when detached or outside git.
func CurrentBranch(root string) string {
	b, _ := Git(root, "branch", "--show-current")
	return b
}

// MergeBase returns the merge base of HEAD and branch, trying origin/<branch> when the local
// branch is absent (CI checkouts usually have only the remote ref).
func MergeBase(root, branch string) (string, error) {
	if branch == "" || strings.HasPrefix(branch, "-") {
		return "", fmt.Errorf("invalid branch %q", branch)
	}
	mb, err := Git(root, "merge-base", "HEAD", branch)
	if err == nil {
		return mb, nil
	}
	if mb, err2 := Git(root, "merge-base", "HEAD", "origin/"+branch); err2 == nil {
		return mb, nil
	}
	return "", err
}

// InstructionFiles lists which of AGENTS.md, CLAUDE.md and GEMINI.md exist at root, in that order.
func InstructionFiles(root string) []string {
	var found []string
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			found = append(found, name)
		}
	}
	return found
}

// BinaryHome is the stable directory hooks point at: ~/.local/share/caveman-blocks/bin.
func BinaryHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "caveman-blocks", "bin")
}

// IsCachePath reports whether p lives in an npm or npx cache that may be garbage collected.
func IsCachePath(p string) bool {
	p = filepath.ToSlash(p)
	for _, s := range []string{"/_npx/", "/.npm/", "/node_modules/"} {
		if strings.Contains(p, s) {
			return true
		}
	}
	return false
}

// InstallBinary copies src to BinaryHome()/caveman-blocks when the copy is missing or its
// `version` output does not end in version. It writes a temp file and renames it, never in place,
// so a running hook is not disturbed.
func InstallBinary(src, version string) (path string, changed bool, err error) {
	home := BinaryHome()
	if home == "" {
		return "", false, errors.New("no home directory")
	}
	path = filepath.Join(home, "caveman-blocks")
	if installedVersion(path) == version {
		return path, false, nil
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", false, err
	}
	in, err := os.Open(src)
	if err != nil {
		return "", false, err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(home, ".caveman-blocks-*")
	if err != nil {
		return "", false, err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return "", false, err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		return "", false, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", false, err
	}
	return path, true, nil
}

// installedVersion runs `<bin> version` and returns its last word, or "" when it cannot run.
func installedVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

// ListBlocks returns the .blocks/*.py paths under root, sorted. Symlinks are skipped.
func ListBlocks(root string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(root, ".blocks", "*.py"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range matches { // Glob returns sorted paths
		if fi, err := os.Lstat(m); err == nil && fi.Mode().IsRegular() {
			out = append(out, m)
		}
	}
	return out, nil
}

// FixtureFiles reads .blocks/fixtures/<name>/** as slash-separated relpath → content. No directory
// means an empty map. A symlink anywhere under it is an error.
func FixtureFiles(root, name string) (map[string][]byte, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return nil, fmt.Errorf("invalid block name %q", name)
	}
	dir := filepath.Join(root, ".blocks", "fixtures", name)
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s: symlink in fixtures", p)
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
