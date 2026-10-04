package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/registry"
	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
	"github.com/spf13/cobra"
)

func lintCmd() *cobra.Command {
	var firstParty bool
	c := &cobra.Command{
		Use:   "lint [path|dir] [--first-party]",
		Short: "Check block files against the format rules; exit 1 on any F finding.",
		Args:  args(cobra.MaximumNArgs(1)),
		RunE: func(c *cobra.Command, a []string) error {
			target := ""
			if len(a) == 1 {
				target = a[0]
			} else {
				root, err := findRoot()
				if err != nil {
					return err
				}
				target = filepath.Join(root, ".blocks")
				if wd, err := os.Getwd(); err == nil {
					if rel, err := filepath.Rel(wd, target); err == nil {
						target = rel
					}
				}
			}
			files, err := blockFiles(target)
			if err != nil {
				return err
			}
			opts := blockfile.LintOptions{FirstParty: firstParty}
			if !firstParty { // W001 is about promotion; registry blocks are not promoted
				dir, _ := os.Getwd()
				if root, ok := repo.FindRoot(dir); ok {
					dir = root
				}
				br := repo.CurrentBranch(dir)
				opts.OnDefaultBranch = br != "" && br == repo.DefaultBranch(dir)
			}
			out := c.OutOrStdout()
			failed := false
			for _, f := range files {
				for _, fd := range lintFile(f, opts) {
					failed = failed || strings.HasPrefix(fd.Code, "F")
					loc := f
					if fd.Line > 0 {
						loc = fmt.Sprintf("%s:%d", f, fd.Line)
					}
					fmt.Fprintf(out, "%s %s (%s)\n", fd.Code, fd.Message, loc)
				}
			}
			if failed {
				return errFailed
			}
			if len(files) == 0 {
				fmt.Fprintln(out, "lint: no blocks in", target)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&firstParty, "first-party", false, "also apply F012: standard library imports only, one per line")
	return c
}

// blockFiles is target itself when it is a file, else the *.py files directly inside it.
func blockFiles(target string) ([]string, error) {
	fi, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return []string{target}, nil
	}
	return filepath.Glob(filepath.Join(target, "*.py"))
}

func lintFile(path string, opts blockfile.LintOptions) []blockfile.Finding {
	src, err := os.ReadFile(path)
	if err != nil {
		return []blockfile.Finding{{Code: "F001", Message: err.Error()}}
	}
	b, err := blockfile.Parse(path, src)
	if err != nil {
		var f blockfile.Finding
		if errors.As(err, &f) {
			return []blockfile.Finding{f}
		}
		return []blockfile.Finding{{Code: "F001", Message: err.Error()}}
	}
	findings := blockfile.Lint(b, opts)
	// Fixtures sit beside the block: .blocks/fixtures/<name> or, first-party, blocks/fixtures/<name>.
	// Under .blocks/ they are read like verify and sync do, so a symlinked fixtures dir fails here too.
	if registry.ValidName(b.Header.Name) {
		dir := filepath.Dir(path)
		var err error
		if abs, e := filepath.Abs(dir); e == nil && filepath.Base(abs) == ".blocks" {
			_, err = repo.FixtureFiles(filepath.Dir(abs), b.Header.Name)
		} else {
			_, err = repo.ReadFixtureDir(filepath.Join(dir, "fixtures", b.Header.Name))
		}
		if err != nil {
			findings = append(findings, blockfile.Finding{Code: "F015", Message: "fixtures: " + err.Error()})
		}
	}
	return findings
}
