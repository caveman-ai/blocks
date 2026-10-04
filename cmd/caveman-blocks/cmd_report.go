package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/caveman-ai/blocks/internal/blockfile"
	"github.com/caveman-ai/blocks/internal/capture"
	"github.com/caveman-ai/blocks/internal/promote"
	"github.com/caveman-ai/blocks/internal/repo"
	"github.com/caveman-ai/blocks/internal/scan"
	"github.com/caveman-ai/blocks/internal/stats"
	"github.com/spf13/cobra"
)

// promoteWindow matches hook rule 9.
const promoteWindow = 14 * 24 * time.Hour

func scanCmd() *cobra.Command {
	var since string
	var harnesses []string
	c := &cobra.Command{
		Use:   "scan [--since 30d] [--harness x]",
		Short: "Group the inline scripts in local agent transcripts by shape; works before init.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			window, err := parseSince(since)
			if err != nil {
				return err
			}
			var readers []scan.Reader
			for _, r := range scan.Readers() {
				if len(harnesses) == 0 || slices.Contains(harnesses, r.Name()) {
					readers = append(readers, r)
				}
			}
			if len(readers) == 0 {
				return usageError{fmt.Errorf("unknown --harness %s: want claude, codex or cursor", strings.Join(harnesses, ","))}
			}
			infos, err := scanBlocks()
			if err != nil {
				return err
			}
			rep, err := scan.Scan(readers, window, capture.Extract, infos)
			if err != nil {
				return err
			}
			fmt.Fprint(c.OutOrStdout(), scan.Format(rep))
			return nil
		},
	}
	c.Flags().StringVar(&since, "since", "30d", "only transcripts and commands newer than this")
	c.Flags().StringSliceVar(&harnesses, "harness", nil, "claude, codex or cursor; repeatable (default all)")
	return c
}

// scanBlocks is the repo's indexed blocks plus first-party blocks not yet in .blocks/, which are
// named "<name> (add)".
func scanBlocks() ([]scan.MatchInfo, error) {
	present := map[string]bool{}
	var infos []scan.MatchInfo
	wd, _ := os.Getwd()
	if root, ok := repo.FindRoot(wd); ok {
		all, _ := loadBlocks(root)
		for _, l := range all {
			present[l.b.Header.Name] = true
			if l.indexed {
				infos = append(infos, scan.MatchInfo{Name: l.b.Header.Name, Matches: compile(l.b)})
			}
		}
	}
	fp, err := firstParty()
	if err != nil {
		return nil, err
	}
	for _, b := range fp {
		if !present[b.Header.Name] {
			infos = append(infos, scan.MatchInfo{Name: b.Header.Name + " (add)", Matches: compile(b)})
		}
	}
	return infos, nil
}

// compile returns b's matches patterns; ones that do not compile are lint's business (F008).
func compile(b *blockfile.Block) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range b.Header.Matches {
		if re, err := regexp.Compile(p); err == nil {
			out = append(out, re)
		}
	}
	return out
}

func promoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "promote [fp]",
		Short: "List repeated script shapes, or print the promotion brief for one.",
		Args:  args(cobra.MaximumNArgs(1)),
		RunE: func(c *cobra.Command, a []string) error {
			root, err := findRoot()
			if err != nil {
				return err
			}
			stateDir, err := repo.StateDir(root)
			if err != nil {
				return err
			}
			shapes, err := capture.Store{Dir: filepath.Join(stateDir, "candidates")}.Shapes(promoteWindow)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if len(a) == 1 {
				for _, sh := range shapes {
					if sh.FP == a[0] {
						fmt.Fprint(out, promote.Brief(sh, ""))
						return nil
					}
				}
				return fmt.Errorf("no sightings of shape %s in the last 14 days", a[0])
			}
			all, _ := loadBlocks(root)
			var bs []*blockfile.Block
			for _, l := range all {
				bs = append(bs, l.b)
			}
			cands := promote.List(shapes, promote.Covered(bs), promoteWindow)
			if len(cands) == 0 {
				fmt.Fprintln(out, "No uncovered script shapes in the last 14 days.")
				return nil
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "FP\tSIGHTINGS\tSESSIONS\tLAST\tPREVIEW")
			for _, cd := range cands {
				fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\n", cd.FP, cd.Count, cd.Sessions, cd.Last.UTC().Format("2006-01-02"), cd.Preview)
			}
			w.Flush()
			fmt.Fprintln(out, "\nNext: caveman-blocks promote <fp> prints the brief for one shape.")
			return nil
		},
	}
}

func statsCmd() *cobra.Command {
	var since string
	c := &cobra.Command{
		Use:   "stats [--since 7d]",
		Short: "Summarize counted events: scripts, hints, calls, runs, verifies.",
		Args:  args(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			window, err := parseSince(since)
			if err != nil {
				return err
			}
			root, err := findRoot()
			if err != nil {
				return err
			}
			stateDir, err := repo.StateDir(root)
			if err != nil {
				return err
			}
			all, _ := loadBlocks(root)
			var single []string
			unverified := 0
			for _, l := range all {
				if p := l.b.Header.Provenance; p != nil && p.Sessions == 1 {
					single = append(single, l.b.Header.Name)
				}
				if !l.indexed {
					unverified++
				}
			}
			s, err := stats.Summarize(stateDir, window, single)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			fmt.Fprint(out, stats.Format(s))
			fmt.Fprintf(out, "blocks: %d indexed, %d not indexed (unverified, stale or quarantined)\n", len(all)-unverified, unverified)
			return nil
		},
	}
	c.Flags().StringVar(&since, "since", "7d", "window to count over")
	return c
}
