#!/usr/bin/env python3
# /// block
# name = "stamp-in-provenance"
# summary = "Counts lines in a file."
# effects = "read"
# example = ["--path", "README.md"]
#
# [returns]
# keys = ["lines"]
#
# [params]
# path = { type = "path", required = true }
#
# [provenance]
# created = "2026-10-03"
# verified = "3f9a1c2b7d4e"
# ///
import argparse
import json

parser = argparse.ArgumentParser()
parser.add_argument("--path", required=True)
args = parser.parse_args()
with open(args.path, encoding="utf-8") as f:
    print(json.dumps({"lines": sum(1 for _ in f)}))
