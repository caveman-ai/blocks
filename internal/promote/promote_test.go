package promote

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/caveman-ai/blocks/internal/blockfile"
	"github.com/caveman-ai/blocks/internal/capture"
	"github.com/caveman-ai/blocks/internal/registry"
)

var update = flag.Bool("update", false, "rewrite golden files")

var day = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func init() { now = func() time.Time { return day } }

func shape() capture.Shape {
	return capture.Shape{
		FP: "a1b2c3d4e5f6", Count: 4, Sessions: 3, Last: day.Add(-time.Hour),
		Latest: capture.Sighting{
			CommandHead: "python3 - <<'PY'",
			Script: `
import json
from collections import Counter
rows = [json.loads(l) for l in open("runs/b.jsonl")]
c = Counter(r["status"] for r in rows)
print(c.most_common(5))
`,
		},
		Literals: [][]string{
			{"runs/a.jsonl", "status", "5"},
			{"runs/b.jsonl", "status", "5"},
			{"runs/b.jsonl", "status", "10"},
			{"runs/c.jsonl", "status"},
		},
	}
}

func TestBriefGolden(t *testing.T) {
	got := Brief(shape(), "")
	golden := filepath.Join("testdata", "brief.golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(golden, []byte(got), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("brief differs from %s (go test -update):\n%s", golden, got)
	}
}

func TestRulesMatchFormatDoc(t *testing.T) {
	doc, err := os.ReadFile("../../docs/FORMAT.md")
	if err != nil {
		t.Skip("docs/FORMAT.md not found:", err)
	}
	_, after, _ := strings.Cut(string(doc), "## The eight rules")
	_, after, _ = strings.Cut(after, "```\n")
	block, _, _ := strings.Cut(after, "\n```")
	if block != Rules {
		t.Fatalf("Rules drifted from docs/FORMAT.md:\n%s", block)
	}
}

func TestStepsMatchPromotionDoc(t *testing.T) {
	doc, err := os.ReadFile("../../docs/AGENT-PROMOTION.md")
	if err != nil {
		t.Skip("docs/AGENT-PROMOTION.md not found:", err)
	}
	if !strings.Contains(string(doc), "```\n"+Steps+"\n```") {
		t.Fatal("Steps drifted from docs/AGENT-PROMOTION.md")
	}
}

