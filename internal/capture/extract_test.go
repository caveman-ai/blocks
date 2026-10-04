package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

const jsonBody = "import json, sys\nfrom collections import Counter\nrows = [json.loads(l) for l in open('runs/out.jsonl')]\nc = Counter(r['kind'] for r in rows)\nfor k, n in c.most_common(10):\n    print(k, n)\n"

func TestExtract(t *testing.T) {
	cases := []struct {
		name, cmd string
		ok        bool
		body      string // expected body; "" means only check ok
		lines     int
		edit      bool
	}{
		{"heredoc quoted EOF", "python3 - <<'EOF'\n" + jsonBody + "EOF", true, jsonBody, 6, false},
		{"heredoc bare EOF", "python3 <<EOF\nprint(1)\nEOF\n", true, "print(1)\n", 1, false},
		{"heredoc double-quoted PY", "python3 - << \"PY\"\nimport os\nprint(os.getcwd())\nPY", true, "import os\nprint(os.getcwd())\n", 2, false},
		{"heredoc strip tabs", "python3 - <<-END\n\timport sys\n\tprint(sys.argv)\n\tEND\n", true, "import sys\nprint(sys.argv)\n", 2, false},
		{"cd prefix", "cd /Users/me/repo && python3 - <<'PY'\nimport json\nd = json.load(open('package.json'))\nprint(d['name'])\nPY", true, "import json\nd = json.load(open('package.json'))\nprint(d['name'])\n", 3, false},
		{"set -e and env prefix", "set -e; cd app; PYTHONPATH=. python3 - <<'EOF'\nimport app\nprint(app.VERSION)\nEOF", true, "import app\nprint(app.VERSION)\n", 2, false},
		{"pipe after heredoc", "python3 - <<'EOF' 2>&1 | tail -20\nimport subprocess\nsubprocess.run(['go', 'test'])\nEOF", true, "import subprocess\nsubprocess.run(['go', 'test'])\n", 2, false},
		{"python3.12 path", "/usr/local/bin/python3.12 - <<'EOF'\nprint('hi')\nEOF", true, "print('hi')\n", 1, false},
		{"venv python", ".venv/bin/python - <<'EOF'\nimport yaml\nprint(yaml.safe_load(open('c.yaml')))\nEOF", true, "", 2, false},
		{"uv run python", "uv run python - <<'EOF'\nimport csv\nprint(1)\nEOF", true, "import csv\nprint(1)\n", 2, false},
		{"uv run -", "uv run - <<'EOF'\nimport csv\nEOF", true, "import csv\n", 1, false},
		{"timeout prefix", "timeout 60 python3 - <<'EOF'\nimport time\ntime.sleep(1)\nEOF", true, "import time\ntime.sleep(1)\n", 2, false},
		{"-c double quotes with escapes", `python3 -c "import json,sys; d=json.load(sys.stdin); print(d[\"items\"][0])" < out.json`, true, `import json,sys; d=json.load(sys.stdin); print(d["items"][0])`, 1, false},
		{"-c single quotes", `python -c 'import sys; print(len(sys.argv))' a b`, true, `import sys; print(len(sys.argv))`, 1, false},
		{"-c multi-line", "cat data.json | python3 -c \"\nimport json, sys\nd = json.load(sys.stdin)\nprint(len(d))\n\"", true, "\nimport json, sys\nd = json.load(sys.stdin)\nprint(len(d))\n", 4, false},
		{"bash -lc wrapper", `bash -lc 'cd /repo && python3 -c "print(42)"'`, true, "print(42)", 1, false},
		{"bash -c wrapper heredoc", "bash -c \"python3 - <<'EOF'\nimport os\nprint(os.sep)\nEOF\"", true, "import os\nprint(os.sep)\n", 2, false},
		{"sh -c wrapper", `/bin/sh -c 'python3 -c "import re; print(re.sub(\"a\", \"b\", \"aa\"))"'`, true, `import re; print(re.sub("a", "b", "aa"))`, 1, false},
		{"edit open read write", "python3 - <<'EOF'\np = 'src/app.py'\ns = open(p).read()\ns = s.replace('old', 'new')\nopen(p, 'w').write(s)\nEOF", true, "", 4, true},
		{"edit pathlib", "python3 - <<'EOF'\nfrom pathlib import Path\nimport re\nf = Path('docs/README.md')\nt = f.read_text()\nt = re.sub(r'v\\d+', 'v2', t)\nf.write_text(t)\nEOF", true, "", 6, true},
		{"edit Path literal", "python3 - <<'EOF'\nimport pathlib\nt = pathlib.Path(\"a/b.go\").read_text()\npathlib.Path(\"a/b.go\").write_text(t.replace(\"x\", \"y\"))\nEOF", true, "", 3, true},
		{"edit with blocks", "python3 - <<'EOF'\nimport sys\nfor p in sys.argv[1:]:\n    with open(p) as f:\n        s = f.read()\n    with open(p, 'w', encoding='utf-8') as f:\n        f.write(s.rstrip() + '\\n')\nEOF", true, "", 6, true},
		{"edit variable resolves to literal", "python3 - <<'EOF'\npath = 'go.mod'\ns = open('go.mod').read()\nopen(path, mode='w').write(s)\nEOF", true, "", 3, true},
		{"not edit: read a, write b", "python3 - <<'EOF'\nimport json\nd = json.load(open('in.json'))\njson.dump(d, open('out.json', 'w'))\nEOF", true, "", 3, false},
		{"not edit: write only", "python3 - <<'EOF'\nopen('notes.txt', 'w').write('hello')\nEOF", true, "", 1, false},
		{"node -e", `node -e "console.log(require('./package.json').version)"`, false, "", 0, false},
		{"node heredoc", "node - <<'EOF'\nconsole.log(1)\nEOF", false, "", 0, false},
		{"cat heredoc", "cat > notes.md <<'EOF'\npython3 -c \"print(1)\"\nEOF", false, "", 0, false},
		{"psql heredoc", "psql \"$DATABASE_URL\" <<'SQL'\nselect 1;\nSQL", false, "", 0, false},
		{"script file", "python3 scripts/build.py --fast", false, "", 0, false},
		{"script file with stdin heredoc", "python3 tool.py <<'EOF'\ninput\nEOF", false, "", 0, false},
		{"module", "python3 -m pytest -q", false, "", 0, false},
		{"echo mentioning python", `echo "python3 -c 'print(1)'"`, false, "", 0, false},
		{"comment mentioning python", "# python3 -c 'print(1)'\nls", false, "", 0, false},
		{"plain command", "go test ./... 2>&1 | tail -30", false, "", 0, false},
		{"commit message heredoc", "git commit -m \"$(cat <<'EOF'\nfix: run python3 -c 'print(1)' in CI\n\npython3 - <<EOF too\nEOF\n)\"", false, "", 0, false},
		{"python after non-python heredoc", "cat > /tmp/x.json <<'JSON'\n{\"a\": 1}\nJSON\npython3 -c \"import json; print(json.load(open('/tmp/x.json')))\"", true, "import json; print(json.load(open('/tmp/x.json')))", 1, false},
		{"empty -c", `python3 -c ""`, false, "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, ok := Extract(c.cmd)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v (body %q)", ok, c.ok, s.Body)
			}
			if !ok {
				return
			}
			if c.body != "" && s.Body != c.body {
				t.Errorf("body = %q, want %q", s.Body, c.body)
			}
			if s.Lines != c.lines {
				t.Errorf("lines = %d, want %d", s.Lines, c.lines)
			}
			if s.Edit != c.edit {
				t.Errorf("edit = %v, want %v", s.Edit, c.edit)
			}
			if s.Lang != "py" || len(s.ScriptSHA) != 64 {
				t.Errorf("lang %q sha %q", s.Lang, s.ScriptSHA)
			}
		})
	}
}

