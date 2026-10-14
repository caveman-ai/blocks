// Caveman Blocks for OpenCode. Forwards each shell call to `caveman-blocks hook --harness generic`
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
    const hint = r.status === 0 && r.stdout ? JSON.parse(r.stdout).hint : "";
    return typeof hint === "string" ? hint : "";
  } catch {
    return "";
  }
}

function pendingHints(directory) {
  const hints = new Map(); // (session, call) -> hint, consumed even when the tool fails
  const key = (session, call) => JSON.stringify([session, call]);
  return {
    before(session, call, args) {
      const id = key(session, call);
      hints.delete(id);
      const hint = hook({ phase: "pre", session, call_id: call, cwd: args?.workdir || directory, command: String(args?.command ?? "") });
      if (hint) hints.set(id, hint);
    },
    take(session, call) {
      const id = key(session, call);
      const hint = hints.get(id);
      hints.delete(id);
      return hint;
    },
    clear() { hints.clear(); },
  };
}

// V1's hook function is retained for hosts supporting the server() entrypoint (1.18.29+).
export const CavemanBlocks = async ({ directory }) => {
  const hints = pendingHints(directory);
  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") return;
      hints.before(input.sessionID, input.callID, output.args);
    },
    "tool.execute.after": async (input, output) => {
      if (input.tool !== "bash") return;
      const hint = hints.take(input.sessionID, input.callID);
      if (hint) output.output = `${output.output}\n\n${hint}`;
    },
    dispose: () => hints.clear(),
  };
};

// A plain definition keeps this standalone, installer-embedded file dependency-free.
export default {
  id: "caveman-blocks",
  server: CavemanBlocks,
  async setup(ctx) {
    const hints = pendingHints(ctx.location.directory);
    await ctx.tool.hook("execute.before", (event) => {
      if (event.tool !== "shell") return;
      hints.before(event.sessionID, event.id, event.input);
    });
    await ctx.tool.hook("execute.after", (event) => {
      if (event.tool !== "shell") return;
      const hint = hints.take(event.sessionID, event.id);
      if (!hint || event.status !== "completed") return;
      const content = event.result.content;
      event.result = {
        ...event.result,
        content: typeof content === "string"
          ? `${content}\n\n${hint}`
          : [...(content ?? []), { type: "text", text: hint }],
      };
    });
    return () => hints.clear();
  },
};
