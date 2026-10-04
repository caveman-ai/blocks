// Package scan reads the transcripts agent harnesses keep, applies the hook's script extraction,
// and reports inline scripts grouped by shape with the blocks that would cover each group
// (docs/ARCHITECTURE.md, "Scan"). Every number it prints is counted.
package scan

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/internal/capture"
)

// MatchInfo is a block's name and compiled `matches` patterns.
type MatchInfo struct {
	Name    string
	Matches []*regexp.Regexp
}

// Group is one shape fingerprint's sightings.
type Group struct {
	FP        string
	Count     int
	Sessions  int
	Lines     int    // median script length
	Example   string // latest script of the shape, scrubbed
	Last      time.Time
	CoveredBy []string // block names whose matches hit Example, sorted
}

// Report is the outcome of a scan.
type Report struct {
	Commands  int            // shell commands read
	Scripts   int            // inline Python scripts extracted, edits included
	Edits     int            // scripts that edit a file in place; never grouped
	Unshaped  int            // scripts with no fingerprint; never grouped
	Groups    []Group        // by count, then sessions, then fp
	Files     map[string]int // transcript files read, per harness
	Skipped   map[string]int // unparsable lines and unreadable files, per harness
	Harnesses []string       // reader names, in the order given
}

type acc struct {
	g        Group
	sessions map[string]bool
	lines    []int
}

// Scan reads every file matched by each reader's Default patterns. since > 0 drops files last
// modified, and commands timestamped, before now-since. extract is capture.Extract in production.
func Scan(readers []Reader, since time.Duration, extract func(string) (capture.Script, bool), blocks []MatchInfo) (Report, error) {
	rep := Report{Files: map[string]int{}, Skipped: map[string]int{}}
	cutoff := time.Now().Add(-since)
	old := func(t time.Time) bool { return since > 0 && !t.IsZero() && t.Before(cutoff) }
	groups := map[string]*acc{}

	for _, r := range readers {
		name := r.Name()
		rep.Harnesses = append(rep.Harnesses, name)
		seen := map[string]bool{}
		for _, pat := range r.Default() {
			files, err := expand(pat)
			if err != nil {
				return rep, fmt.Errorf("%s: pattern %s: %w", name, pat, err)
			}
			for _, f := range files {
				if seen[f] {
					continue
				}
				seen[f] = true
				if fi, err := os.Stat(f); err != nil || old(fi.ModTime()) {
					continue
				}
				res, err := r.Commands(f)
				rep.Files[name]++
				rep.Skipped[name] += res.Skipped
				if err != nil {
					rep.Skipped[name]++
				}
				for _, c := range res.Cmds {
					if old(c.TS) {
						continue
					}
					rep.Commands++
					if _, ok := capture.IsBlockCall(c.Command); ok { // hook rule 3: not a script
						continue
					}
					s, ok := extract(c.Command)
					if !ok {
						continue
					}
					rep.Scripts++
					if s.Edit {
						rep.Edits++
						continue
					}
					if s.FP == "" {
						rep.Unshaped++
						continue
					}
					a := groups[s.FP]
					if a == nil {
						a = &acc{g: Group{FP: s.FP}, sessions: map[string]bool{}}
						groups[s.FP] = a
					}
					a.g.Count++
					a.sessions[c.Session] = true
					a.lines = append(a.lines, s.Lines)
					if !c.TS.Before(a.g.Last) { // latest wins; untimed commands keep reading order
						a.g.Last, a.g.Example = c.TS, s.Body
					}
				}
			}
		}
	}

	for _, a := range groups {
		a.g.Sessions = len(a.sessions)
		sort.Ints(a.lines)
		a.g.Lines = a.lines[len(a.lines)/2]
		for _, b := range blocks {
			for _, re := range b.Matches {
				if re.MatchString(a.g.Example) {
					a.g.CoveredBy = append(a.g.CoveredBy, b.Name)
					break
				}
			}
		}
		a.g.Example = capture.Scrub(a.g.Example) // matched raw, like the hook; shown scrubbed
		sort.Strings(a.g.CoveredBy)
		rep.Groups = append(rep.Groups, a.g)
	}
	sort.Slice(rep.Groups, func(i, j int) bool {
		a, b := rep.Groups[i], rep.Groups[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Sessions != b.Sessions {
			return a.Sessions > b.Sessions
		}
		return a.FP < b.FP
	})
	return rep, nil
}

// TopN is how many groups Format prints.
const TopN = 20

// Format renders the repeat table: the top groups, totals and a footer on transcript formats.
func Format(r Report) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("Inline scripts your agents wrote, grouped by shape (measured, from local transcripts)\n\n")
	if len(r.Groups) == 0 {
		w("  No repeated script shapes found.\n")
	}
	for i, g := range r.Groups {
		if i == TopN {
			break
		}
		w("%5dx  %-60s  ~%d lines  %d sessions", g.Count, preview(g.Example, 60), g.Lines, g.Sessions)
		if len(g.CoveredBy) > 0 {
			w("  covered by: %s", strings.Join(g.CoveredBy, ", "))
		}
		w("\n")
	}
	if len(r.Groups) > TopN {
		w("  … %d more shapes\n", len(r.Groups)-TopN)
	}

	files, skipped := 0, 0
	var per []string
	for _, h := range r.Harnesses {
		files += r.Files[h]
		skipped += r.Skipped[h]
		per = append(per, fmt.Sprintf("%s %d files, %d skipped", h, r.Files[h], r.Skipped[h]))
	}
	w("\nMeasured: %d shell commands, %d inline Python scripts, %d shapes. Not grouped: %d edits, %d scripts without a shape.\n",
		r.Commands, r.Scripts, len(r.Groups), r.Edits, r.Unshaped)
	w("Read %d transcript files (%s).\n", files, strings.Join(per, "; "))
	w("Transcript formats are internal to each harness and may change; %d lines or files were skipped.\n", skipped)
	return b.String()
}

// preview joins the script's non-empty lines with "; " and cuts the result at n runes.
func preview(script string, n int) string {
	var parts []string
	for _, l := range strings.Split(script, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	r := []rune(strings.Join(parts, "; "))
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return string(r)
}
