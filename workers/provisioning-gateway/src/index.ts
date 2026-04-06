import { verifyInstallHmac } from "./crypto";

export interface Env {
  INSTALL_SECRETS: KVNamespace;
  RATE_LIMITS: KVNamespace;
  CONTROL_PLANE_URL: string;
  CONTROL_PLANE_BEARER_TOKEN: string;
}

const MAX_SKEW_SEC = 300;
const LIMIT_IP_PER_MINUTE = 60;
const LIMIT_POST_PER_INSTALL_HOUR = 15;
const LIMIT_DELETE_PER_INSTALL_HOUR = 10;

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    const url = new URL(req.url);
    const path = url.pathname;

    if (req.method === "GET" && path === "/health") {
      return json(200, { status: "ok" });
    }

    if (!path.startsWith("/v1/")) {
      return json(404, { error: "not_found" });
    }

    const cpBase = (env.CONTROL_PLANE_URL || "").trim().replace(/\/$/, "");
    const cpTok = (env.CONTROL_PLANE_BEARER_TOKEN || "").trim();
    if (!cpBase || !cpTok) {
      return json(503, { error: "gateway_misconfigured" });
    }

    const ip = req.headers.get("CF-Connecting-IP") || "unknown";
    const minute = Math.floor(Date.now() / 60_000);
    const ipOk = await incrementRateLimit(
      env.RATE_LIMITS,
      `ip:${ip}:${minute}`,
      LIMIT_IP_PER_MINUTE,
      120
    );
    if (!ipOk) {
      return json(429, { error: "rate_limited" }, { "Retry-After": "60" });
    }

    const installId = req.headers.get("X-Hyperspeed-Install-Id")?.trim() ?? "";
    const tsStr = req.headers.get("X-Hyperspeed-Timestamp")?.trim() ?? "";
    const sig = req.headers.get("X-Hyperspeed-Signature")?.trim() ?? "";

    if (!installId || !tsStr || !sig) {
      return json(401, { error: "unauthorized" });
    }

    const installSecret = await env.INSTALL_SECRETS.get(`install:${installId}`);
    if (!installSecret) {
      return json(401, { error: "invalid_install" });
    }

    const ts = parseInt(tsStr, 10);
    if (!Number.isFinite(ts)) {
      return json(401, { error: "unauthorized" });
    }
    const nowSec = Math.floor(Date.now() / 1000);
    if (Math.abs(nowSec - ts) > MAX_SKEW_SEC) {
      return json(401, { error: "unauthorized" });
    }

    const bodyBuf =
      req.method === "GET" || req.method === "HEAD" ? new ArrayBuffer(0) : await req.arrayBuffer();

    const macOk = await verifyInstallHmac(installSecret, ts, req.method, path, bodyBuf, sig);
    if (!macOk) {
      return json(401, { error: "unauthorized" });
    }

    const hour = Math.floor(Date.now() / 3_600_000);
    if (req.method === "POST" && path === "/v1/claims") {
      const ok = await incrementRateLimit(
        env.RATE_LIMITS,
        `post:${installId}:${hour}`,
        LIMIT_POST_PER_INSTALL_HOUR,
        7200
      );
      if (!ok) {
        return json(429, { error: "rate_limited" }, { "Retry-After": "3600" });
      }
    }
    if (req.method === "DELETE" && /^\/v1\/claims\/[^/]+$/.test(path)) {
      const ok = await incrementRateLimit(
        env.RATE_LIMITS,
        `del:${installId}:${hour}`,
        LIMIT_DELETE_PER_INSTALL_HOUR,
        7200
      );
      if (!ok) {
        return json(429, { error: "rate_limited" }, { "Retry-After": "3600" });
      }
    }

    const cpUrl = cpBase + path;
    const headers = new Headers();
    headers.set("Authorization", `Bearer ${cpTok}`);
    const ct = req.headers.get("Content-Type");
    if (ct) {
      headers.set("Content-Type", ct);
    } else if (req.method === "POST") {
      headers.set("Content-Type", "application/json");
    }

    const cpResp = await fetch(cpUrl, {
      method: req.method,
      headers,
      body:
        req.method === "GET" || req.method === "HEAD" ? undefined : bodyBuf.slice(0),
    });

    const outHeaders = new Headers();
    const respCt = cpResp.headers.get("Content-Type");
    if (respCt) {
      outHeaders.set("Content-Type", respCt);
    }

    return new Response(cpResp.status === 204 ? null : cpResp.body, {
      status: cpResp.status,
      headers: outHeaders,
    });
  },
};

function json(
  status: number,
  body: Record<string, unknown>,
  extra?: Record<string, string>
): Response {
  const h = new Headers({ "Content-Type": "application/json" });
  if (extra) {
    for (const [k, v] of Object.entries(extra)) {
      h.set(k, v);
    }
  }
  return new Response(JSON.stringify(body), { status, headers: h });
}

/** Returns false if limit exceeded. Otherwise increments counter. */
async function incrementRateLimit(
  kv: KVNamespace,
  key: string,
  limit: number,
  ttlSeconds: number
): Promise<boolean> {
  const raw = await kv.get(key);
  const n = raw ? parseInt(raw, 10) : 0;
  if (!Number.isFinite(n) || n >= limit) {
    return false;
  }
  await kv.put(key, String(n + 1), { expirationTtl: ttlSeconds });
  return true;
}
