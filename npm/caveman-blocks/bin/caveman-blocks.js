#!/usr/bin/env node
// Bootstrap shim only. `caveman-blocks hooks install` copies the native binary to a stable path and
// points agent hooks at it, so this file is never on the per-command hot path (docs/decisions/0007).
const { spawnSync } = require("child_process");
const exe = process.platform === "win32" ? ".exe" : "";
const pkg = `@caveman-blocks/${process.platform}-${process.arch}/caveman-blocks${exe}`;
let bin = process.env.CAVEMAN_BLOCKS_BINARY;
if (!bin) {
  try {
    bin = require.resolve(pkg);
  } catch {
    console.error(
      `caveman-blocks: no prebuilt binary for ${process.platform}-${process.arch}.\n` +
        `Install with: curl -fsSL https://caveman.so/blocks/install.sh | sh\n` +
        `or set CAVEMAN_BLOCKS_BINARY to a built binary.`
    );
    process.exit(1);
  }
}
const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (r.error) {
  console.error(`caveman-blocks: ${r.error.message}`);
  process.exit(1);
}
process.exit(r.status ?? 1);
