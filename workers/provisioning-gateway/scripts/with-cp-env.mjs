#!/usr/bin/env node
/**
 * Loads apps/control-plane/.env so CLOUDFLARE_API_TOKEN is set for Wrangler (non-interactive).
 * Usage: node scripts/with-cp-env.mjs -- npx wrangler whoami
 */
import { config } from "dotenv";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import fs from "node:fs";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const workerRoot = path.join(__dirname, "..");
const cpEnvPath = path.join(workerRoot, "..", "..", "apps", "control-plane", ".env");

if (!fs.existsSync(cpEnvPath)) {
  console.error(`Missing ${cpEnvPath}\nCopy apps/control-plane/.env.example and set CLOUDFLARE_API_TOKEN.`);
  process.exit(1);
}

const result = config({ path: cpEnvPath, quiet: true });
if (result.error) {
  console.error("Failed to read control-plane .env:", result.error.message);
  process.exit(1);
}

// Optional: Workers-capable token; falls back to CLOUDFLARE_API_TOKEN (DNS-only tokens break cf:bootstrap / deploy).
const workersTok =
  String(process.env.CLOUDFLARE_WORKERS_API_TOKEN || "").trim() ||
  String(process.env.CLOUDFLARE_API_TOKEN || "").trim();
if (!workersTok) {
  console.error(
    "Set CLOUDFLARE_WORKERS_API_TOKEN or CLOUDFLARE_API_TOKEN in apps/control-plane/.env for Wrangler."
  );
  process.exit(1);
}
process.env.CLOUDFLARE_API_TOKEN = workersTok;

const dash = process.argv.indexOf("--");
if (dash === -1 || dash === process.argv.length - 1) {
  console.error("Usage: node scripts/with-cp-env.mjs -- <command> [args...]");
  process.exit(1);
}

const args = process.argv.slice(dash + 1);
const cmd = args[0];
const cmdArgs = args.slice(1);

const r = spawnSync(cmd, cmdArgs, {
  stdio: "inherit",
  env: { ...process.env },
  cwd: workerRoot,
  shell: process.platform === "win32",
});

process.exit(r.status ?? 1);
