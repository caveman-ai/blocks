package export

import (
	"flag"
	"os"
	"testing"

	"github.com/caveman-ai/blocks/blocks"
	"github.com/caveman-ai/blocks/internal/blockfile"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestSkillGolden(t *testing.T) {
	for _, name := range []string{"json-peek", "http-json"} {
		src, err := blocks.FS.ReadFile(name + ".py")
		if err != nil {
			t.Fatal(err)
		}
		b, err := blockfile.Parse(name+".py", src)
		if err != nil {
			t.Fatal(err)
		}
		got := Skill(b)
		golden := "testdata/" + name + ".SKILL.md"
		if *update {
			if err := os.WriteFile(golden, got, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s: got\n%s\nwant\n%s", name, got, want)
		}
	}
}
