#!/usr/bin/env python3
# /// block
# name = "crlf-file"
# summary = "Shape of a JSON or JSONL file: keys, row count, one sample. Never the data."
# effects = "read"
# example = ["--path", "$FIXTURES/sample.json", "--depth", "1"]
# matches = ['json\.loads?\(', 'JSON\.parse\(']
#
# [returns]
# keys = ["kind", "keys", "rows", "sample", "truncated"]
# doc = "kind is object|array|jsonl; keys are top-level; sample is one element"
#
# [params]
# path  = { type = "path", required = true, help = "JSON or JSONL file inside the repo" }
# depth = { type = "int", default = 2, max = 10, help = "How deep to walk nested keys" }
#
# [provenance]
# created = "2026-10-03"
# source = "registry:crlf-file@0.1.0"
#
# [stamp]
# verified = "3f9a1c2b7d4e"
# ///
import argparse
import json
import sys


def shape(value, depth):
    if depth <= 0 or not isinstance(value, dict):
        return type(value).__name__
    return {k: shape(v, depth - 1) for k, v in value.items()}


def main():
    parser = argparse.ArgumentParser(description="Shape of a JSON or JSONL file.")
    parser.add_argument("--path", required=True)
    parser.add_argument("--depth", type=int, default=2)
    args = parser.parse_args()
    try:
        with open(args.path, encoding="utf-8") as f:
            text = f.read()
    except OSError as e:
        print(json.dumps({"error": str(e)}))
        return 1
    try:
        data, kind = json.loads(text), None
    except json.JSONDecodeError:
        data, kind = [json.loads(line) for line in text.splitlines() if line.strip()], "jsonl"
    rows = data if isinstance(data, list) else [data]
    answer = {
        "kind": kind or ("array" if isinstance(data, list) else "object"),
        "keys": sorted(data) if isinstance(data, dict) else [],
        "rows": len(rows),
        "sample": shape(rows[0], args.depth) if rows else None,
        "truncated": False,
    }
    print(json.dumps(answer))
    return 0


if __name__ == "__main__":
    sys.exit(main())
