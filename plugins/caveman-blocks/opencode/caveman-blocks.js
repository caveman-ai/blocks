// Caveman Blocks for OpenCode. Forwards each bash call to `caveman-blocks hook --harness generic`
// (docs/INTEGRATION.md, layer 1) and appends the one-line hint to the tool output, since OpenCode's
// before-hook has no context field. Never blocks, never rewrites, fails open: without the binary, or
// on any error, OpenCode runs exactly as it would without this file.
//
// Install: `caveman-blocks hooks install --harness opencode` writes this file with the binary path
// baked in, or copy it to ~/.config/opencode/plugins/ or .opencode/plugins/ yourself.
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

const INSTALLED = "";

function binary() {
  for (const p of [INSTALLED, process.env.CAVEMAN_BLOCKS_BIN, join(homedir(), ".local/share/caveman-blocks/bin/caveman-blocks")]) {
    if (p && existsSync(p)) return p;
  }
  return "caveman-blocks"; // PATH; a missing binary is swallowed by hook()
}

function hook(req) {
  try {
    const r = spawnSync(binary(), ["hook", "--harness", "generic"], {
      input: JSON.stringify({ protocol: 1, harness: "opencode", ...req }),
      encoding: "utf8",
      timeout: 5000,
    });
    return r.status === 0 && r.stdout ? JSON.parse(r.stdout).hint || "" : "";
  } catch {
    return "";
  }
}

export const CavemanBlocks = async ({ directory }) => {
  const hints = new Map(); // callID -> hint from the pre phase, delivered after the call
  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") return;
      const command = String(output.args?.command ?? "");
      const hint = hook({ phase: "pre", session: input.sessionID, call_id: input.callID, cwd: output.args?.workdir || directory, command });
      if (hint) hints.set(input.callID, hint);
    },
    "tool.execute.after": async (input, output) => {
      const hint = hints.get(input.callID);
      hints.delete(input.callID);
      if (hint) output.output = `${output.output}\n\n${hint}`;
    },
  };
};
