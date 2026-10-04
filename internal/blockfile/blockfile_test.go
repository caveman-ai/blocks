package blockfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden.json")

// golden is the recorded outcome of parsing and linting one testdata file.
type golden struct {
	Error    string    `json:"error,omitempty"`
	Header   *Header   `json:"header,omitempty"`
	Fences   []int     `json:"fences,omitempty"` // FenceStart, FenceEnd, StampLine
	Hash     string    `json:"hash,omitempty"`
	Findings []Finding `json:"findings"`
}

func TestGolden(t *testing.T) {
	inputs, _ := filepath.Glob("testdata/*.py")
	shells, _ := filepath.Glob("testdata/*.sh")
	for _, path := range append(inputs, shells...) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var got golden
			b, err := Load(path)
			if err != nil {
				got.Error = err.Error()
			} else {
				got.Header = &b.Header
				got.Fences = []int{b.FenceStart, b.FenceEnd, b.StampLine}
				got.Hash = ContentHash(b, nil)
				got.Findings = Lint(b, LintOptions{})
			}
			data, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, '\n')
			want := strings.TrimSuffix(path, filepath.Ext(path)) + ".golden.json"
			if *update {
				if err := os.WriteFile(want, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			exp, err := os.ReadFile(want)
			if err != nil {
				t.Fatalf("%v (run go test -update)", err)
			}
			if !bytes.Equal(data, exp) {
				t.Errorf("golden mismatch for %s\n--- got\n%s\n--- want\n%s", path, data, exp)
			}
		})
	}
}

func TestParseErrorIsF001(t *testing.T) {
	_, err := Load("testdata/second-fence.py")
	var f Finding
	if !errors.As(err, &f) || f.Code != "F001" || f.Line != 10 {
		t.Fatalf("want F001 at line 10, got %#v (%v)", f, err)
	}
}

func TestStampRoundTrip(t *testing.T) {
	for _, path := range []string{"testdata/json-peek.py", "testdata/crlf-file.py"} {
		t.Run(path, func(t *testing.T) {
			b := mustLoad(t, path)
			hash := ContentHash(b, nil)
			if Indexed(b, hash) {
				t.Fatal("the example's stamp is not the real hash, so it must not be indexed")
			}

			stamped := WithStamp(b, Stamp{Verified: hash})
			b2 := mustParse(t, path, stamped)
			if got := ContentHash(b2, nil); got != hash {
				t.Fatalf("stamping changed the hash: %s -> %s", hash, got)
			}
			if !Indexed(b2, hash) {
				t.Fatal("stamped block is not indexed")
			}
			if again := WithStamp(b2, Stamp{Verified: hash}); !bytes.Equal(again, stamped) {
				t.Fatal("re-stamping unchanged content is not a no-op")
			}
			if crlf := bytes.Contains(b.Source, []byte("\r\n")); crlf && bytes.Count(stamped, []byte("\n")) != bytes.Count(stamped, []byte("\r\n")) {
				t.Fatal("CRLF file gained LF-only lines")
			}

			q := mustParse(t, path, WithStamp(b2, Stamp{Verified: hash, State: "quarantined"}))
			if Indexed(q, hash) || q.Header.Stamp.State != "quarantined" {
				t.Fatal("quarantined block must not be indexed")
			}

			bare := mustParse(t, path, WithStamp(b2, Stamp{}))
			if bare.StampLine != -1 || bare.Header.Stamp != nil || ContentHash(bare, nil) != hash {
				t.Fatal("removing the stamp must drop the table and keep the hash")
			}
		})
	}
}

func TestCRLFHashEqualsLF(t *testing.T) {
	crlf := mustLoad(t, "testdata/crlf-file.py")
	lf := mustParse(t, "crlf-file.py", bytes.ReplaceAll(crlf.Source, []byte("\r\n"), []byte("\n")))
	if ContentHash(crlf, nil) != ContentHash(lf, nil) {
		t.Fatal("CRLF and LF copies hash differently")
	}
}

func TestWithStampBytes(t *testing.T) {
	src := "# /// block\n# name = \"t\"\n# ///\nprint()\n"
	want := "# /// block\n# name = \"t\"\n#\n# [stamp]\n# verified = \"abcdef012345\"\n# ///\nprint()\n"
	b := mustParse(t, "t.py", []byte(src))
	got := WithStamp(b, Stamp{Verified: "abcdef012345"})
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if string(WithStamp(b, Stamp{})) != src {
		t.Fatal("removing an absent stamp changed bytes")
	}
	if h1, h2 := ContentHash(b, nil), ContentHash(mustParse(t, "t.py", got), nil); h1 != h2 {
		t.Fatalf("hash changed after stamping: %s -> %s", h1, h2)
	}
}

