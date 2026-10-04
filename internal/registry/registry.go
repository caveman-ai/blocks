// Package registry serves the first-party blocks embedded in the binary and installs them into a
// repo's .blocks/ with a blocks.lock entry (docs/ARCHITECTURE.md, "Add").
package registry

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/pelletier/go-toml/v2"
)

// Registry reads blocks from FS: `<name>.py` at the root, fixtures under `fixtures/<name>/`, and an
// optional `VERSION` file.
type Registry struct {
	FS fs.FS
	// Parse parses a block source; nil means blockfile.Parse.
	Parse func(path string, src []byte) (*blockfile.Block, error)
}

// LockEntry is one table in .blocks/blocks.lock.
type LockEntry struct {
	Source  string `toml:"source"`
	Version string `toml:"version"`
	Hash    string `toml:"hash"`
}

// AddResult reports what Add wrote.
type AddResult struct {
	Path     string           // .blocks/<name>.py, absolute
	Fixtures []string         // fixture relpaths written, sorted
	Effect   blockfile.Effect // declared effect, for the allow_effects grant line
	Entry    LockEntry
}

var validName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidName reports whether name is a legal block name (docs/FORMAT.md, `name`).
func ValidName(name string) bool { return len(name) <= 64 && validName.MatchString(name) }

// List returns the embedded block names, sorted.
func (r Registry) List() ([]string, error) {
	files, err := fs.Glob(r.FS, "*.py")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, strings.TrimSuffix(f, ".py"))
	}
	sort.Strings(names)
	return names, nil
}

// Read returns the block source and its fixtures keyed by path relative to fixtures/<name>/.
func (r Registry) Read(name string) ([]byte, map[string][]byte, error) {
	if !ValidName(name) {
		return nil, nil, fmt.Errorf("invalid block name %q", name)
	}
	src, err := fs.ReadFile(r.FS, name+".py")
	if err != nil {
		return nil, nil, fmt.Errorf("no first-party block %q: %w", name, err)
	}
	fixtures := map[string][]byte{}
	dir := path.Join("fixtures", name)
	err = fs.WalkDir(r.FS, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(r.FS, p)
		if err != nil {
			return err
		}
		fixtures[strings.TrimPrefix(p, dir+"/")] = b
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	return src, fixtures, nil
}

// Version returns the trimmed VERSION file in FS, or "0.0.0".
func (r Registry) Version() string {
	b, err := fs.ReadFile(r.FS, "VERSION")
	if v := strings.TrimSpace(string(b)); err == nil && v != "" {
		return v
	}
	return "0.0.0"
}

// Add copies block name and its fixtures into root/.blocks, strips any [stamp] table, and records
// the block in blocks.lock with hash = hashFn(stripped source, fixtures). An existing block file is
// refused unless force is set; with force its fixture directory is replaced.
func (r Registry) Add(root, name string, force bool, hashFn func(src []byte, fixtures map[string][]byte) string) (AddResult, error) {
	src, fixtures, err := r.Read(name)
	if err != nil {
		return AddResult{}, err
	}
	src = StripStamp(src)
	parse := r.Parse
	if parse == nil {
		parse = blockfile.Parse
	}
	b, err := parse(name+".py", src)
	if err != nil {
		return AddResult{}, fmt.Errorf("embedded block %s: %w", name, err)
	}

	blocks := filepath.Join(root, ".blocks")
	dst := filepath.Join(blocks, name+".py")
	fixDir := filepath.Join(blocks, "fixtures", name)
	for _, p := range []string{dst, fixDir} {
		fi, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if !force {
			return AddResult{}, fmt.Errorf("%s exists; pass --force to overwrite", p)
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return AddResult{}, fmt.Errorf("%s is a symlink; refusing to write through it", p)
		}
	}
	if err := os.RemoveAll(fixDir); err != nil {
		return AddResult{}, err
	}
	if err := os.MkdirAll(blocks, 0o755); err != nil {
		return AddResult{}, err
	}
	if err := os.WriteFile(dst, src, 0o755); err != nil { // executable: ruff EXE001, direct python3 .blocks/x.py
		return AddResult{}, err
	}
	rels := make([]string, 0, len(fixtures))
	for rel, data := range fixtures {
		p := filepath.Join(fixDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return AddResult{}, err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return AddResult{}, err
		}
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	v := r.Version()
	entry := LockEntry{Source: "registry:" + name + "@" + v, Version: v, Hash: hashFn(src, fixtures)}
	lock, err := Lock(root)
	if err != nil {
		return AddResult{}, err
	}
	lock[name] = entry
	if err := WriteLock(root, lock); err != nil {
		return AddResult{}, err
	}
	return AddResult{Path: dst, Fixtures: rels, Effect: b.Header.Effects, Entry: entry}, nil
}

// StripStamp removes the `# [stamp]` table textually: from the `# [stamp]` line up to the closing
// `# ///`, plus a bare `#` line directly before it. Source without a stamp is returned unchanged.
func StripStamp(src []byte) []byte {
	lines := bytes.SplitAfter(src, []byte("\n"))
	trim := func(l []byte) string { return strings.TrimRight(string(l), "\r\n") }
	start := -1
	for i, l := range lines {
		if trim(l) == "# [stamp]" {
			start = i
			break
		}
	}
	if start < 0 {
		return src
	}
	end := start
	for end < len(lines) && trim(lines[end]) != "# ///" {
		end++
	}
	if end == len(lines) { // no closing fence: not a header we understand
		return src
	}
	if start > 0 && trim(lines[start-1]) == "#" {
		start--
	}
	out := bytes.Join(lines[:start], nil)
	return append(out, bytes.Join(lines[end:], nil)...)
}

func lockPath(root string) string { return filepath.Join(root, ".blocks", "blocks.lock") }

// Lock reads root/.blocks/blocks.lock. A missing file is an empty lock.
func Lock(root string) (map[string]LockEntry, error) {
	m := map[string]LockEntry{}
	b, err := os.ReadFile(lockPath(root))
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("blocks.lock: %w", err)
	}
	return m, nil
}

// WriteLock writes the lock, one table per block sorted by name.
func WriteLock(root string, lock map[string]LockEntry) error {
	b, err := toml.Marshal(lock)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".blocks"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(lockPath(root), b, 0o644)
}
