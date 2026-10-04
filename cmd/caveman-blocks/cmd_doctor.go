package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
	"github.com/JuliusBrussee/caveman-blocks/internal/runner"
	"github.com/spf13/cobra"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report on the binary, hooks, trust notes, Python, the repo, CI and sync state.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			out := c.OutOrStdout()
			report := func(good bool, format string, a ...any) {
				mark := "ok  "
				if !good {
					mark = "warn"
				}
				fmt.Fprintf(out, "%s %s\n", mark, fmt.Sprintf(format, a...))
			}

			exe, _ := os.Executable()
			report(true, "binary %s, version %s", exe, version)
			if p, changed := refreshBinary(); changed {
				report(true, "refreshed the hook's copy at %s to %s", p, version)
			}
			copyPath := filepath.Join(repo.BinaryHome(), "caveman-blocks")
			if v := binaryVersion(copyPath); v == "" {
				report(false, "no hook binary at %s (run caveman-blocks hooks install)", copyPath)
			} else {
				report(v == version, "hook binary %s, version %s", copyPath, v)
			}

			profiles, err := harnesses(nil)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				report(false, "no harness detected (~/.claude, ~/.codex, ~/.cursor)")
			}
			for _, p := range profiles {
				var b strings.Builder
				good := hookStatus(&b, p)
				report(good, "%s", strings.TrimSpace(strings.ReplaceAll(b.String(), "\n  ", "; ")))
			}

			if py, err := runner.Python(); err != nil {
				report(false, "no python3 or python on PATH; blocks cannot run")
			} else {
				v := pythonVersion(py)
				report(v != "" && !oldPython(v), "python %s (%s); blocks need 3.10 or later", v, py)
			}

			wd, _ := os.Getwd()
			root, found := repo.FindRoot(wd)
			if !found {
				report(false, "no .blocks/ here or above %s (run caveman-blocks init at the repo root)", wd)
				return nil
			}
			report(true, "repo root %s", root)
			parentClaude(root, report)
			wf, _ := filepath.Glob(filepath.Join(root, ".github", "workflows", "*blocks*"))
			report(len(wf) > 0, "CI workflow for blocks: %s", either(len(wf) > 0, strings.Join(wf, ", "), "none (see docs/CI.md)"))
			if stale, err := syncRepo(root, true, false); err != nil {
				report(false, "sync: %s", oneLine(err.Error()))
			} else {
				report(len(stale) == 0, "generated files: %s", either(len(stale) == 0, "up to date", "stale, run caveman-blocks sync: "+strings.Join(stale, ", ")))
			}
			all, errs := loadBlocks(root)
			indexed := 0
			for _, l := range all {
				if l.indexed {
					indexed++
				}
			}
			report(indexed == len(all) && len(errs) == 0, "blocks: %d indexed, %d not indexed, %d unreadable", indexed, len(all)-indexed, len(errs))
			return nil
		},
	}
}

// parentClaude warns when a parent directory has CLAUDE.md and the root does not: Claude Code then
// reads neither the parent's AGENTS.md fallback nor ours (docs/HOOK.md, instruction files).
func parentClaude(root string, report func(bool, string, ...any)) {
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err == nil {
		return
	}
	for dir := filepath.Dir(root); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
			report(false, "%s exists but %s/CLAUDE.md does not, so Claude Code skips AGENTS.md; add a root CLAUDE.md containing @AGENTS.md",
				filepath.Join(dir, "CLAUDE.md"), root)
			return
		}
		if filepath.Dir(dir) == dir {
			return
		}
	}
}

func either(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

// binaryVersion runs `<bin> version` and returns its last word, or "" when it cannot run.
func binaryVersion(bin string) string {
	return lastWord(bin, "version")
}

func pythonVersion(py string) string { return lastWord(py, "--version") }

func lastWord(bin, arg string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, arg)
	cmd.Stderr = io.Discard
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

// oldPython reports a 3.x version below 3.10, or a major version below 3.
func oldPython(v string) bool {
	var major, minor int
	if _, err := fmt.Sscanf(v, "%d.%d", &major, &minor); err != nil {
		return true
	}
	return major < 3 || major == 3 && minor < 10
}
