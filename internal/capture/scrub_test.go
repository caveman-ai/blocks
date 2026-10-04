package capture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScrubGolden runs testdata/scrub/*.txt: the input, a "-- want --" line, the expected output.
func TestScrubGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/scrub/*.txt")
	if err != nil || len(files) < 15 {
		t.Fatalf("want at least 15 golden cases, got %d (%v)", len(files), err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			in, want, ok := strings.Cut(string(raw), "\n-- want --\n")
			if !ok {
				t.Fatal("missing -- want -- separator")
			}
			want = strings.TrimSuffix(want, "\n")
			if got := Scrub(in); got != want {
				t.Errorf("Scrub:\n got: %s\nwant: %s", got, want)
			}
			if got := Scrub(want); got != want {
				t.Errorf("Scrub is not idempotent:\n got: %s", got)
			}
		})
	}
}
