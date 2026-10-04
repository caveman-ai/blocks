package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
	"github.com/spf13/cobra"
)

// configTemplate is the config.toml init writes: every key at its default, with what it does.
const configTemplate = `# Caveman Blocks repo policy (docs/FORMAT.md). Every key is optional; these are the defaults.

# How a promoted block lands: "commit" on the current branch.
promote = "commit"

# Effects blocks may use beyond read, write-workspace and exec: "network", "external".
# caveman-blocks run reads this from the committed file at HEAD; an uncommitted edit grants nothing.
allow_effects = []

# Most blocks listed in the index.
index_max = 20

# "inline" writes the index into AGENTS.md; "import" writes a pointer to .blocks/INDEX.md instead.
section = "inline"

# false keeps capture and stats but shows the agent no hints.
hint = true
`

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create .blocks/ and config.toml in the current directory, then sync.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			blocks, err := repo.SafePath(root, ".blocks") // refuses a symlinked .blocks
			if err != nil {
				return err
			}
			if err := os.MkdirAll(blocks, 0o755); err != nil {
				return err
			}
			if _, err := os.Lstat(filepath.Join(blocks, "config.toml")); errors.Is(err, fs.ErrNotExist) {
				if err := repo.WriteFileSafe(root, ".blocks/config.toml", []byte(configTemplate), 0o644); err != nil {
					return err
				}
				fmt.Fprintln(out, "created: .blocks/config.toml")
			}
			refreshBinary()
			files, err := syncRepo(root, false, false)
			if err != nil {
				return err
			}
			for _, f := range files {
				fmt.Fprintln(out, "changed:", f)
			}
			fmt.Fprint(out, `
Next steps:
  caveman-blocks hooks install     once per machine: hints and capture in your agents
  caveman-blocks add json-peek     add a first-party block (see caveman-blocks scan for which)
  git add .blocks AGENTS.md && git commit -m "blocks: init"
CI: add the workflow in docs/CI.md (lint, verify --check, sync --check).
`)
			return nil
		},
	}
}

func exportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Write SKILL.md wrappers for indexed blocks under .claude/skills and .agents/skills.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := findRoot()
			if err != nil {
				return err
			}
			files, err := syncRepo(root, false, true)
			if err != nil {
				return err
			}
			printChanged(c.OutOrStdout(), files)
			return nil
		},
	}
}
