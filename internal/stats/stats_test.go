package stats

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAppendSummarize(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	at := func(min int) time.Time { return now.Add(time.Duration(min) * time.Minute) }
	yes, no := true, false
	exit0 := 0
	events := []Event{
		{TS: at(-60 * 24 * 30), Session: "old", Kind: KindScript}, // outside a 7-day window
		{TS: at(-10), Session: "s1", Kind: KindScript, FP: "abc"},
		{TS: at(-9), Session: "s1", Kind: KindHint, Block: "json-peek"},
		{TS: at(-8), Session: "s1", Kind: KindCall, Block: "json-peek"},    // follows the hint
		{TS: at(-7), Session: "s1", Kind: KindCall, Block: "json-peek"},    // no second credit
		{TS: at(-6), Session: "s2", Kind: KindCall, Block: "first-error"},  // call before its hint
		{TS: at(-5), Session: "s2", Kind: KindHint, Block: "first-error"},  // never followed
		{TS: at(-5), Session: "s3", Kind: KindHint, Block: "json-peek"},    // followed only in another session
		{TS: at(-4), Session: "s1", Kind: KindHint, Block: "test-summary"}, // shown twice, followed once
		{TS: at(-3), Session: "s1", Kind: KindHint, Block: "test-summary"},
		{TS: at(-2), Session: "s1", Kind: KindCall, Block: "test-summary"},
		{TS: at(-2), Kind: KindRun, Block: "json-peek", OK: &yes, BytesFull: 5000, BytesReturned: 300, Exit: &exit0},
		{TS: at(-1), Kind: KindRun, Block: "json-peek", OK: &yes, BytesFull: 100, BytesReturned: 120, Exit: &exit0},
		{TS: at(-1), Kind: KindVerify, Block: "json-peek", OK: &yes},
		{TS: at(-1), Kind: KindVerify, Block: "broken", OK: &no},
	}
	for _, e := range events {
		if err := Append(dir, e); err != nil {
			t.Fatal(err)
		}
	}
	// A malformed line is skipped, not fatal.
	f, _ := os.OpenFile(filepath.Join(dir, "stats.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("{not json\n")
	f.Close()

	got, err := Summarize(dir, 7*24*time.Hour, []string{"json-peek", "zeta", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	want := Summary{
		Since: 7 * 24 * time.Hour, Sessions: 3, Scripts: 1, Hints: 5, HintsFollowed: 2, Calls: 4,
		Runs: 2, VerifyPass: 1, VerifyFail: 1, BytesWithheld: 4700, Idle: []string{"alpha", "zeta"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	all, _ := Summarize(dir, 0, nil)
	if all.Scripts != 2 || all.Sessions != 4 {
		t.Errorf("all-time: %+v", all)
	}

	fi, err := os.Stat(filepath.Join(dir, "stats.jsonl"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, %v", fi.Mode().Perm(), err)
	}

	out := Format(got)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
		if strings.HasPrefix(line, "never run:") {
			continue
		}
		if !strings.HasSuffix(strings.TrimSpace(line), "measured") {
			t.Errorf("unlabelled row %q", line)
		}
	}
	if !strings.Contains(out, "last 7 days") || !strings.Contains(out, "never run: alpha, zeta") {
		t.Errorf("format:\n%s", out)
	}
}

func TestSummarizeMissingFile(t *testing.T) {
	s, err := Summarize(t.TempDir(), time.Hour, []string{"b", "a"})
	if err != nil || s.Runs != 0 || !reflect.DeepEqual(s.Idle, []string{"a", "b"}) {
		t.Errorf("%+v, %v", s, err)
	}
}

func TestAppendRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "victim")
	if err := os.Symlink(target, filepath.Join(dir, "stats.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := Append(dir, Event{Kind: KindRun}); err == nil {
		t.Error("appended through a symlink")
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("symlink target created")
	}
}
