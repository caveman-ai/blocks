package hook_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caveman-ai/blocks/internal/hook/dialect/claude"
	"github.com/caveman-ai/blocks/internal/hook/dialect/cursor"
	"github.com/caveman-ai/blocks/internal/hook/dialect/generic"
	"github.com/caveman-ai/blocks/internal/hook/protocol"
)

const fixtureHint = "Blocks: json-peek covers this. Next time: caveman-blocks run json-peek --path <path>"

type dialect struct {
	parse  func([]byte) (protocol.Request, error)
	render func(protocol.Response, string) ([]byte, error)
}

// TestDialectConformance replays testdata/dialect/<name>/<case>-in.json through Parse, compares the
// request with <case>-req.json, renders an allow with the fixture hint (none for nohint-* cases) and
// compares with <case>-out.json, both after JSON canonicalization.
func TestDialectConformance(t *testing.T) {
	dialects := map[string]dialect{
		"claude":  {claude.Parse, claude.Render},
		"cursor":  {cursor.Parse, cursor.Render},
		"generic": {generic.Parse, generic.Render},
	}
	for name, d := range dialects {
		ins, _ := filepath.Glob(filepath.Join("testdata/dialect", name, "*-in.json"))
		if len(ins) < 2 {
			t.Fatalf("%s: want pre and post fixtures, got %d", name, len(ins))
		}
		for _, in := range ins {
			base := strings.TrimSuffix(in, "-in.json")
			t.Run(name+"/"+filepath.Base(base), func(t *testing.T) {
				req, err := d.parse(read(t, in))
				if err != nil {
					t.Fatal(err)
				}
				got, _ := json.Marshal(req)
				same(t, "request", got, read(t, base+"-req.json"))

				hint := fixtureHint
				if strings.HasPrefix(filepath.Base(base), "nohint-") {
					hint = ""
				}
				out, err := d.render(protocol.Allow(hint), req.Phase)
				if err != nil {
					t.Fatal(err)
				}
				same(t, "response", out, read(t, base+"-out.json"))
			})
		}
	}
}

func TestParseRejects(t *testing.T) {
	for name, b := range map[string]string{
		"generic old protocol":  `{"protocol":0,"phase":"pre"}`,
		"generic bad phase":     `{"protocol":1,"phase":"during"}`,
		"generic not json":      `{`,
		"claude other event":    `{"hook_event_name":"Stop"}`,
		"claude object command": `{"hook_event_name":"PreToolUse","tool_input":{"command":{"x":1}}}`,
		"cursor other event":    `{"hook_event_name":"beforeReadFile"}`,
	} {
		var err error
		switch strings.Fields(name)[0] {
		case "generic":
			_, err = generic.Parse([]byte(b))
		case "claude":
			_, err = claude.Parse([]byte(b))
		case "cursor":
			_, err = cursor.Parse([]byte(b))
		}
		if err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

func read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func same(t *testing.T, what string, got, want []byte) {
	t.Helper()
	if c1, c2 := canon(t, got), canon(t, want); !bytes.Equal(c1, c2) {
		t.Errorf("%s\n got %s\nwant %s", what, c1, c2)
	}
}

func canon(t *testing.T, b []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("not JSON: %s", b)
	}
	out, _ := json.Marshal(v)
	return out
}
