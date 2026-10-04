#!/usr/bin/env sh
# Codex plugin shim: forward the PreToolUse payload on stdin to the caveman-blocks binary.
# The binary is not part of the plugin (hook scripts are not deployed by a web install), so look
# for it where `hooks install` puts it, then on PATH. Without one, answer {} and exit 0: the hook
# must never break a session (docs/HOOK.md, failure policy).
bin=${CAVEMAN_BLOCKS_BIN:-"$HOME/.local/share/caveman-blocks/bin/caveman-blocks"}
[ -x "$bin" ] || bin=$(command -v caveman-blocks 2>/dev/null) || { echo '{}'; exit 0; }
exec "$bin" hook --harness claude
