# AI Agent QA Matrix

This matrix validates IDE Ask/Plan/Agent behavior across actor types and workspace states.

## Preconditions

- API and web are running.
- A test org + space exist.
- At least one top-level project folder exists in space.
- At least one text file exists under that folder.

## Matrix

| Mode | Actor | Folder selected | File open | Expected result |
|---|---|---:|---:|---|
| Ask | Human | No | No | Send disabled; folder-first guardrail message shown |
| Ask | Human | Yes | No | `list_files` path works; no write affordances |
| Ask | Human | Yes | Yes | `file.read` path works; edit-intent prompt gets read-only hint |
| Ask | Service account | Yes | Yes | Read/list only; no direct apply UI |
| Plan | Human | Yes | No | Planning read/list responses; no write affordances |
| Plan | Human | Yes | Yes | Read/list only; edit-intent prompt blocked with guidance |
| Agent | Human | Yes | No | List/read works |
| Agent | Human | Yes | Yes | Proposal card can be created; direct-apply optional + confirm gated |
| Agent | Service account | Yes | Yes | Proposal can be created; direct apply not shown |

## Policy Tests (Backend)

1. Invoke `space.file.propose_patch` with `mode=ask` -> `403` with `code=mode_policy`.
2. Invoke `space.file.propose_patch` with `mode=plan` -> `403` with `code=mode_policy`.
3. Invoke `space.file.read` with `mode=ask` -> success (assuming auth/resource access).
4. Invoke invalid mode -> `400` with `code=invalid_mode`.

## UX / Recovery Tests

1. Force transient network failure during invoke -> retry banner appears.
2. Retry button resends last prompt and clears failure on success.
3. Panel mode selection persists after refresh (per space).
4. Panel open/closed state persists after refresh (per space).

## MCP Path Smoke

1. Start MCP process with env vars set.
2. Call `tools/list` -> tool list returns.
3. Call `tools/call` with `_hyperspeed.mode=ask` metadata -> mode reaches API.
4. Expired/invalid token -> actionable auth error message.

## Single organization per database

1. Register user A, create the workspace, confirm `GET /api/v1/organizations` returns `can_create_organization: false` for subsequent requests.
2. Register user B (no org), confirm dashboard hides “Create workspace” and B can join only via invite.

## Self-host with public hostname (BYO domain)

Preconditions: instance reachable at a public HTTPS origin; `CORS_ORIGIN` and `PUBLIC_API_BASE_URL` aligned with that origin per [custom-domains-and-subdomains.md](custom-domains-and-subdomains.md).

1. DNS resolves the hostname to the expected IP.
2. TLS: valid cert in browser for that hostname.
3. Login works; no CORS errors on `/api` calls (same origin).
4. WebSocket: realtime path loads (e.g. open a space that uses org WS).
5. IDE preview (if used): preview iframe loads with `PUBLIC_API_BASE_URL` set.

Full step-by-step table: [custom-domains-and-subdomains.md](custom-domains-and-subdomains.md#validation-playbook-e2e-smoke).

## OpenRouter staff (chat mention) tools

Preconditions: org OpenRouter key configured; AI staff uses an OpenRouter model that supports `tools`; `OPENROUTER_CHAT_TOOLS_ENABLED` left at default (on) or explicitly true.

1. Mention OpenRouter staff with a question that only needs chat context -> assistant replies (tool loop may run zero user-defined tool calls).
2. Ask to summarize a known space file by name -> model uses `space.file.read` or list+read; reply references file content.
3. Ask for an edit to an existing file -> model uses `space.file.propose_patch`; proposal appears in the web UI for acceptance.
4. Ask to add a new text file -> model uses `space.file.create_text`; file appears in the space tree.
5. Ask for current events / web grounding -> with `OPENROUTER_WEB_SEARCH_TOOL` enabled, reply is grounded (watch OpenRouter usage/cost).
6. Set `OPENROUTER_CHAT_TOOLS_ENABLED=false` -> mention still works via plain completion (no Hyperspeed tool loop).
7. Optional: valid `OPENROUTER_PLUGINS_JSON` array -> server starts; spot-check one mention (account-level OpenRouter plugin defaults may also apply).
