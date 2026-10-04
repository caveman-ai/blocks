package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/export"
	"github.com/JuliusBrussee/caveman-blocks/internal/index"
	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
	"github.com/spf13/cobra"
)

func syncCmd() *cobra.Command {
	var check bool
	c := &cobra.Command{
		Use:   "sync [--check]",
		Short: "Regenerate INDEX.md, the AGENTS.md section, import lines and existing exports.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := findRoot()
			if err != nil {
				return err
			}
			if !check {
				refreshBinary()
			}
			files, err := syncRepo(root, check, false)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if check {
				if len(files) == 0 {
					fmt.Fprintln(out, "sync: up to date")
					return nil
				}
				for _, f := range files {
					fmt.Fprintln(out, "stale:", f)
				}
				return exitError{1, "generated text is stale; run caveman-blocks sync and commit"}
			}
			printChanged(out, files)
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "write nothing; exit 1 listing stale files")
	return c
}

func printChanged(out io.Writer, files []string) {
	if len(files) == 0 {
		fmt.Fprintln(out, "sync: nothing changed")
	}
	for _, f := range files {
		fmt.Fprintln(out, "changed:", f)
	}
}

// syncRepo renders every generated file and writes those whose bytes differ, returning their
// root-relative paths. With check it writes nothing. Exports are refreshed where an export directory
// already exists, or created when forceExport is set (the export command).
func syncRepo(root string, check, forceExport bool) ([]string, error) {
	cfg, err := repo.LoadConfig(root)
	if err != nil {
		return nil, err
	}
	all, errs := loadBlocks(root)
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "caveman-blocks: skipped:", oneLine(e.Error()))
	}
	var indexed []*blockfile.Block
	var entries []index.Entry
	for _, l := range all {
		if l.indexed {
			indexed = append(indexed, l.b)
			entries = append(entries, index.Entry{Name: l.b.Header.Name, Params: blockfile.ParamsColumn(l.b), Summary: l.b.Header.Summary})
		}
	}
	body, err := index.Body(entries, cfg.IndexMax, index.DefaultBudget)
	if err != nil {
		return nil, err
	}

	want := map[string][]byte{".blocks/INDEX.md": []byte(body + "\n")}
	agents := body
	if index.Mode(cfg.Section) == index.ModeImport {
		agents = index.PointerBody
	}
	upsert := func(name, section string) error {
		cur, err := readSafe(root, name)
		if err != nil {
			return err
		}
		want[name] = index.Upsert(cur, section)
		return nil
	}
	if err := upsert("AGENTS.md", agents); err != nil {
		return nil, err
	}
	for _, name := range repo.InstructionFiles(root) {
		if name != "AGENTS.md" {
			if err := upsert(name, index.ImportBody); err != nil {
				return nil, err
			}
		}
	}
	remove := exports(root, indexed, forceExport, want)

	var changed []string
	for name, data := range want {
		cur, err := readSafe(root, name)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(cur, data) {
			continue
		}
		changed = append(changed, name)
		if check {
			continue
		}
		if err := repo.WriteFileSafe(root, name, data, 0o644); err != nil {
			return nil, err
		}
	}
	for _, name := range remove {
		changed = append(changed, name)
		if !check {
			if err := repo.RemoveAllSafe(root, name); err != nil {
				return nil, err
			}
			if p, err := repo.SafePath(root, filepath.Dir(name)); err == nil {
				os.Remove(p) // only when now empty
			}
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// readSafe reads root/rel through repo.SafePath; a missing file is nil content, a symlink an error.
func readSafe(root, rel string) ([]byte, error) {
	p, err := repo.SafePath(root, rel)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// exports adds the SKILL.md files for indexed blocks to want, in each export dir that exists (or
// every one when force), and returns generated SKILL.md files of blocks no longer indexed. An
// export dir reached through a symlink counts as absent.
func exports(root string, indexed []*blockfile.Block, force bool, want map[string][]byte) []string {
	var remove []string
	for _, dir := range export.Dirs {
		p, err := repo.SafePath(root, dir)
		if err != nil {
			continue
		}
		if _, err := os.Lstat(p); err != nil && !force {
			continue
		}
		keep := map[string]bool{}
		for _, b := range indexed {
			name := dir + "/" + b.Header.Name + "/SKILL.md"
			want[name] = export.Skill(b)
			keep[name] = true
		}
		old, _ := filepath.Glob(filepath.Join(root, dir, "*", "SKILL.md"))
		for _, p := range old {
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if keep[rel] {
				continue
			}
			if data, err := readSafe(root, rel); err == nil && strings.Contains(string(data), export.Marker) {
				remove = append(remove, rel)
			}
		}
	}
	return remove
}
