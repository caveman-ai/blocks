"""Runs every first-party block's header example and a few behaviors. python3 -m unittest blocks/test_blocks.py"""
import functools
import http.server
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import unittest

import tomllib

HERE = os.path.dirname(os.path.abspath(__file__))
FIXTURES = os.path.join(HERE, "fixtures")
BLOCKS = sorted(name[:-3] for name in os.listdir(HERE) if name.endswith(".py") and name != "test_blocks.py")
FENCE = re.compile(r"(?m)^# /// block$\s(?P<content>(^#(| .*)$\s)+)^# ///$")


def header(name):
    with open(os.path.join(HERE, f"{name}.py"), encoding="utf-8") as fh:
        source = fh.read()
    match = FENCE.search(source)
    assert match, f"{name}: no block header"
    assert source[: match.start()].count("\n") < 20, f"{name}: header not in the first 20 lines"
    content = "".join(line[2:] if line.startswith("# ") else line[1:] for line in match["content"].splitlines(True))
    return tomllib.loads(content)


def run(name, args, out_dir):
    env = dict(os.environ, BLOCKS_OUT=out_dir)
    return subprocess.run(
        [sys.executable, os.path.join(HERE, f"{name}.py"), *args], capture_output=True, text=True, env=env, timeout=60, check=False
    )


class Blocks(unittest.TestCase):
    def setUp(self):
        self.out = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.out)

    def answer(self, name, args, code=0):
        result = run(name, args, self.out)
        self.assertEqual(result.returncode, code, result.stdout + result.stderr)
        answer = json.loads(result.stdout)
        self.assertIsInstance(answer, dict)
        return answer

    def test_examples(self):
        self.assertEqual(len(BLOCKS), 8, BLOCKS)
        for name in BLOCKS:
            with self.subTest(block=name):
                head = header(name)
                self.assertEqual(head["name"], name)
                self.assertLessEqual(len(head["summary"]), 100)
                self.assertIn(head["effects"], ("read", "write-workspace", "exec", "network"))
                self.assertEqual(head["provenance"]["source"], f"registry:{name}@0.1.0")
                self.assertNotIn("stamp", head)
                fixtures = os.path.join(FIXTURES, name)
                args = [arg.replace("$FIXTURES", fixtures) for arg in head["example"]]
                result = run(name, args, self.out)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertLessEqual(len(result.stdout.encode()), 2048, result.stdout)
                answer = json.loads(result.stdout)
                self.assertIsInstance(answer, dict)
                for key in head["returns"]["keys"]:
                    self.assertIn(key, answer)
                self.assertEqual(run(name, ["--help"], self.out).returncode, 0)
                size = sum(os.path.getsize(os.path.join(d, f)) for d, _, fs in os.walk(fixtures) for f in fs)
                self.assertLessEqual(size, 64 * 1024)

    def test_json_peek_jsonl(self):
        answer = self.answer("json-peek", ["--path", os.path.join(FIXTURES, "json-peek", "events.jsonl")])
        self.assertEqual(answer["kind"], "jsonl")
        self.assertEqual(answer["rows"], 4)
        self.assertIn("usage.input", answer["keys"])

    def test_first_error_finds_first_match(self):
        answer = self.answer("first-error", ["--path", os.path.join(FIXTURES, "first-error", "build.log")])
        self.assertEqual(answer["line"], 8)
        self.assertIn("undefined: SessionStore", answer["match"])
        self.assertEqual(answer["total_matches"], 4)

    def test_replace_in_file(self):
        path = os.path.join(self.out, "a.txt")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("x = 1\nx = 1\nx = 1\n")
        answer = self.answer("replace-in-file", ["--path", path, "--old", "x = 1", "--new", "x = 2"], code=1)
        self.assertEqual(answer, {"error": "expected 1 match, found 3"})
        answer = self.answer("replace-in-file", ["--path", path, "--old", "x = 1", "--new", "x = 2", "--count", "0"])
        self.assertEqual((answer["matches"], answer["replaced"]), (3, 3))
        with open(path, encoding="utf-8") as fh:
            self.assertEqual(fh.read(), "x = 2\nx = 2\nx = 2\n")

    def test_grep_defs_go(self):
        path = os.path.join(self.out, "store.go")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("package store\n\ntype Store struct{}\n\nfunc (s *Store) Get(k string) string { return k }\n")
        answer = self.answer("grep-defs", ["--path", path])
        self.assertEqual([(d["kind"], d["name"]) for d in answer["defs"]], [("type", "Store"), ("func", "Get")])

    def test_test_summary_go(self):
        script = os.path.join(self.out, "go.txt")
        with open(script, "w", encoding="utf-8") as fh:
            fh.write("=== RUN   TestA\n--- PASS: TestA (0.00s)\n=== RUN   TestB\n    b_test.go:9: want 2\n")
            fh.write("--- FAIL: TestB (0.00s)\nFAIL\n")
        answer = self.answer("test-summary", ["--cmd", f"cat '{script}'; exit 1"])
        self.assertEqual((answer["passed"], answer["failed"], answer["exit"]), (1, 1, 1))
        self.assertEqual(answer["first_failure"]["name"], "TestB")
        self.assertIn("    b_test.go:9: want 2", answer["first_failure"]["trace_head"])
        self.assertTrue(answer["log"].startswith(self.out))

    def test_http_json_over_http(self):
        handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=os.path.join(FIXTURES, "http-json"))
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        base = f"http://127.0.0.1:{server.server_address[1]}"
        answer = self.answer("http-json", ["--url", f"{base}/sample.json", "--keys", "owner.id"])
        self.assertEqual((answer["status"], answer["selected"]), (200, {"owner.id": 42}))
        self.assertEqual(self.answer("http-json", ["--url", f"{base}/missing.json"])["status"], 404)


if __name__ == "__main__":
    unittest.main()
