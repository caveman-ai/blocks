package index

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

func entries(n int) []Entry {
	var out []Entry
	for i := n - 1; i >= 0; i-- { // reverse order: Body must sort
		e := Entry{
			Name:    fmt.Sprintf("block-%02d", i),
			Params:  "--path <path> [--depth 2]",
			Summary: "Shape of a JSON or JSONL file: keys, row count, one sample.",
		}
		switch i {
		case 1:
			e.Params = ""
			e.Summary = "Counts lines."
		case 2:
			e.Summary = "A summary that is long enough to run past the one hundred and ten column cut, so it ends early."
		case 3:
			e.Name = "a-block-name-longer-than-fourteen"
			e.Params = "--pattern <str> --path <path> [--ignore-case] [--limit 20]"
		}
		out = append(out, e)
	}
	return out
}

func TestBodyGolden(t *testing.T) {
	for _, n := range []int{0, 1, 20} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			got, err := Body(entries(n), DefaultMax, DefaultBudget)
			if err != nil {
				t.Fatal(err)
			}
			path := fmt.Sprintf("testdata/body-%d.golden", n)
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test -update)", err)
			}
			if got != string(want) {
				t.Errorf("body mismatch\n--- got\n%s\n--- want\n%s", got, want)
			}
			for _, l := range strings.Split(got, "\n") {
				if utf8.RuneCountInString(l) > LineWidth {
					t.Errorf("line over %d runes: %q", LineWidth, l)
				}
			}
		})
	}
}

func TestBodyLimits(t *testing.T) {
	if _, err := Body(entries(21), DefaultMax, DefaultBudget); err == nil || !strings.Contains(err.Error(), "retire 1") {
		t.Errorf("over max: got %v", err)
	}
	if _, err := Body(entries(5), DefaultMax, len(Rules)+10); err == nil || !strings.Contains(err.Error(), "shorten summaries or retire blocks") {
		t.Errorf("over budget: got %v", err)
	}
}

func TestUpsert(t *testing.T) {
	const body = "line one\nline two"
	section := MarkerStart + "\nline one\nline two\n" + MarkerEnd + "\n"
	tests := []struct {
		name, file, want string
	}{
		{name: "empty file", file: "", want: section},
		{name: "no section", file: "# Repo\n\nText.\n", want: "# Repo\n\nText.\n\n" + section},
		{name: "no trailing newline", file: "# Repo", want: "# Repo\n\n" + section},
		{
			name: "existing section",
			file: "# Repo\n" + MarkerStart + "\nold\n" + MarkerEnd + "\nAfter.\n",
			want: "# Repo\n" + MarkerStart + "\nline one\nline two\n" + MarkerEnd + "\nAfter.\n",
		},
		{
			name: "crlf file",
			file: "# Repo\r\n" + MarkerStart + "\r\nold\r\n" + MarkerEnd + "\r\nAfter.\r\n",
			want: "# Repo\r\n" + MarkerStart + "\r\nline one\r\nline two\r\n" + MarkerEnd + "\r\nAfter.\r\n",
		},
		{name: "crlf without section", file: "# Repo\r\n", want: "# Repo\r\n\r\n" + strings.ReplaceAll(section, "\n", "\r\n")},
		{
			name: "stray start marker is left alone",
			file: MarkerStart + " stray\n" + MarkerStart + "\nold\n" + MarkerEnd + "\n",
			want: MarkerStart + " stray\n" + MarkerStart + "\nline one\nline two\n" + MarkerEnd + "\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			once := Upsert([]byte(tc.file), body)
			if string(once) != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", once, tc.want)
			}
			if twice := Upsert(once, body); string(twice) != string(once) {
				t.Fatalf("not idempotent:\n%q\n%q", once, twice)
			}
			if got, ok := Section(once); !ok || got != body {
				t.Fatalf("Section = %q, %v", got, ok)
			}
		})
	}
}

func TestSectionAbsent(t *testing.T) {
	for _, f := range []string{"", "text", MarkerStart + "\nno end\n", MarkerEnd + "\n" + MarkerStart} {
		if _, ok := Section([]byte(f)); ok {
			t.Errorf("Section(%q) found a section", f)
		}
	}
}
