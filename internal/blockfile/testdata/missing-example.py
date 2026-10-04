#!/usr/bin/env python3
# /// block
# name = "missing-example"
# summary = "Counts lines in a file."
# effects = "read"
#
# [returns]
# keys = ["lines"]
#
# [params]
# path = { type = "path", required = true }
# ///
import argparse
import json

parser = argparse.ArgumentParser()
parser.add_argument("--path", required=True)
args = parser.parse_args()
with open(args.path, encoding="utf-8") as f:
    print(json.dumps({"lines": sum(1 for _ in f)}))
