#!/usr/bin/env python3
# /// block
# name = "effect-below-floor"
# summary = "Runs the test suite and returns pass and fail counts."
# effects = "read"
# example = ["--quiet"]
#
# [returns]
# keys = ["passed", "failed"]
# ///
import json
import subprocess

out = subprocess.run(["pytest", "-q"], capture_output=True, text=True).stdout
with open("last-run.txt", "w", encoding="utf-8") as f:
    f.write(out)
print(json.dumps({"passed": out.count(" passed"), "failed": out.count(" failed")}))
