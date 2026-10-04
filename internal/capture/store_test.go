package capture

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "candidates")
	st := Store{Dir: dir}
	now := time.Now().UTC().Truncate(time.Second)
	add := func(fp, session string, age time.Duration, lits ...string) {
		t.Helper()
		if err := st.Append(fp, Sighting{TS: now.Add(-age), Session: session, ScriptSHA: "s", Lines: 12, Literals: lits}); err != nil {
			t.Fatal(err)
		}
	}
	add("aaaaaaaaaaaa", "s1", 3*time.Hour, "a.json")
	add("aaaaaaaaaaaa", "s2", time.Hour, "b.json")
	add("aaaaaaaaaaaa", "s2", 2*time.Hour, "c.json")
	add("bbbbbbbbbbbb", "s1", time.Hour)
	add("cccccccccccc", "s3", 30*24*time.Hour) // outside a 14 day window
	add("dddddddddddd", "", time.Minute)
	add("dddddddddddd", "", time.Minute)
	add("dddddddddddd", "s4", 2*time.Minute)

	// A corrupt line is skipped, not fatal.
	f, err := os.OpenFile(filepath.Join(dir, "bbbbbbbbbbbb.jsonl"), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{not json\n")
	f.Close()

	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, %v; want 0700", fi.Mode().Perm(), err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "aaaaaaaaaaaa.jsonl")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, %v; want 0600", fi.Mode().Perm(), err)
	}

	shapes, err := st.Shapes(14 * 24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range shapes {
		got = append(got, s.FP)
	}
	if want := []string{"aaaaaaaaaaaa", "dddddddddddd", "bbbbbbbbbbbb"}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("order = %v, want %v", got, want)
	}
	a := shapes[0]
	if a.Count != 3 || a.Sessions != 2 || !a.Last.Equal(now.Add(-time.Hour)) || a.Latest.Literals[0] != "b.json" || len(a.Literals) != 3 {
		t.Errorf("shape a = %+v", a)
	}
	if d := shapes[1]; d.Count != 3 || d.Sessions != 1 {
		t.Errorf("shape d = %+v (unknown sessions do not count)", d)
	}
	if b := shapes[2]; b.Count != 1 {
		t.Errorf("shape b = %+v (corrupt line must be skipped)", b)
	}

	all, err := st.Shapes(0)
	if err != nil || len(all) != 4 {
		t.Errorf("Shapes(0) = %d shapes, %v; want all 4", len(all), err)
	}
	if none, err := (Store{Dir: filepath.Join(dir, "missing")}).Shapes(time.Hour); err != nil || none != nil {
		t.Errorf("missing dir = %v, %v", none, err)
	}
}

func TestStoreRefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no O_NOFOLLOW on windows")
	}
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "abcdefabcdef.jsonl")); err != nil {
		t.Fatal(err)
	}
	st := Store{Dir: dir}
	if err := st.Append("abcdefabcdef", Sighting{TS: time.Now()}); err == nil {
		t.Fatal("Append wrote through a symlink")
	}
	if b, _ := os.ReadFile(target); string(b) != "keep\n" {
		t.Errorf("symlink target changed: %q", b)
	}
	if shapes, err := st.Shapes(0); err != nil || len(shapes) != 0 {
		t.Errorf("Shapes read through a symlink: %v, %v", shapes, err)
	}
}

func TestStoreRejectsBadFP(t *testing.T) {
	st := Store{Dir: t.TempDir()}
	for _, fp := range []string{"", "../x", "ABCDEF", "a/b"} {
		if err := st.Append(fp, Sighting{}); err == nil {
			t.Errorf("Append(%q) accepted", fp)
		}
	}
}
