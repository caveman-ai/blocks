// Command caveman-blocks is the Caveman Blocks CLI and hook binary (docs/REPOSITORY.md, "Commands").
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

const exitCodes = `
Exit codes: 0 success; 1 failure (lint finding, failed or stale verify, stale sync, refused run);
2 usage error. run passes the block's own exit code through. hook always exits 0.
`

func main() { os.Exit(run()) }

// run executes the CLI and returns the process exit code.
func run() int {
	root := newRoot()
	err := root.Execute()
	var ex exitError
	switch {
	case err == nil:
		return 0
	case len(os.Args) > 1 && os.Args[1] == "hook":
		fmt.Println("{}") // the hook never fails a session, even on a malformed command line
		return 0
	case errors.As(err, &ex):
		if ex.msg != "" {
			fmt.Fprintln(os.Stderr, "caveman-blocks:", ex.msg)
		}
		return ex.code
	case isUsage(err):
		fmt.Fprintln(os.Stderr, "caveman-blocks:", err)
		return 2
	}
	fmt.Fprintln(os.Stderr, "caveman-blocks:", oneLine(err.Error()))
	return 1
}

// exitError ends the command with code, printing msg when it is not empty.
type exitError struct {
	code int
	msg  string
}

func (e exitError) Error() string { return e.msg }

// errFailed is exit 1 with nothing more to print; the command already reported why.
var errFailed = exitError{code: 1}

type usageError struct{ error }

func isUsage(err error) bool {
	var u usageError
	return errors.As(err, &u) || strings.HasPrefix(err.Error(), "unknown command") ||
		strings.HasPrefix(err.Error(), "unknown flag") || strings.HasPrefix(err.Error(), "unknown shorthand flag")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// args wraps a cobra validator so its error is a usage error (exit 2).
func args(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(c *cobra.Command, a []string) error {
		if err := v(c, a); err != nil {
			return usageError{err}
		}
		return nil
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "caveman-blocks",
		Short:         "Turn agent-written scripts into verified, reusable blocks in .blocks/.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.SetUsageTemplate(root.UsageTemplate() + exitCodes)
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(
		initCmd(), hooksCmd(), addCmd(), runCmd(), lintCmd(), verifyCmd(), syncCmd(), scanCmd(),
		promoteCmd(), retireCmd(), exportCmd(), statsCmd(), hookCmd(), doctorCmd(),
		versionCmd(),
	)
	return root
}

// capabilities names what this build supports, for callers that probe `version --json`
// (the caveman CLI reads it before it relies on `hooks status --json`).
var capabilities = []string{"hooks_status_json"}

func versionCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "version [--json]",
		Short: "Print the version.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"version": version, "capabilities": capabilities})
			}
			fmt.Fprintln(c.OutOrStdout(), "caveman-blocks", version)
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print {version, capabilities} as JSON")
	return c
}