func TestStampMustBeLast(t *testing.T) {
	src := "# /// block\n# name = \"t\"\n# [stamp]\n# verified = \"x\"\n# [returns]\n# keys = []\n# ///\n"
	if _, err := Parse("t.py", []byte(src)); err == nil || !strings.Contains(err.Error(), "last table") {
		t.Fatalf("want last-table error, got %v", err)
	}
}

func TestContentHashFixtures(t *testing.T) {
	b := mustLoad(t, "testdata/json-peek.py")
	base := ContentHash(b, nil)
	a := ContentHash(b, map[string][]byte{"sample.json": []byte("{}"), "b/x.json": []byte("[]")})
	if a == base || len(a) != 12 {
		t.Fatalf("fixtures must change the 12-hex hash: %s %s", base, a)
	}
	if c := ContentHash(b, map[string][]byte{"sample.json": []byte("{ }"), "b/x.json": []byte("[]")}); c == a {
		t.Fatal("a fixture edit must change the hash")
	}
}

func TestParamOrderAndColumns(t *testing.T) {
	tests := []struct {
		name, header, order, column, hint string
	}{
		{
			name:   "inline tables keep header order",
			header: "[params]\nzeta = { type = \"str\", required = true }\nalpha = { type = \"int\", default = 2 }",
			order:  "zeta,alpha", column: "--zeta <str> [--alpha 2]", hint: "caveman-blocks run t --zeta <str>",
		},
		{
			name:   "sub-tables keep header order",
			header: "[params.mode]\ntype = \"enum\"\nvalues = [\"a\", \"b\"]\ndefault = \"a b\"\n[params.all]\ntype = \"bool\"\n[params.ratio]\ntype = \"float\"\ndefault = 0.5\n[params.out]\ntype = \"path\"",
			order:  "mode,all,ratio,out", column: `[--mode "a b"] [--all] [--ratio 0.5] [--out <path>]`, hint: "caveman-blocks run t",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := mustParse(t, "t.py", block("name = \"t\"\n"+tc.header, ""))
			var names []string
			for _, p := range b.Header.Params {
				names = append(names, p.Name)
			}
			if got := strings.Join(names, ","); got != tc.order {
				t.Errorf("order %q, want %q", got, tc.order)
			}
			if got := ParamsColumn(b); got != tc.column {
				t.Errorf("column %q, want %q", got, tc.column)
			}
			if got := CallHint(b); got != tc.hint {
				t.Errorf("hint %q, want %q", got, tc.hint)
			}
		})
	}
}

const (
	okHeader = "name = \"t\"\nsummary = \"Answers a test.\"\neffects = \"read\"\nexample = [\"--x\"]\n\n[returns]\nkeys = [\"ok\"]"
	okCode   = "import json\n\nprint(json.dumps({\"ok\": True}))\n"
)

// block renders a block file from TOML header text and code.
func block(header, code string) []byte {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env python3\n# /// block\n")
	for _, l := range strings.Split(header, "\n") {
		if l == "" {
			b.WriteString("#\n")
		} else {
			b.WriteString("# " + l + "\n")
		}
	}
	b.WriteString("# ///\n" + code)
	return []byte(b.String())
}

