#!/usr/bin/env bash
# /// block
# name = "shell-block"
# summary = "Counts files under a directory."
# effects = "read"
# example = ["."]
#
# [returns]
# keys = ["files"]
# ///
set -euo pipefail
printf '{"files": %d}\n' "$(find "$1" -type f | wc -l)"