func TestFingerprint(t *testing.T) {
	a, _ := Extract("python3 - <<'EOF'\n" + jsonBody + "EOF")
	b, _ := Extract("cd x && python3 - <<'PY'\n" + strings.NewReplacer("runs/out.jsonl", "/tmp/other.jsonl", "'kind'", "'model'", "10", "25").Replace(jsonBody) + "PY")
	if a.FP == "" || len(a.FP) != 12 {
		t.Fatalf("fp = %q", a.FP)
	}
	if a.FP != b.FP {
		t.Errorf("same shape, different literals: fp %s != %s", a.FP, b.FP)
	}
	if a.ScriptSHA != b.ScriptSHA {
		t.Errorf("literals are stripped from script_sha, so a literal-only change must keep it")
	}
	if got, want := a.Literals, []string{"runs/out.jsonl", "kind", "10"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("literals = %q, want %q", got, want)
	}

	// The documented formula, computed by hand.
	_, fp, _ := analyze("import json\nimport os.path\nfrom collections import Counter\nprint(json.load(open('x')))\nprint(1)\n")
	if want := hex12("py|collections,json,os|json.load:1,open:1,print:2"); fp != want {
		t.Errorf("fp = %s, want %s", fp, want)
	}

	// Call names match whole dotted segments, not substrings, and never inside strings or comments.
	_, fp1, _ := analyze("x = json.loads(s)  # json.load\nprint('open')\n")
	if want := hex12("py||json.loads:1,print:1"); fp1 != want {
		t.Errorf("fp = %s, want %s", fp1, want)
	}
	_, fp2, _ := analyze("import glob\nfor f in glob.glob('*.py'): print(f)\nprint(1)\nprint(2)\n")
	if want := hex12("py|glob|glob:2,print:3+"); fp2 != want {
		t.Errorf("fp = %s, want %s", fp2, want)
	}

	// No imports and no table hits: no fp.
	if _, fp, _ := analyze("x = 1 + 2\ny = x * 3\n"); fp != "" {
		t.Errorf("featureless body fp = %q", fp)
	}
}

