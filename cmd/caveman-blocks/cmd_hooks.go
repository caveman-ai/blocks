package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/caveman-ai/blocks/internal/hook"
	"github.com/caveman-ai/blocks/internal/hook/install"
	"github.com/caveman-ai/blocks/internal/repo"
	"github.com/spf13/cobra"
)

func hooksCmd() *cobra.Command {
	var names []string
	c := &cobra.Command{
		Use:   "hooks",
		Short: "Install, remove or show the per-machine hook for each detected harness.",
		Args:  args(cobra.NoArgs),
	}
	c.PersistentFlags().StringSliceVar(&names, "harness", nil, "claude-code, codex or cursor; repeatable (default: every detected one)")

	c.AddCommand(&cobra.Command{
		Use:   "install [--harness x]...",
		Short: "Copy this binary to a stable path and add the hook to each harness's user config.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			profiles, err := harnesses(names)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if len(profiles) == 0 {
				fmt.Fprintln(out, "No harness detected (~/.claude, ~/.codex, ~/.cursor). Name one with --harness.")
				return nil
			}
			exe, err := executable()
			if err != nil {
				return err
			}
			bin, changed, err := repo.InstallBinary(exe, version)
			if err != nil {
				return fmt.Errorf("copy binary: %w", err)
			}
			fmt.Fprintf(out, "binary: %s (%s)\n", bin, either(changed, "copied", "unchanged"))
			for _, p := range profiles {
				f, err := install.For(p.ConfigFormat)
				if err != nil {
					return err
				}
				changed, err := f.Install(expandHome(p.Config), bin)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "%s: %s in %s\n", p.Name, either(changed, "installed", "unchanged"), p.Config)
				if p.TrustNote != "" {
					fmt.Fprintf(out, "  %s\n", p.TrustNote)
				}
			}
			return nil
		},
	}, &cobra.Command{
		Use:   "uninstall [--harness x]...",
		Short: "Remove exactly the hook entries install wrote.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			profiles, err := harnesses(names)
			if err != nil {
				return err
			}
			for _, p := range profiles {
				f, err := install.For(p.ConfigFormat)
				if err != nil {
					return err
				}
				changed, err := f.Uninstall(expandHome(p.Config))
				if err != nil {
					return err
				}
				fmt.Fprintf(c.OutOrStdout(), "%s: %s\n", p.Name, either(changed, "removed from "+p.Config, "not installed"))
			}
			return nil
		},
	}, &cobra.Command{
		Use:   "status [--harness x]...",
		Short: "Show which harnesses have the hook and which binary it runs.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			profiles, err := harnesses(names)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "No harness detected (~/.claude, ~/.codex, ~/.cursor).")
			}
			for _, p := range profiles {
				hookStatus(c.OutOrStdout(), p)
			}
			return nil
		},
	})
	return c
}

// hookStatus prints one harness's install state, the binary it runs and its trust note.
// It returns false when something needs attention.
func hookStatus(out io.Writer, p hook.Profile) bool {
	f, err := install.For(p.ConfigFormat)
	if err != nil {
		fmt.Fprintf(out, "%s: %v\n", p.Name, err)
		return false
	}
	installed, bin, err := f.Status(expandHome(p.Config))
	switch {
	case err != nil:
		fmt.Fprintf(out, "%s: cannot read %s: %s\n", p.Name, p.Config, oneLine(err.Error()))
		return false
	case !installed:
		fmt.Fprintf(out, "%s: not installed (run caveman-blocks hooks install)\n", p.Name)
		return false
	}
	ok := true
	state := "installed"
	if _, err := os.Stat(bin); err != nil {
		state, ok = "installed, but the binary is missing", false
	}
	fmt.Fprintf(out, "%s: %s, runs %s\n", p.Name, state, bin)
	if p.TrustNote != "" {
		fmt.Fprintf(out, "  %s\n", p.TrustNote)
	}
	return ok
}

// harnesses returns the phase-1 profiles named, or every detected one when none is named.
func harnesses(names []string) ([]hook.Profile, error) {
	all, err := hook.Profiles()
	if err != nil {
		return nil, err
	}
	var out, phase1 []hook.Profile
	for _, p := range all {
		if p.Phase != 1 {
			continue
		}
		phase1 = append(phase1, p)
		if len(names) > 0 {
			if slices.Contains(names, p.Name) {
				out = append(out, p)
			}
		} else if detected(p) {
			out = append(out, p)
		}
	}
	if len(out) < len(names) {
		var known []string
		for _, p := range phase1 {
			known = append(known, p.Name)
		}
		return nil, usageError{fmt.Errorf("unknown --harness in %s: want %s", strings.Join(names, ","), strings.Join(known, ", "))}
	}
	return out, nil
}

func detected(p hook.Profile) bool {
	for _, d := range p.Detect {
		if _, err := os.Stat(expandHome(d)); err == nil {
			return true
		}
	}
	return false
}
