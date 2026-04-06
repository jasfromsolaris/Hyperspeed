#!/usr/bin/env node
/**
 * Ensures provision-gw.hyperspeedapp.com has a proxied DNS record so the Worker route resolves.
 * Uses CLOUDFLARE_API_TOKEN + CLOUDFLARE_ZONE_ID from apps/control-plane/.env
 */
import { config } from "dotenv";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const cpEnv = path.join(__dirname, "..", "..", "..", "apps", "control-plane", ".env");
config({ path: cpEnv, quiet: true });

const token = String(process.env.CLOUDFLARE_API_TOKEN || "").trim();
const zoneId = String(process.env.CLOUDFLARE_ZONE_ID || "").trim();
if (!token || !zoneId) {
  console.error("Missing CLOUDFLARE_API_TOKEN or CLOUDFLARE_ZONE_ID in apps/control-plane/.env");
  process.exit(1);
}

const base = `https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records`;
const headers = { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };

const name = "provision-gw.hyperspeedapp.com";
const listRes = await fetch(`${base}?name=${encodeURIComponent(name)}&type=AAAA`, { headers });
const list = await listRes.json();
if (!list.success) {
  console.error("DNS list failed:", list.errors || list);
  process.exit(1);
}
if (list.result?.length > 0) {
  console.log("DNS already present:", name, list.result[0].id);
  process.exit(0);
}

const body = {
  type: "AAAA",
  name: "provision-gw",
  content: "100::",
  proxied: true,
  ttl: 1,
  comment: "Hyperspeed provisioning gateway Worker",
};

const createRes = await fetch(base, { method: "POST", headers, body: JSON.stringify(body) });
const created = await createRes.json();
if (!created.success) {
  console.error("DNS create failed:", created.errors || created);
  process.exit(1);
}
console.log("Created DNS record:", name, created.result?.id);
