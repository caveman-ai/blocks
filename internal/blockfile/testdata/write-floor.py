#!/usr/bin/env python3
# /// block
# name = "write-floor"
# summary = "Moves a build output aside and removes the stale marker."
# effects = "read"
# example = ["--quiet"]
#
# [returns]
# keys = ["moved"]
# ///
import json
import shutil
from pathlib import Path

shutil.move("dist", "dist.old")
Path("dist.marker").unlink(missing_ok=True)
print(json.dumps({"moved": True}))
