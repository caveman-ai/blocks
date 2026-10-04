// Package stats appends counted events to <state-dir>/stats.jsonl and summarizes them
// (docs/ARCHITECTURE.md#formats-on-disk). Every number it reports is counted, never inferred.
package stats

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/caveman-ai/blocks/internal/repo"
)

// Event kinds.
const (
	KindScript      = "script"
	KindHint        = "hint"
	KindPromoteHint = "promote-hint"
	KindCall        = "call"
	KindRun         = "run"
	KindVerify      = "verify"
)

// Event is one line of stats.jsonl.
type Event struct {
	TS            time.Time `json:"ts"`
	Session       string    `json:"session"`
	Kind          string    `json:"kind"`
	Block         string    `json:"block,omitempty"`
	FP            string    `json:"fp,omitempty"`
	OK            *bool     `json:"ok,omitempty"`
	BytesFull     int       `json:"bytes_full,omitempty"`
	BytesReturned int       `json:"bytes_returned,omitempty"`
	Exit          *int      `json:"exit,omitempty"`
	Lines         int       `json:"lines,omitempty"` // script events: body length
}

// Append writes e as one line to <stateDir>/stats.jsonl, created 0600, never through a symlink.
// A zero TS is set to now.
func Append(stateDir string, e Event) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(stateDir, "stats.jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE|repo.NoFollow, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Summary is the counted view of stats.jsonl over a window.
type Summary struct {
	Since         time.Duration // 0 means all events
	Sessions      int           // distinct non-empty session ids
	Scripts       int           // script events: inline scripts captured
	Hints         int           // hint events shown
	HintsFollowed int           // hints followed by a call of the hinted block in the same session
	Calls         int
	Runs          int
	VerifyPass    int
	VerifyFail    int
	BytesWithheld int // sum of bytes_full - bytes_returned over runs
	// Idle lists blocks the caller says came from one source session that have no run in the window.
	Idle []string
}

// Summarize reads <stateDir>/stats.jsonl and counts events newer than since (all when since <= 0).
// singleSession names blocks whose provenance shows one source session; those without a run in
// the window land in Summary.Idle. Malformed lines are skipped. A missing file is an empty summary.
func Summarize(stateDir string, since time.Duration, singleSession []string) (Summary, error) {
	s := Summary{Since: since}
	f, err := os.Open(filepath.Join(stateDir, "stats.jsonl"))
	if errors.Is(err, fs.ErrNotExist) {
		s.Idle = append([]string(nil), singleSession...)
		sort.Strings(s.Idle)
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()

	var events []Event
	cutoff := time.Now().Add(-since)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) != nil || since > 0 && e.TS.Before(cutoff) {
			continue
		}
		events = append(events, e)
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })

	type key struct{ session, block string }
	pending := map[key]bool{} // an unanswered hint per (session, block)
	sessions := map[string]bool{}
	ran := map[string]bool{}
	for _, e := range events {
		if e.Session != "" {
			sessions[e.Session] = true
		}
		k := key{e.Session, e.Block}
		switch e.Kind {
		case KindScript:
			s.Scripts++
		case KindHint:
			s.Hints++
			if e.Session != "" && e.Block != "" {
				pending[k] = true // a repeated hint replaces the earlier one, which stays unfollowed
			}
		case KindCall:
			s.Calls++
			if pending[k] {
				s.HintsFollowed++
				delete(pending, k)
			}
		case KindRun:
			s.Runs++
			ran[e.Block] = true
			if d := e.BytesFull - e.BytesReturned; d > 0 {
				s.BytesWithheld += d
			}
		case KindVerify:
			if e.OK != nil && *e.OK {
				s.VerifyPass++
			} else {
				s.VerifyFail++
			}
		}
	}
	s.Sessions = len(sessions)
	for _, b := range singleSession {
		if !ran[b] {
			s.Idle = append(s.Idle, b)
		}
	}
	sort.Strings(s.Idle)
	return s, nil
}

// Format renders s as a plain-text table. Every number is labelled measured.
func Format(s Summary) string {
	window := "all time"
	if s.Since > 0 {
		window = "last " + s.Since.String()
		if s.Since%(24*time.Hour) == 0 {
			window = fmt.Sprintf("last %d days", s.Since/(24*time.Hour))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "caveman-blocks stats, %s\n", window)
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, row := range []struct {
		label string
		n     int
	}{
		{"sessions seen", s.Sessions},
		{"scripts captured", s.Scripts},
		{"hints shown", s.Hints},
		{"hints followed", s.HintsFollowed},
		{"block calls", s.Calls},
		{"block runs", s.Runs},
		{"verify passed", s.VerifyPass},
		{"verify failed", s.VerifyFail},
		{"output bytes withheld", s.BytesWithheld},
		{"one-session blocks never run", len(s.Idle)},
	} {
		fmt.Fprintf(w, "%s\t%d\tmeasured\t\n", row.label, row.n)
	}
	w.Flush()
	if len(s.Idle) > 0 {
		fmt.Fprintf(&b, "never run: %s\n", strings.Join(s.Idle, ", "))
	}
	return b.String()
}