func TestParams(t *testing.T) {
	got := params(shape().Literals)
	want := []param{
		{pos: 0, name: "path", typ: "path", values: []string{"runs/a.jsonl", "runs/b.jsonl", "runs/c.jsonl"}},
		{pos: 2, name: "n", typ: "int", values: []string{"5", "10"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("params = %+v", got)
	}
	if got := params([][]string{{"a", "x"}, {"b", "y"}}); got[0].name != "value" || got[1].name != "value2" {
		t.Fatalf("dup names = %+v", got)
	}
	if got := guessType([]string{"1.5", "2"}); got != "float" {
		t.Fatalf("guessType = %s", got)
	}
	if params(nil) != nil {
		t.Fatal("params(nil) not nil")
	}
}

func TestProposeName(t *testing.T) {
	cases := map[string]string{
		"import json\nprint(json.load(open('x')))":                   "read-json",
		"import sys\nimport yaml\nyaml.safe_load(open(sys.argv[1]))": "read-yaml",
		"import subprocess\nsubprocess.run(['go','test'])":           "block-a1b2c3",
		"print(1)": "block-a1b2c3",
	}
	for body, want := range cases {
		sh := capture.Shape{FP: "a1b2c3d4e5f6", Latest: capture.Sighting{Script: body}}
		if got := ProposeName(sh); got != want {
			t.Errorf("ProposeName(%q) = %s, want %s", body, got, want)
		}
	}
}

func TestList(t *testing.T) {
	shapes := []capture.Shape{
		{FP: "old", Count: 9, Sessions: 9, Last: day.Add(-30 * 24 * time.Hour)},
		{FP: "few", Count: 9, Sessions: 1, Last: day},
		{FP: "many", Count: 2, Sessions: 2, Last: day, Latest: capture.Sighting{Script: "\n\n  import json  \nx"}},
		{FP: "more", Count: 3, Sessions: 2, Last: day, Latest: capture.Sighting{Script: strings.Repeat("y", 100)}},
		{FP: "done", Count: 50, Sessions: 50, Last: day},
	}
	got := List(shapes, map[string]bool{"done": true}, 14*24*time.Hour)
	var fps []string
	for _, c := range got {
		fps = append(fps, c.FP)
	}
	if !reflect.DeepEqual(fps, []string{"more", "many", "few"}) {
		t.Fatalf("order = %v", fps)
	}
	if got[1].Preview != "import json" || len([]rune(got[0].Preview)) != 80 {
		t.Fatalf("previews = %q %q", got[1].Preview, got[0].Preview)
	}
	if len(List(shapes, nil, 0)) != 5 {
		t.Fatal("since 0 should keep everything")
	}
}

func TestCovered(t *testing.T) {
	bl := func(src string) *blockfile.Block {
		return &blockfile.Block{Header: blockfile.Header{Provenance: &blockfile.Provenance{Source: src}}}
	}
	got := Covered([]*blockfile.Block{bl("candidate:abc"), bl("registry:json-peek@0.1.0"), {}, nil, bl("candidate:")})
	if !reflect.DeepEqual(got, map[string]bool{"abc": true}) {
		t.Fatalf("Covered = %v", got)
	}
}

func TestRetire(t *testing.T) {
	root := t.TempDir()
	blocks := filepath.Join(root, ".blocks")
	os.MkdirAll(filepath.Join(blocks, "fixtures", "json-peek"), 0o755)
	os.WriteFile(filepath.Join(blocks, "json-peek.py"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(blocks, "fixtures", "json-peek", "a.json"), []byte("{}"), 0o644)
	registry.WriteLock(root, map[string]registry.LockEntry{
		"json-peek": {Source: "registry:json-peek@0.1.0", Version: "0.1.0", Hash: "h"},
		"wait-for":  {Source: "registry:wait-for@0.1.0", Version: "0.1.0", Hash: "h"},
	})
	if err := Retire(root, "json-peek"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(blocks, "fixtures", "json-peek")); !os.IsNotExist(err) {
		t.Fatal("fixtures kept")
	}
	lock, _ := registry.Lock(root)
	if _, ok := lock["json-peek"]; ok || len(lock) != 1 {
		t.Fatalf("lock = %v", lock)
	}
	if err := Retire(root, "json-peek"); err == nil {
		t.Fatal("retiring a missing block succeeded")
	}
	if err := Retire(root, "../x"); err == nil {
		t.Fatal("invalid name accepted")
	}
	// No lock file: Retire must not create one.
	root2 := t.TempDir()
	os.MkdirAll(filepath.Join(root2, ".blocks"), 0o755)
	os.WriteFile(filepath.Join(root2, ".blocks", "mine.py"), []byte("x"), 0o644)
	if err := Retire(root2, "mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root2, ".blocks", "blocks.lock")); !os.IsNotExist(err) {
		t.Fatal("Retire created blocks.lock")
	}
}

// TestRetireRefusesSymlinkedFixtures: .blocks/fixtures -> ~ must not let retire x delete ~/x.
func TestRetireRefusesSymlinkedFixtures(t *testing.T) {
	outside := t.TempDir()
	victim := filepath.Join(outside, "x", "keep")
	if err := os.MkdirAll(filepath.Dir(victim), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".blocks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".blocks", "x.py"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".blocks", "fixtures")); err != nil {
		t.Fatal(err)
	}
	if err := Retire(root, "x"); err == nil {
		t.Error("symlinked .blocks/fixtures: no error")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("outside file removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".blocks", "x.py")); err != nil {
		t.Errorf("block removed although retire failed: %v", err)
	}
}

// TestRetireRetry: a retire that removed the .py but not the rest finishes on a second run.
func TestRetireRetry(t *testing.T) {
	root := t.TempDir()
	blocks := filepath.Join(root, ".blocks")
	os.MkdirAll(filepath.Join(blocks, "fixtures", "x"), 0o755)
	registry.WriteLock(root, map[string]registry.LockEntry{"x": {Source: "registry:x@0.1.0", Version: "0.1.0", Hash: "h"}})
	if err := Retire(root, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(blocks, "fixtures", "x")); !os.IsNotExist(err) {
		t.Error("fixtures kept")
	}
	if lock, _ := registry.Lock(root); len(lock) != 0 {
		t.Errorf("lock = %v", lock)
	}
}

// TestBriefDatesUTC: the brief's dates are UTC days whatever the local zone.
func TestBriefDatesUTC(t *testing.T) {
	east := time.FixedZone("+14", 14*3600)
	defer func() { now = func() time.Time { return day } }()
	now = func() time.Time { return time.Date(2026, 10, 4, 1, 0, 0, 0, east) } // 2026-10-03 in UTC
	sh := shape()
	sh.Last = now()
	got := Brief(sh, "")
	if !strings.Contains(got, `created = "2026-10-03"`) || strings.Contains(got, "2026-10-04") {
		t.Errorf("brief dates are not UTC:\n%s", got)
	}
}
