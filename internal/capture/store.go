package capture

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Append writes one sighting with O_APPEND|O_NOFOLLOW, creating the file 0600 and Dir 0700. It fails
// when <fp>.jsonl is a symlink.
func (st Store) Append(fp string, sg Sighting) error {
	if !validFP(fp) {
		return fmt.Errorf("capture: invalid fingerprint %q", fp)
	}
	line, err := json.Marshal(sg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(st.Dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(st.Dir, fp+".jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE|oNoFollow, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Shapes lists shapes with sightings since the window, most sightings first, then by fp. A window of
// zero or less keeps every sighting. Corrupt lines are skipped; a missing Dir is no shapes.
func (st Store) Shapes(since time.Duration) ([]Shape, error) {
	ents, err := os.ReadDir(st.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cutoff time.Time
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}
	var out []Shape
	for _, e := range ents {
		fp, ok := strings.CutSuffix(e.Name(), ".jsonl")
		if !ok || !e.Type().IsRegular() || !validFP(fp) {
			continue
		}
		sh, err := readShape(filepath.Join(st.Dir, e.Name()), fp, cutoff)
		if err != nil {
			return nil, err
		}
		if sh.Count > 0 {
			out = append(out, sh)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].FP < out[j].FP
	})
	return out, nil
}

func readShape(path, fp string, cutoff time.Time) (Shape, error) {
	sh := Shape{FP: fp}
	f, err := os.OpenFile(path, os.O_RDONLY|oNoFollow, 0)
	if err != nil {
		return sh, err
	}
	defer f.Close()
	sessions := map[string]bool{}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var sg Sighting
			if json.Unmarshal(line, &sg) == nil && !sg.TS.Before(cutoff) {
				sh.Count++
				if sg.Session != "" {
					sessions[sg.Session] = true
				}
				if sh.Count == 1 || sg.TS.After(sh.Last) {
					sh.Last, sh.Latest = sg.TS, sg
				}
				sh.Literals = append(sh.Literals, sg.Literals)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return sh, err
		}
	}
	sh.Sessions = len(sessions)
	return sh, nil
}

// validFP accepts lowercase hex, so a fingerprint can never name a path outside Dir.
func validFP(fp string) bool {
	if fp == "" || len(fp) > 64 {
		return false
	}
	return strings.Trim(fp, "0123456789abcdef") == ""
}
