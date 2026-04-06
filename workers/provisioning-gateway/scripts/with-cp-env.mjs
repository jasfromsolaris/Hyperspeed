#!/usr/bin/env node
/**
 * Loads operator Cloudflare / control-plane secrets for Wrangler (non-interactive).
 * Resolves env file in order: HYPERSPEED_OPERATOR_ENV, workers/provisioning-gateway/.env,
 * apps/control-plane/.env (private monorepo).
 * Usage: node scripts/with-cp-env.mjs -- npx wrangler whoami
 */
import { config } from "dotenv";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import fs from "node:fs";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const workerRoot = path.join(__dirname, "..");
const repoRoot = path.join(workerRoot, "..", "..");

const candidates = [];
const fromEnv = String(process.env.HYPERSPEED_OPERATOR_ENV || "").trim();
if (fromEnv) {
  candidates.push(path.isAbsolute(fromEnv) ? fromEnv : path.join(process.cwd(), fromEnv));
}
candidates.push(path.join(workerRoot, ".env"));
candidates.push(path.join(repoRoot, "apps", "control-plane", ".env"));

const cpEnvPath = candidates.find((p) => fs.existsSync(p));

if (!cpEnvPath) {
  console.error(
    `No operator env file found. Tried:\n${candidates.map((p) => `  - ${p}`).join("\n")}\n\n` +
      `Create workers/provisioning-gateway/.env from .env.example, or set HYPERSPEED_OPERATOR_ENV, ` +
      `or use apps/control-plane/.env in the private monorepo.`
  );
  process.exit(1);
}

const result = config({ path: cpEnvPath, quiet: true });
if (result.error) {
  console.error("Failed to read operator env:", result.error.message);
  process.exit(1);
}

// Optional: Workers-capable token; falls back to CLOUDFLARE_API_TOKEN (DNS-only tokens break cf:bootstrap / deploy).
const workersTok =
  String(process.env.CLOUDFLARE_WORKERS_API_TOKEN || "").trim() ||
  String(process.env.CLOUDFLARE_API_TOKEN || "").trim();
if (!workersTok) {
  console.error(
    "Set CLOUDFLARE_WORKERS_API_TOKEN or CLOUDFLARE_API_TOKEN in the operator env file for Wrangler."
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
