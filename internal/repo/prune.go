package repo

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// State dir retention (docs/HOOK.md, "State directory").
const (
	outTTL       = 7 * 24 * time.Hour
	cacheTTL     = time.Hour
	hintedTTL    = 2 * 24 * time.Hour
	candidateTTL = 14 * 24 * time.Hour // hook rule 9's window
	candidateMax = 8 << 20             // newest bytes of a candidates file read when pruning
	pruneEvery   = 24 * time.Hour
)

// Prune removes out/ entries older than 7 days, cache/ entries older than 1 hour, hinted/ flags
// older than 2 days and sightings older than 14 days, at most once per day, tracked by the mtime of
// <stateDir>/.pruned. The marker is touched first, so concurrent callers (the runner and the hook)
// do not prune twice and a prune cut short waits for the next day. Best effort: errors are ignored.
func Prune(stateDir string, now time.Time) {
	if !PruneDue(stateDir, now) {
		return
	}
	marker := filepath.Join(stateDir, ".pruned")
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|NoFollow, 0o600)
	if err != nil {
		return
	}
	f.Close()
	os.Chtimes(marker, now, now)
	pruneCandidates(filepath.Join(stateDir, "candidates"), now)
	for sub, ttl := range map[string]time.Duration{"out": outTTL, "cache": cacheTTL, "hinted": hintedTTL} {
		dir := filepath.Join(stateDir, sub)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if fi, err := e.Info(); err == nil && now.Sub(fi.ModTime()) > ttl {
				os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
	}
}

// PruneDue reports whether a day has passed since the last Prune.
func PruneDue(stateDir string, now time.Time) bool {
	fi, err := os.Lstat(filepath.Join(stateDir, ".pruned"))
	return err != nil || now.Sub(fi.ModTime()) >= pruneEvery
}

// pruneCandidates drops sightings older than candidateTTL from <stateDir>/candidates/*.jsonl.
// Sightings are appended in time order, so only the newest candidateMax bytes of a file are read
// and a bigger file is cut to them. A file is rewritten (temp + rename) only when a line goes.
//
// ponytail: a sighting the hook appends during the rewrite is lost; the hook would need a lock.
func pruneCandidates(dir string, now time.Time) {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, p := range paths {
		pruneSightings(p, now.Add(-candidateTTL))
	}
}

func pruneSightings(p string, cutoff time.Time) error {
	f, err := os.OpenFile(p, os.O_RDONLY|NoFollow, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return err
	}
	cut := fi.Size() > candidateMax
	if cut {
		if _, err := f.Seek(fi.Size()-candidateMax, io.SeekStart); err != nil {
			return err
		}
	}
	var keep bytes.Buffer
	r := bufio.NewReader(f)
	dropped, partial := cut, cut // after a seek the first line is partial
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && !partial {
			var sg struct {
				TS time.Time `json:"ts"`
			}
			if json.Unmarshal(line, &sg) == nil && !sg.TS.Before(cutoff) {
				keep.Write(line)
			} else {
				dropped = true
			}
		}
		partial = false
		if err != nil {
			break
		}
	}
	if !dropped {
		return nil
	}
	if keep.Len() == 0 {
		return os.Remove(p)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".prune-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(keep.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil { // CreateTemp makes it 0600
		return err
	}
	return os.Rename(tmp.Name(), p)
}