func TestScriptSHA(t *testing.T) {
	a, _, _ := analyze("import json\nd = json.load(open('a.json'))\nprint(d['x'], 3)\n")
	b, _, _ := analyze("import json\nd   =   json.load(open(\"b/c.json\"))   # read\n\nprint(d['y'], 7)\n")
	c, _, _ := analyze("import json\nd = json.load(open('a.json'))\nprint(d['x'] + 3)\n")
	if a != b {
		t.Errorf("literal, comment and whitespace changes should keep script_sha")
	}
	if a == c {
		t.Errorf("code change should change script_sha")
	}
	_, fpA, _ := analyze("import json\nprint(json.load(open('a.json'))['x'])\n")
	_, fpB, _ := analyze("import json\nd = json.load(open('zzz.json'))\nk = 'y'\nprint(d[k])\n")
	shaA, _, _ := analyze("import json\nprint(json.load(open('a.json'))['x'])\n")
	shaB, _, _ := analyze("import json\nd = json.load(open('zzz.json'))\nk = 'y'\nprint(d[k])\n")
	if fpA != fpB || shaA == shaB {
		t.Errorf("same shape, different script: want equal fp (%s, %s) and different script_sha", fpA, fpB)
	}
}

func TestIsBlockCall(t *testing.T) {
	cases := []struct {
		cmd, name string
		ok        bool
	}{
		{"caveman-blocks run json-peek --path a.json", "json-peek", true},
		{"cd /repo && caveman-blocks run jsonl-stats --path x.jsonl", "jsonl-stats", true},
		{"python3 .blocks/first-error.py --log build.log", "first-error", true},
		{"python ./.blocks/test-summary.py", "test-summary", true},
		{`bash -lc 'caveman-blocks run http-json --url x'`, "http-json", true},
		{"FOO=1 ~/.local/share/caveman-blocks/bin/caveman-blocks run wait-for", "wait-for", true},
		{"caveman-blocks run Bad_Name", "", false},
		{"caveman-blocks verify --all", "", false},
		{"python3 scripts/x.py", "", false},
		{"python3 .blocks/../evil.py", "", false},
		{"ls && caveman-blocks run json-peek", "", false},
	}
	for _, c := range cases {
		name, ok := IsBlockCall(c.cmd)
		if name != c.name || ok != c.ok {
			t.Errorf("IsBlockCall(%q) = %q, %v; want %q, %v", c.cmd, name, ok, c.name, c.ok)
		}
	}
}

