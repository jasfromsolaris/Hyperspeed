#!/usr/bin/env node
/**
 * Pushes Worker secrets from apps/control-plane/.env (non-interactive stdin).
 * Expects:
 *   WORKER_CONTROL_PLANE_URL — base URL the Worker uses to reach the control plane (no trailing slash)
 *   CONTROL_PLANE_BEARER_TOKEN — same bearer the control plane expects from the Worker
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
  console.error(`Missing ${cpEnvPath}`);
  process.exit(1);
}
config({ path: cpEnvPath, quiet: true });

const workersTok =
  String(process.env.CLOUDFLARE_WORKERS_API_TOKEN || "").trim() ||
  String(process.env.CLOUDFLARE_API_TOKEN || "").trim();
if (!workersTok) {
  console.error("Set CLOUDFLARE_WORKERS_API_TOKEN or CLOUDFLARE_API_TOKEN in apps/control-plane/.env");
  process.exit(1);
}
process.env.CLOUDFLARE_API_TOKEN = workersTok;

const cpURL = String(process.env.WORKER_CONTROL_PLANE_URL || "").trim();
const bearer = String(process.env.CONTROL_PLANE_BEARER_TOKEN || "").trim();

if (!cpURL) {
  console.error(
    "Set WORKER_CONTROL_PLANE_URL in apps/control-plane/.env (HTTPS or tunnel URL; no trailing slash)."
  );
  process.exit(1);
}
if (!bearer) {
  console.error("Set CONTROL_PLANE_BEARER_TOKEN in apps/control-plane/.env");
  process.exit(1);
}

function putSecret(name, value) {
  const r = spawnSync("npx", ["wrangler", "secret", "put", name], {
    input: value,
    encoding: "utf8",
    cwd: workerRoot,
    env: { ...process.env },
    shell: true,
  });
  if (r.status !== 0) {
    console.error(r.stderr || r.stdout || `secret put ${name} failed`);
    process.exit(r.status ?? 1);
  }
  console.log(`Set secret ${name}`);
}

putSecret("CONTROL_PLANE_URL", cpURL);
putSecret("CONTROL_PLANE_BEARER_TOKEN", bearer);
console.log("Done. Run npm run cf:deploy when ready.");
