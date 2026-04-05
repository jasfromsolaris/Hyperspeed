# ADR: IDE embedded preview — Phase 2 (server-backed)

## Status

Accepted (initial implementation: API + in-process static snapshot; isolated runner is future work).

## Context

Phase 1 renders a single HTML file from the editor in a sandboxed blob iframe. Phase 2 must support **multi-file** previews by serving synced space files over **HTTP**, embeddable in an iframe, without requiring a local machine.

## Decision

1. **REST API** (authenticated, space-scoped):
   - `POST /api/v1/organizations/{orgID}/spaces/{spaceID}/preview/sessions` — create session; body may include `command` / `cwd` for future runners.
   - `GET .../preview/sessions/{sessionID}` — status and `preview_url` (creator-only).
   - `DELETE .../preview/sessions/{sessionID}` — tear down (creator; `files.read` for poll/delete as implemented).

2. **Public content URL** (no `Authorization` header; iframe-compatible):
   - `GET /api/v1/organizations/{orgID}/spaces/{spaceID}/preview/sessions/{sessionID}/content/*?token=...`
   - Token is a per-session secret stored in `preview_sessions.access_token`.

3. **Stub “runner” (current)**  
   On create, the API builds a **static snapshot** of space files (object storage → base64 in `snapshot_json`, size-capped), stores it in Postgres, and serves bytes from the content handler. **No** separate container, venv, or `npm run dev` yet.

4. **Future: real runner + ingress**  
   Replace snapshot-only flow with an isolated process (CPU/RAM/time/network limits), optional **Python venv** and **Node `npm ci`** under `/workspace`, and a reverse-proxied preview hostname. See product plan “IDE embedded preview” for ingress, CSP, and session API hardening.

5. **Frontend**  
   IDE preview panel supports **Editor (instant)** = Phase 1 blob, and **Space snapshot (API)** = Phase 2 URL in a sandboxed iframe.

## Consequences

- `PUBLIC_API_BASE_URL` should be set when the API is behind a different public origin than `Host`, so `preview_url` in JSON is correct for iframes.
- Orphaned sessions expire by TTL (1 hour); optional cron can call `ExpireStalePreviewSessions`.
- Embedding uses `Content-Security-Policy: frame-ancestors *` on the content response for dev friendliness; tighten to known app origins in production.
