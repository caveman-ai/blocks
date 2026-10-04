package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/blocks"
	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/registry"
	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
)

// findRoot resolves the repo root from the working directory, or explains how to create one.
func findRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, ok := repo.FindRoot(wd)
	if !ok {
		return "", errors.New("no .blocks/ here or in any parent directory; run caveman-blocks init at the repo root")
	}
	return root, nil
}

// loaded is one parsed block in .blocks/ with its current content hash.
type loaded struct {
	b       *blockfile.Block
	fx      map[string][]byte
	hash    string
	indexed bool
}

// loadBlocks parses every .blocks/*.py. Files that do not parse are returned as errors, not blocks.
func loadBlocks(root string) ([]loaded, []error) {
	paths, err := repo.ListBlocks(root)
	if err != nil {
		return nil, []error{err}
	}
	var out []loaded
	var errs []error
	for _, p := range paths {
		l, err := loadBlock(root, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, l)
	}
	return out, errs
}

func loadBlock(root, path string) (loaded, error) {
	b, err := blockfile.Load(path)
	if err != nil {
		return loaded{}, err
	}
	fx, err := repo.FixtureFiles(root, b.Header.Name)
	if errors.Is(err, repo.ErrFixturesTooLarge) {
		return loaded{b: b}, nil // never indexed; lint and verify report why
	}
	if err != nil {
		return loaded{}, err
	}
	h := blockfile.ContentHash(b, fx)
	return loaded{b: b, fx: fx, hash: h, indexed: blockfile.Indexed(b, h)}, nil
}

// blockByName loads .blocks/<name>.py.
func blockByName(root, name string) (loaded, error) {
	if !registry.ValidName(name) {
		return loaded{}, usageError{fmt.Errorf("invalid block name %q", name)}
	}
	p := filepath.Join(root, ".blocks", name+".py")
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return loaded{}, fmt.Errorf("no block %s in .blocks/", name)
	}
	return loadBlock(root, p)
}

// policy reads allow_effects as committed at ref. Outside git it falls back to the working tree,
// and in a repository with no commits yet to the defaults; both say so on stderr.
func policy(root, ref string) ([]string, error) {
	if _, err := repo.Git(root, "rev-parse", "--git-dir"); err != nil {
		fmt.Fprintln(os.Stderr, "caveman-blocks: not a git repository; reading allow_effects from the working tree")
		cfg, err := repo.LoadConfig(root)
		return cfg.AllowEffects, err
	}
	if ref == "HEAD" {
		if _, err := repo.Git(root, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
			fmt.Fprintln(os.Stderr, "caveman-blocks: no commits yet; using the default allow_effects")
			return repo.DefaultConfig().AllowEffects, nil
		}
	}
	cfg, err := repo.LoadConfigAt(root, ref)
	return cfg.AllowEffects, err
}

// firstParty parses every embedded first-party block.
func firstParty() ([]*blockfile.Block, error) {
	reg := registry.Registry{FS: blocks.FS}
	names, err := reg.List()
	if err != nil {
		return nil, err
	}
	var out []*blockfile.Block
	for _, n := range names {
		src, _, err := reg.Read(n)
		if err != nil {
			return nil, err
		}
		b, err := blockfile.Parse(n+".py", src)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// parseSince accepts Go durations plus a day suffix: 30d, 7d, 36h.
func parseSince(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		d, err := strconv.Atoi(n)
		if err != nil || d < 0 {
			return 0, usageError{fmt.Errorf("invalid --since %q: want a number of days like 7d", s)}
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, usageError{fmt.Errorf("invalid --since %q: want 7d or a duration like 36h", s)}
	}
	return d, nil
}

// expandHome turns a leading ~/ into the home directory.
func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return p
}

// executable is the running binary, refused when it lives in an npm or npx cache.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	real := exe
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		real = r
	}
	for _, p := range []string{exe, real} {
		if repo.IsCachePath(p) {
			return "", fmt.Errorf("this binary runs from a package cache (%s) that may be deleted; install caveman-blocks with install.sh or brew first, then run hooks install again", p)
		}
	}
	return real, nil
}

// refreshBinary updates the hook's installed copy when one exists and its version differs from this
// binary's (docs/HOOK.md, failure policy). Dev builds and cache paths never replace it.
func refreshBinary() (string, bool) {
	home := repo.BinaryHome()
	if home == "" || version == "0.0.0-dev" {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(home, "caveman-blocks")); err != nil {
		return "", false
	}
	exe, err := executable()
	if err != nil {
		return "", false
	}
	p, changed, err := repo.InstallBinary(exe, version)
	return p, err == nil && changed
}
