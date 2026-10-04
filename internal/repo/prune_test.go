package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrune(t *testing.T) {
	state := t.TempDir()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	files := map[string]time.Duration{
		"out/old.log":   8 * 24 * time.Hour,
		"out/new.log":   6 * 24 * time.Hour,
		"cache/old":     2 * time.Hour,
		"cache/new":     30 * time.Minute,
		"cache/olddir/": 2 * time.Hour,
		"hinted/old":    3 * 24 * time.Hour,
		"hinted/new":    24 * time.Hour,
	}
	for name, age := range files {
		p := filepath.Join(state, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(state, name)); return err == nil }

	Prune(state, now)
	for name, want := range map[string]bool{"out/old.log": false, "out/new.log": true, "cache/old": false, "cache/new": true, "cache/olddir": false, "hinted/old": false, "hinted/new": true} {
		if exists(name) != want {
			t.Errorf("%s exists = %v, want %v", name, !want, want)
		}
	}

	// Within a day of the last prune nothing is removed, even when stale.
	stale := filepath.Join(state, "cache", "stale")
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stale, now.Add(-5*time.Hour), now.Add(-5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	Prune(state, now.Add(23*time.Hour))
	if !exists("cache/stale") {
		t.Error("pruned twice within a day")
	}
	Prune(state, now.Add(25*time.Hour))
	if exists("cache/stale") {
		t.Error("not pruned after a day")
	}
}

func TestPruneCandidates(t *testing.T) {
	state := t.TempDir()
	dir := filepath.Join(state, "candidates")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	line := func(age time.Duration) string {
		return fmt.Sprintf(`{"ts":%q,"session":"s"}`+"\n", now.Add(-age).Format(time.RFC3339Nano))
	}
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mixed := write("aa.jsonl", line(20*24*time.Hour)+"not json\n"+line(time.Hour))
	gone := write("bb.jsonl", line(30*24*time.Hour))
	fresh := write("cc.jsonl", line(time.Hour))
	big := write("dd.jsonl", strings.Repeat(line(time.Minute), candidateMax/len(line(time.Minute))+1000))

	Prune(state, now)
	if b, _ := os.ReadFile(mixed); string(b) != line(time.Hour) {
		t.Errorf("mixed = %q", b)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Errorf("all-old file kept: %v", err)
	}
	if b, _ := os.ReadFile(fresh); string(b) != line(time.Hour) {
		t.Errorf("fresh = %q", b)
	}
	fi, err := os.Stat(big)
	if err != nil || fi.Size() > candidateMax || fi.Size() < candidateMax/2 {
		t.Errorf("big file size %v, %v", fi.Size(), err)
	}
	if b, _ := os.ReadFile(big); !strings.HasPrefix(string(b), `{"ts"`) {
		t.Errorf("big file starts with a partial line: %.40q", b)
	}
}
