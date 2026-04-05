import type { TokenResponse } from "./types";

const ACCESS = "hyperspeed_access";
const REFRESH = "hyperspeed_refresh";

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS);
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH);
}

export function setTokens(t: TokenResponse) {
  localStorage.setItem(ACCESS, t.access_token);
  localStorage.setItem(REFRESH, t.refresh_token);
}

export function clearTokens() {
  localStorage.removeItem(ACCESS);
  localStorage.removeItem(REFRESH);
}

function apiBase(): string {
  return (import.meta.env.VITE_API_URL || "").trim();
}

export async function apiFetch(
  path: string,
  init: RequestInit & { json?: unknown } = {},
): Promise<Response> {
  const headers = new Headers(init.headers);
  if (init.json !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  const token = getAccessToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  const body = init.json !== undefined ? JSON.stringify(init.json) : init.body;
  const { json: _j, ...rest } = init;
  let res = await fetch(`${apiBase()}${path}`, { ...rest, headers, body });

  if (res.status === 401 && getRefreshToken()) {
    const r = await fetch(`${apiBase()}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: getRefreshToken() }),
    });
    if (r.ok) {
      const tok = (await r.json()) as TokenResponse;
      setTokens(tok);
      const h2 = new Headers(init.headers);
      if (init.json !== undefined) {
        h2.set("Content-Type", "application/json");
      }
      h2.set("Authorization", `Bearer ${tok.access_token}`);
      res = await fetch(`${apiBase()}${path}`, {
        ...rest,
        headers: h2,
        body: init.json !== undefined ? JSON.stringify(init.json) : init.body,
      });
    }
  }
  return res;
}

export function wsUrl(pathWithQuery: string): string {
  const base = apiBase() || window.location.origin;
  const u = new URL(pathWithQuery, base);
  u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
  return u.toString();
}

/** Parses failed integration API responses (JSON error or plain-text 404 from stale API builds). */
export async function integrationQueryError(res: Response, fallback: string): Promise<Error> {
  const raw = await res.text();
  try {
    const j = JSON.parse(raw) as { error?: string };
    if (j.error) return new Error(j.error);
  } catch {
    // not JSON — e.g. Go's default 404 body when no route exists
  }
  if (res.status === 404 && /404 page not found|Not Found/i.test(raw)) {
    return new Error(
      "This API build is missing integration routes (404). Rebuild and restart the API from apps/api: go build ./cmd/server, or docker compose build api && docker compose up -d api, or restart Air.",
    );
  }
  if (raw.trim()) return new Error(raw.trim());
  return new Error(`${fallback} (HTTP ${res.status})`);
}
