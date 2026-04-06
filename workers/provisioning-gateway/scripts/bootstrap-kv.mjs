#!/usr/bin/env node
/**
 * Creates INSTALL_SECRETS + RATE_LIMITS KV namespaces via Wrangler API and writes ids into wrangler.toml.
 * Requires CLOUDFLARE_API_TOKEN in apps/control-plane/.env (same token as control-plane DNS).
 */
import { config } from "dotenv";
import { execSync } from "node:child_process";
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

function wrangler(args) {
  return execSync(`npx wrangler ${args}`, {
    encoding: "utf8",
    cwd: workerRoot,
    env: { ...process.env },
    shell: true,
  });
}

function parseKvId(output) {
  const m = output.match(/id\s*=\s*"([a-f0-9]{32})"/i);
  if (m) return m[1];
  const m2 = output.match(/\b([a-f0-9]{32})\b/);
  return m2 ? m2[1] : null;
}

console.log("Creating KV namespace INSTALL_SECRETS…");
const out1 = wrangler(`kv namespace create "hyperspeed-provisioning-gateway-INSTALL_SECRETS" --config wrangler.bootstrap.toml`);
const id1 = parseKvId(out1);
if (!id1) {
  console.error("Could not parse KV id from wrangler output:\n", out1);
  process.exit(1);
}
console.log("INSTALL_SECRETS id:", id1);

console.log("Creating KV namespace RATE_LIMITS…");
const out2 = wrangler(`kv namespace create "hyperspeed-provisioning-gateway-RATE_LIMITS" --config wrangler.bootstrap.toml`);
const id2 = parseKvId(out2);
if (!id2) {
  console.error("Could not parse KV id from wrangler output:\n", out2);
  process.exit(1);
}
console.log("RATE_LIMITS id:", id2);

const toml = `# KV bindings created by npm run cf:bootstrap (see scripts/bootstrap-kv.mjs)
name = "hyperspeed-provisioning-gateway"
main = "src/index.ts"
compatibility_date = "2024-11-01"

[[kv_namespaces]]
binding = "INSTALL_SECRETS"
id = "${id1}"

[[kv_namespaces]]
binding = "RATE_LIMITS"
id = "${id2}"
`;

const tomlPath = path.join(workerRoot, "wrangler.toml");
fs.writeFileSync(tomlPath, toml, "utf8");
console.log("Updated", tomlPath);
console.log("Next: npx wrangler secret put CONTROL_PLANE_URL && npx wrangler secret put CONTROL_PLANE_BEARER_TOKEN");
console.log("      (use npm run cf:secret:url etc. if you prefer loading bearer from control-plane .env)");
console.log("Then: npm run cf:deploy");
