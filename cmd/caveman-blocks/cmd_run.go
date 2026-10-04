package main

import (
	"errors"
	"os"

	"github.com/caveman-ai/blocks/internal/repo"
	"github.com/caveman-ai/blocks/internal/runner"
	"github.com/caveman-ai/blocks/internal/stats"
	"github.com/spf13/cobra"
)

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run <name> [--param value]...",
		Short:              "Run a block from the repo root: effects gate, path confinement, 2 KB cap.",
		DisableFlagParsing: true, // every flag after the name belongs to the block
		RunE: func(c *cobra.Command, a []string) error {
			if len(a) == 0 || a[0] == "-h" || a[0] == "--help" {
				if len(a) == 0 {
					return usageError{errors.New("run needs a block name")}
				}
				return c.Help()
			}
			root, err := findRoot()
			if err != nil {
				return err
			}
			l, err := blockByName(root, a[0])
			if err != nil {
				return err
			}
			allow, err := policy(root, "HEAD")
			if err != nil {
				return err
			}
			stateDir, err := repo.StateDir(root)
			if err != nil {
				return err
			}
			res, err := runner.Run(root, stateDir, l.b, !l.indexed, allow, a[1:], os.Stdin, os.Stderr)
			if err != nil {
				return err
			}
			ok := res.Exit == 0
			exit := res.Exit
			// run events carry no session; "hint followed" comes from the hook's call events.
			stats.Append(stateDir, stats.Event{Kind: stats.KindRun, Block: l.b.Header.Name,
				OK: &ok, BytesFull: res.BytesFull, BytesReturned: res.BytesReturned, Exit: &exit})
			c.OutOrStdout().Write(res.Stdout)
			if res.Exit != 0 {
				return exitError{code: res.Exit}
			}
			return nil
		},
	}
}