func TestLintRules(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		header string
		code   string
		opts   LintOptions
		want   []string // codes, in order
	}{
		{name: "clean", want: nil},
		{name: "bad name", header: strings.Replace(okHeader, `"t"`, `"Bad--Name"`, 1), path: "Bad--Name.py", want: []string{"F003"}},
		{name: "name not file name", path: "other.py", want: []string{"F003"}},
		{name: "long summary", header: strings.Replace(okHeader, "Answers a test.", strings.Repeat("x", 101), 1), want: []string{"F004"}},
		{name: "missing summary", header: strings.Replace(okHeader, "summary = \"Answers a test.\"\n", "", 1), want: []string{"F004"}},
		{name: "unknown effect", header: strings.Replace(okHeader, `"read"`, `"write"`, 1), want: []string{"F005"}},
		{name: "network floor", code: "import json\nimport urllib.request\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "from http import client", code: "import json\nfrom http import client\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "urllib.parse is not network", code: "import json\nfrom urllib.parse import quote\nprint(json.dumps({}))\n", want: nil},
		{name: "os.system is exec", code: "import json, os\nos.system('make')\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "write_text is a write", code: "import json\nfrom pathlib import Path\nPath('x').write_text('y')\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "open for append is a write", code: "import json\nopen('x', mode='a').close()\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "open for reading is not", code: "import json\nopen(os.path.join(d, 'a'), 'r').close()\nprint(json.dumps({}))\n", want: nil},
		{name: "declared effect at floor", header: strings.Replace(okHeader, `"read"`, `"network"`, 1), code: "import json\nimport httpx\nprint(json.dumps({}))\n", want: nil},
		{name: "smtplib is network", code: "import json\nimport smtplib\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "aiohttp is network", code: "import json\nfrom aiohttp import ClientSession\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "asyncio.open_connection is network", header: strings.Replace(okHeader, `"read"`, `"exec"`, 1), code: "import asyncio, json\nasyncio.open_connection('h', 1)\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "os.popen is exec", header: strings.Replace(okHeader, `"read"`, `"write-workspace"`, 1), code: "import json, os\nos.popen('ls')\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "os.spawnv is exec", code: "import json, os\nos.spawnv(os.P_WAIT, 'x', [])\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "multiprocessing is exec", code: "import json\nimport multiprocessing\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "create_subprocess_exec is exec", code: "import asyncio, json\nasyncio.create_subprocess_exec('ls')\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "exec floor met", header: strings.Replace(okHeader, `"read"`, `"exec"`, 1), code: "import json\nimport pty\nprint(json.dumps({}))\n", want: nil},
		{name: "shutil.rmtree is a write", code: "import json, shutil\nshutil.rmtree('x')\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "shutil.copy2 is a write", code: "import json, shutil\nshutil.copy2('a', 'b')\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "os.unlink is a write", code: "import json, os\nos.unlink('x')\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "Path.touch is a write", code: "import json\nfrom pathlib import Path\nPath('x').touch()\nprint(json.dumps({}))\n", want: []string{"F005"}},
		{name: "tempfile is not a write", code: "import json, tempfile\ntempfile.mkdtemp()\nprint(json.dumps({}))\n", want: nil},
		{name: "write floor met", header: strings.Replace(okHeader, `"read"`, `"write-workspace"`, 1), code: "import json, os\nos.makedirs('x')\nos.rename('x', 'y')\nprint(json.dumps({}))\n", want: nil},
		{name: "example not strings", header: strings.Replace(okHeader, `["--x"]`, `["--n", 1]`, 1), want: []string{"F006"}},
		{name: "bad matches", header: strings.Replace(okHeader, "example", "matches = ['(']\nexample", 1), want: []string{"F008"}},
		{name: "shell matches", header: strings.Replace(okHeader, "example", "matches = ['curl .*\\| *python', 'tail -n 5', 'json\\.load\\(']\nexample", 1), want: []string{"W002", "W002"}},
		{name: "stdlib name", header: strings.Replace(okHeader, `"t"`, `"json"`, 1), path: "json.py", want: []string{"F013"}},
		{name: "hyphenated name is not a module", header: strings.Replace(okHeader, `"t"`, `"json-peek"`, 1), path: "json-peek.py", want: nil},
		{name: "home path", code: okCode + "p = '/Users/ada/x'\nq = 'C:\\\\Users\\\\ada'\n", want: []string{"F009", "F009"}},
		{name: "no json.dumps", code: "print('ok')\n", want: []string{"F010", "F010"}},
		{name: "stderr print is fine", code: "import json, sys\nprint('log', file=sys.stderr)\nsys.stdout.write(json.dumps({}))\n", want: nil},
		{name: "stdout write", code: okCode + "import sys\nsys.stdout.write('x')\n", want: []string{"F010"}},
		{name: "empty returns", header: strings.Replace(okHeader, `keys = ["ok"]`, "keys = []", 1), want: []string{"F011"}},
		{name: "first party off", code: "import json, yaml\nprint(json.dumps({}))\n", want: nil},
		{name: "first party", code: "import json, yaml\nfrom . import x\nprint(json.dumps({}))\n", opts: LintOptions{FirstParty: true}, want: []string{"F012", "F012", "F012"}},
		{name: "default branch", opts: LintOptions{OnDefaultBranch: true}, want: []string{"W001"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header, code, path := tc.header, tc.code, tc.path
			if header == "" {
				header = okHeader
			}
			if code == "" {
				code = okCode
			}
			if path == "" {
				path = "t.py"
			}
			b := mustParse(t, path, block(header, code))
			var got []string
			for _, f := range Lint(b, tc.opts) {
				got = append(got, f.Code)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("codes %v, want %v; findings %+v", got, tc.want, Lint(b, tc.opts))
			}
		})
	}
}

func mustLoad(t *testing.T, path string) *Block {
	t.Helper()
	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustParse(t *testing.T, path string, src []byte) *Block {
	t.Helper()
	b, err := Parse(path, src)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
