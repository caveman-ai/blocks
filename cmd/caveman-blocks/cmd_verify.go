package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/registry"
	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
	"github.com/JuliusBrussee/caveman-blocks/internal/stats"
	"github.com/JuliusBrussee/caveman-blocks/internal/verify"
	"github.com/spf13/cobra"
)

func verifyCmd() *cobra.Command {
	var all, changed, check, firstParty bool
	var policyRef, fixturesRoot, blocksDir string
	c := &cobra.Command{
		Use:   "verify [name|--changed|--all] [--check] [--policy-ref r]",
		Short: "Run block examples, stamp passes and quarantine failures; --check writes nothing.",
		Args:  args(cobra.MaximumNArgs(1)),
		RunE: func(c *cobra.Command, a []string) error {
			if len(a) == 1 && (all || changed) || all && changed {
				return usageError{fmt.Errorf("give one of a block name, --changed or --all")}
			}
			var root string
			var paths []string
			var err error
			if blocksDir != "" { // a registry layout: blocks in their own directory, no .blocks/
				if root, err = os.Getwd(); err != nil {
					return err
				}
				if len(a) == 1 {
					if !registry.ValidName(a[0]) {
						return usageError{fmt.Errorf("invalid block name %q", a[0])}
					}
					paths = []string{filepath.Join(blocksDir, a[0]+".py")}
				} else if paths, err = filepath.Glob(filepath.Join(blocksDir, "*.py")); err != nil {
					return err
				}
			} else {
				if root, err = findRoot(); err != nil {
					return err
				}
				mode, arg := "changed", ""
				switch {
				case len(a) == 1:
					mode, arg = "name", a[0]
				case all:
					mode = "all"
				}
				if paths, err = verify.Select(root, mode, arg, repo.DefaultBranch(root)); err != nil {
					return err
				}
			}
			ref := policyRef
			if ref == "" {
				ref = "HEAD"
			}
			allow, err := policy(root, ref)
			if err != nil {
				return err
			}

			out := c.OutOrStdout()
			write := !check && !firstParty
			failed := false
			for _, p := range paths {
				o := verifyOne(root, p, allow, fixturesRoot, check, firstParty)
				printOutcome(out, o)
				failed = failed || o.Status == verify.Fail
				if write {
					recordVerify(root, o)
				}
			}
			if len(paths) == 0 {
				fmt.Fprintln(out, "verify: no blocks selected")
			}
			if write && blocksDir == "" {
				files, err := syncRepo(root, false, false)
				if err != nil {
					return err
				}
				printChanged(out, files)
			}
			if failed {
				return errFailed
			}
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&all, "all", false, "every block in .blocks/")
	f.BoolVar(&changed, "changed", false, "blocks changed since the merge base with the default branch (the default)")
	f.BoolVar(&check, "check", false, "write nothing; also fail on stale stamps and quarantined blocks (CI)")
	f.StringVar(&policyRef, "policy-ref", "", "read allow_effects from .blocks/config.toml at this ref (default HEAD)")
	f.StringVar(&fixturesRoot, "fixtures-root", "", "fixtures directory instead of .blocks/fixtures")
	f.BoolVar(&firstParty, "first-party", false, "registry blocks, which carry no stamp: pass when the example passes; writes nothing")
	f.StringVar(&blocksDir, "blocks-dir", "", "directory holding the blocks instead of .blocks/")
	f.MarkHidden("blocks-dir")
	return c
}

// verifyOne loads and verifies one block file. In first-party mode an unstamped block is checked as
// if stamped with its current hash, so only its example decides; nothing is written.
func verifyOne(root, path string, allow []string, fixturesRoot string, check, firstParty bool) verify.Outcome {
	name := strings.TrimSuffix(filepath.Base(path), ".py")
	b, err := blockfile.Load(path)
	if err != nil {
		return verify.Outcome{Name: name, Status: verify.Fail, Reason: oneLine(err.Error())}
	}
	var fx map[string][]byte
	if fixturesRoot != "" {
		fx, err = repo.ReadFixtureDir(filepath.Join(fixturesRoot, b.Header.Name))
	} else {
		fx, err = repo.FixtureFiles(root, b.Header.Name)
	}
	if err != nil {
		return verify.Outcome{Name: name, Status: verify.Fail, Reason: oneLine(err.Error())}
	}
	if firstParty && b.Header.Stamp == nil {
		b.Header.Stamp = &blockfile.Stamp{Verified: blockfile.ContentHash(b, fx)}
	}
	return verify.Verify(root, b, fx, allow, verify.Options{Check: check || firstParty, FixturesRoot: fixturesRoot})
}

func printOutcome(out io.Writer, o verify.Outcome) {
	line := fmt.Sprintf("%-4s %s %s", o.Status, o.Name, o.Hash)
	if o.Reason != "" {
		line += ": " + o.Reason
	}
	fmt.Fprintln(out, strings.TrimRight(line, " "))
}

// recordVerify appends a verify event; skips are not counted as pass or fail.
func recordVerify(root string, o verify.Outcome) {
	if o.Status == verify.Skip {
		return
	}
	dir, err := repo.StateDir(root)
	if err != nil {
		return
	}
	ok := o.Status == verify.Pass
	stats.Append(dir, stats.Event{Session: os.Getenv("CAVEMAN_BLOCKS_SESSION"), Kind: stats.KindVerify, Block: o.Name, OK: &ok})
}
