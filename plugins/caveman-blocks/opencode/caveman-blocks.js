// Caveman Blocks for OpenCode. Forwards each bash call to `caveman-blocks hook --harness generic`
// (docs/INTEGRATION.md, layer 1) and appends the one-line hint to the tool output, since OpenCode's
// before-hook has no context field. Never blocks, never rewrites, fails open: without the binary, or
// on any error, OpenCode runs exactly as it would without this file.
//
// Install: `caveman-blocks hooks install --harness opencode` writes this file with the binary path
// baked in, or copy it to ~/.config/opencode/plugins/ or .opencode/plugins/ yourself.
//
// OpenCode 2 replaced the plugin API: a V2 plugin default-exports a definition with an `id` and a
// `setup(ctx)`, and it registers tool hooks through `ctx.tool.hook`. A V1 hook map no longer loads
// (PluginModule.LoadError, missing "default"). This file carries both: the named `CavemanBlocks`
// export is the V1 hook map, and the default export is the V2 definition whose `server` points at
// the same V1 implementation for OpenCode 1.4+.
//
// OpenCode 2 also renamed the shell tool: V1 called it `bash`, V2 calls it `shell`. The V2 hook
// matches both names, so one file serves either host.
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

const INSTALLED = "";

// The tool that runs a shell command: `bash` on OpenCode 1.x, `shell` on OpenCode 2.x.
const SHELL_TOOLS = new Set(["shell", "bash"]);

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

// withHint returns the tool result with the hint appended. OpenCode's bash result carries the text
// in `output`; `content` is the fallback shape for tools that return content parts.
function withHint(result, hint) {
  if (!result || typeof result !== "object") return result;
  if (typeof result.output === "string") return { ...result, output: `${result.output}\n\n${hint}` };
  if (typeof result.content === "string") return { ...result, content: `${result.content}\n\n${hint}` };
  if (Array.isArray(result.content)) return { ...result, content: [...result.content, { type: "text", text: hint }] };
  return result;
}

export const CavemanBlocks = async ({ directory }) => {
  const hints = new Map(); // callID -> hint from the pre phase, delivered after the call
  return {
    "tool.execute.before": async (input, output) => {
      if (!SHELL_TOOLS.has(input.tool)) return;
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

export default {
  id: "caveman-blocks",

  // OpenCode 1.4+ calls this; OpenCode 2.x reads `id` and calls `setup(ctx)`.
  server: CavemanBlocks,

  async setup(ctx) {
    // OpenCode 1.x also calls setup(), with a context that has no tool API.
    if (!ctx || !ctx.tool || typeof ctx.tool.hook !== "function") return;

    const hints = new Map(); // call id -> hint from the pre phase, delivered after the call
    const directory = ctx.location?.directory;

    await ctx.tool.hook("execute.before", (event) => {
      if (!SHELL_TOOLS.has(event.tool)) return;
      const input = event.input && typeof event.input === "object" ? event.input : {};
      const command = String(input.command ?? "");
      const hint = hook({ phase: "pre", session: event.sessionID, call_id: event.id, cwd: input.workdir || directory, command });
      if (hint) hints.set(event.id, hint);
    });

    await ctx.tool.hook("execute.after", (event) => {
      const hint = hints.get(event.id);
      if (!hint) return;
      hints.delete(event.id);
      if (event.status !== "completed") return;
      event.result = withHint(event.result, hint);
    });
  },
};
