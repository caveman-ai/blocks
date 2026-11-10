// Caveman Blocks for OpenCode 2. Forwards each shell call to `caveman-blocks hook --harness generic`
// (docs/INTEGRATION.md, layer 1) and appends the one-line hint to the tool result, since the
// before-hook has no context field. Never blocks, never rewrites, fails open: without the binary, or
// on any error, OpenCode runs exactly as it would without this file.
//
// Contract: OpenCode 2 plugin API — default export { id, setup }
// (https://opencode.ai/v2/docs/build/plugins/migrate-v1).
//
// Install: `caveman-blocks hooks install --harness opencode` writes this file with the binary path
// baked in, or copy it to ~/.config/opencode/plugins/ or .opencode/plugins/ yourself.
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

const INSTALLED = "";

function binary() {
  for (const p of [INSTALLED, process.env.CAVEMAN_BLOCKS_BIN, join(homedir(), ".local/share/caveman-blocks/bin/caveman-blocks" + (process.platform === "win32" ? ".exe" : ""))]) {
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

export default {
  id: "caveman-blocks",
  async setup(ctx) {
    const hints = new Map(); // call id -> hint from the pre phase, delivered after the call
    const directory = ctx.location?.directory;
    const key = (event) => event.id ?? event.sessionID; // fall back when the host omits the call id

    await ctx.tool.hook("execute.before", (event) => {
      if (event.tool !== "bash" && event.tool !== "shell") return;
      const input = event.input ?? {};
      const command = String(input.command ?? "");
      const hint = hook({ phase: "pre", session: event.sessionID, call_id: event.id, cwd: input.workdir || directory, command });
      if (hint) hints.set(key(event), hint);
    });

    await ctx.tool.hook("execute.after", (event) => {
      const hint = hints.get(key(event));
      hints.delete(key(event));
      if (!hint || event.status !== "completed") return;
      const result = event.result;
      if (typeof result === "string") event.result = `${result}\n\n${hint}`;
      else if (result && typeof result.content === "string") event.result = { ...result, content: `${result.content}\n\n${hint}` };
    });
  },
};
