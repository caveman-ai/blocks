// Caveman Blocks for pi. Sends each bash call to `caveman-blocks hook --harness generic`
// (docs/INTEGRATION.md, layer 1) before it runs and appends the one-line hint to the bash result,
// since pi's tool_call event has no context field. The pre phase decides; the post phase replays that
// decision from the state dir's cache/ by tool call id. Never blocks, never rewrites, fails open:
// without the binary, or on any error, pi runs exactly as it would without this file.
//
// Install: `caveman-blocks hooks install --harness pi` writes this file with the binary path baked in,
// or copy it to ~/.pi/agent/extensions/ or .pi/extensions/ yourself.
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

function hook(phase, event, ctx) {
  try {
    const r = spawnSync(binary(), ["hook", "--harness", "generic", "--phase", phase], {
      input: JSON.stringify({
        protocol: 1,
        phase,
        harness: "pi",
        session: ctx.sessionManager.getSessionId(),
        call_id: event.toolCallId,
        cwd: ctx.cwd,
        command: String(event.input?.command ?? ""),
      }),
      encoding: "utf8",
      timeout: 5000,
    });
    return r.status === 0 && r.stdout ? JSON.parse(r.stdout).hint || "" : "";
  } catch {
    return "";
  }
}

export default function (pi) {
  const hinted = new Set(); // tool call ids whose pre phase produced a hint
  pi.on("tool_call", async (event, ctx) => {
    if (event.toolName !== "bash") return;
    if (hook("pre", event, ctx)) hinted.add(event.toolCallId);
  });
  pi.on("tool_result", async (event, ctx) => {
    if (event.toolName !== "bash" || !hinted.delete(event.toolCallId)) return;
    const hint = hook("post", event, ctx);
    if (!hint || !Array.isArray(event.content)) return;
    return { content: [...event.content, { type: "text", text: `\n\n${hint}` }] };
  });
}
