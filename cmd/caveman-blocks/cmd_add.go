package main

import (
	"errors"
	"fmt"

	"github.com/caveman-ai/blocks/blocks"
	"github.com/caveman-ai/blocks/internal/blockfile"
	"github.com/caveman-ai/blocks/internal/promote"
	"github.com/caveman-ai/blocks/internal/registry"
	"github.com/caveman-ai/blocks/internal/runner"
	"github.com/caveman-ai/blocks/internal/verify"
	"github.com/spf13/cobra"
)

func addCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "add <name> [--force]",
		Short: "Copy a first-party block and its fixtures into .blocks/, verify it and sync.",
		Args:  args(cobra.ExactArgs(1)),
		RunE: func(c *cobra.Command, a []string) error {
			root, err := findRoot()
			if err != nil {
				return err
			}
			name := a[0]
			hash := func(src []byte, fx map[string][]byte) string {
				b, err := blockfile.Parse(name+".py", src)
				if err != nil {
					return ""
				}
				return blockfile.ContentHash(b, fx)
			}
			res, err := registry.Registry{FS: blocks.FS}.Add(root, name, force, hash)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			fmt.Fprintf(out, "added: .blocks/%s.py (%s)\n", name, res.Entry.Source)
			for _, f := range res.Fixtures {
				fmt.Fprintf(out, "added: .blocks/fixtures/%s/%s\n", name, f)
			}
			allow, err := policy(root, "HEAD")
			if err != nil {
				return err
			}
			l, err := blockByName(root, name)
			if err != nil {
				return err
			}
			o := verify.Verify(root, l.b, l.fx, allow, verify.Options{})
			printOutcome(out, o)
			recordVerify(root, o)
			var ee *runner.EffectError
			if err := runner.CheckEffect(l.b, allow); errors.As(err, &ee) {
				fmt.Fprintf(out, "%s needs a grant in .blocks/config.toml, committed: %s\n", name, ee.Line)
			}
			files, err := syncRepo(root, false, false)
			if err != nil {
				return err
			}
			printChanged(out, files)
			if o.Status == verify.Fail {
				return errFailed
			}
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing block and its fixtures")
	return c
}

func retireCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retire <name>",
		Short: "Remove a block, its fixtures and lock entry, then sync.",
		Args:  args(cobra.ExactArgs(1)),
		RunE: func(c *cobra.Command, a []string) error {
			root, err := findRoot()
			if err != nil {
				return err
			}
			if err := promote.Retire(root, a[0]); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "removed: .blocks/%s.py\n", a[0])
			files, err := syncRepo(root, false, false)
			if err != nil {
				return err
			}
			printChanged(c.OutOrStdout(), files)
			return nil
		},
	}
}