func TestStructuredDump(t *testing.T) {
	cases := []struct {
		cmd, ext string
		ok       bool
	}{
		{"cat package.json", "json", true},
		{"cat ~/.claude/projects/x/session.jsonl", "jsonl", true},
		{"cd logs && cat -n server.log", "log", true},
		{"more server.log", "log", true},
		{"cd logs && tail -n 200 server.log", "", false},
		{"head -50 data.json", "", false},
		{"cat data.csv", "", false},
		{"less events.ndjson", "ndjson", true},
		{"bat Results.JSON", "json", true},
		{"cat big.jsonl | sort", "jsonl", true},
		{"cat big.jsonl | head -5", "", false},
		{"cat app.log | grep ERROR", "", false},
		{"cat a.json | jq .name", "", false},
		{"cat x.jsonl | sort | wc -l", "", false},
		{"cat a.json b.json", "", false},
		{"cat a.json > b.json", "", false},
		{"cat README.md", "", false},
		{"grep x a.json", "", false},
		{"jq . a.json", "", false},
	}
	for _, c := range cases {
		ext, ok := StructuredDump(c.cmd)
		if ext != c.ext || ok != c.ok {
			t.Errorf("StructuredDump(%q) = %q, %v; want %q, %v", c.cmd, ext, ok, c.ext, c.ok)
		}
	}
}

func hex12(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

const typicalCmd = "cd /Users/me/repo && python3 - <<'EOF'\nimport json, sys\nfrom collections import Counter\n\npath = 'runs/2026-10-03/results.jsonl'\nrows = []\nwith open(path) as f:\n    for line in f:\n        line = line.strip()\n        if not line:\n            continue\n        rows.append(json.loads(line))\nprint('rows', len(rows))\nby_model = Counter(r.get('model') for r in rows)\nfor m, n in by_model.most_common(10):\n    print(f'{m:30s} {n:6d}')\nfail = [r for r in rows if r.get('status') != 'ok']\nprint('failures', len(fail))\nfor r in fail[:5]:\n    print(json.dumps(r)[:200])\nEOF"

func BenchmarkExtract(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := Extract(typicalCmd); !ok {
			b.Fatal("no script")
		}
	}
}

// TestLiteralsScrubbed pins F3: a secret assigned in the body, or repeated elsewhere in it, never
// reaches the literal vector; positions are kept.
func TestLiteralsScrubbed(t *testing.T) {
	body := "import json\nhexkey = \"abcd1234efgh5678\"\nh = {\"X-Api-Key\": \"zzzz9999yyyy8888\"}\nprint(\"abcd1234efgh5678\", 'out.json', 3)\n"
	s, ok := Extract("python3 - <<'EOF'\n" + body + "EOF")
	if !ok {
		t.Fatal("no script")
	}
	want := []string{scrubbed, "X-Api-Key", scrubbed, scrubbed, "out.json", "3"}
	if strings.Join(s.Literals, "|") != strings.Join(want, "|") {
		t.Errorf("literals = %q, want %q", s.Literals, want)
	}
	for _, l := range s.Literals {
		if strings.Contains(l, "abcd1234") || strings.Contains(l, "zzzz9999") {
			t.Errorf("secret in literals: %q", s.Literals)
		}
	}
}
