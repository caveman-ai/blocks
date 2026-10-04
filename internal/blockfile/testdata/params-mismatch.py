#!/usr/bin/env python3
# /// block
# name = "params-mismatch"
# summary = "Top N largest files under a directory."
# effects = "read"
# example = ["--dir", "."]
#
# [returns]
# keys = ["files"]
#
# [params]
# dir   = { type = "path", required = true }
# depth = { type = "int", default = 3 }
# ///
import argparse
import json
import os

parser = argparse.ArgumentParser()
parser.add_argument("--dir", required=True)
parser.add_argument("-n", "--limit", type=int, default=10)
parser.add_argument("--help-me", action="store_true")
args = parser.parse_args()
sizes = []
for root, _, names in os.walk(args.dir):
    sizes += [(os.path.getsize(os.path.join(root, n)), n) for n in names]
print("scanned", len(sizes))
print(json.dumps({"files": sorted(sizes, reverse=True)[: args.limit]}))
