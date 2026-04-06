# Cursor integration (Hyperspeed)

Internal reference for org-scoped Cursor credentials, chat-backed generation, and optional self-hosted workers. **Cursor’s HTTP surface evolves** — verify paths and auth in the current docs before changing production config.

## Official documentation (verify live)

- [Cursor APIs overview](https://cursor.com/docs/api)
- [Cloud Agents API — endpoints](https://cursor.com/docs/cloud-agent/api/endpoints) (same surface as background-agent docs)
- [Background / Cloud Agent API (overview)](https://docs.cursor.com/en/background-agent/api/overview)
- [API key info](https://docs.cursor.com/en/background-agent/api/api-key-info)
- [Self-hosted cloud agents (blog)](https://cursor.com/blog/self-hosted-cloud-agents)

### Cursor Cloud Agents API (v0) — HTTP paths (live reference)

The **programmatic Cloud Agents** product speaks a **versioned JSON API** under **`/v0`** on the Cursor API host (operators and third-party clients consistently use **`https://api.cursor.com`** as the origin). Paths below are **relative to that origin** (e.g. `GET https://api.cursor.com/v0/me`).

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/v0/me` | API key / caller metadata |
| `GET` | `/v0/models` | Curated list of recommended model ids (API may accept additional model keys) |
| `GET` | `/v0/agents` | List agents (pagination via cursor) |
| `POST` | `/v0/agents` | Launch an agent (repo-backed task) |
| `GET` | `/v0/agents/{id}` | Agent status |
| `GET` | `/v0/agents/{id}/conversation` | Conversation history |
| `POST` | `/v0/agents/{id}/followup` | Follow-up prompt |
| `POST` | `/v0/agents/{id}/stop` | Stop run |
| `DELETE` | `/v0/agents/{id}` | Delete agent |
| `GET` | `/v0/repositories` | List GitHub repos (strict rate limits in practice) |

**Authentication (Cloud Agents v0):** use **HTTP Basic**: API key as **username**, **empty password** (standard `Authorization: Basic base64(key + ":")`). This matches how community clients document the official API (e.g. [cursor-cloud-agent-mcp](https://www.npmjs.com/package/@willpowell8/cursor-cloud-agent-mcp)) and differs from a plain **`Bearer`** header.

**Keys:** created from **Cursor Dashboard → Integrations**; keys often look like `key_…` (format can change).

**Models:** forum guidance from Cursor staff: for “auto” routing use model key **`default`** in the payload where applicable; omitting `model` may resolve from user/team settings rather than “auto” ([forum thread](https://forum.cursor.com/t/cursor-cloud-agents-api-auto-model-not-working/152289)). Treat **`GET /v0/models`** as a curated subset — other keys may still work.

**Important:** Hyperspeed’s chat pipeline today uses an **OpenAI-shaped `POST …/chat/completions`** client so you can point at **Cursor’s HTTP surface** *or* a **local OpenAI-compatible bridge** (e.g. tooling that fronts the Cursor CLI). That is **not** the same wire format as **`POST /v0/agents`** (repo/agent lifecycle). Align **base URL, path, and auth scheme** (`CURSOR_HTTP_AUTH`) with whatever endpoint you actually call.

## Hyperspeed model (v1)

- **Tool auth:** `space.chat.read_recent` requires **`chat.read`**, space membership (via `UserCanAccessSpace`), and the usual **`agent.tools.invoke`** gate on the HTTP/MCP path.
- **REST (org `org.manage`):** `GET|PUT|DELETE /api/v1/organizations/{orgID}/integrations/cursor` (legacy alias: `.../cursor-integration`). After pulling new API code, **rebuild/restart** the API (`docker compose build api && docker compose up -d api`, or restart Air) or the router will return **404**.
- **One API key per organization** (Postgres: encrypted `organizations.cursor_api_key_enc`, display hint `cursor_api_key_hint`).
- **Encryption** uses the same 32-byte AES-GCM key as SSH secret storage: `HS_SSH_ENCRYPTION_KEY` (base64). This key protects **multiple** secret types; a future alias `HS_APP_SECRETS_KEY` could point at the same material.
- **Chat AI staff (Cursor provider):** when a service account’s provider is **Cursor**, Hyperspeed uses **Cursor Cloud Agents v0** (`POST /v0/agents`, poll `GET /v0/agents/{id}`, optional `GET /v0/agents/{id}/conversation`) with **HTTP Basic** auth (API key as username, empty password). This is **not** the OpenAI-shaped `POST …/chat/completions` path on `api.cursor.com` (that legacy client path is not used for Cursor-backed staff).
- **No shared session** between web chat and the IDE; the IDE pulls chat only via explicit tools (`space.chat.read_recent`).
- **Failures**: chat AI replies surface a **visible assistant error** (no silent fallback).
- **Writes**: `space.file.propose_patch` is **not** auto-invoked from the Cursor completion path in v1 — see [Deferred: write approvals](#deferred-write-approvals-phase-3).

## Cursor Cloud vs Hyperspeed

| Concern | Runs in Cursor cloud / Cursor API | Runs in Hyperspeed |
|--------|-----------------------------------|---------------------|
| Model + completion request | Yes (when using Cursor HTTP API) | Builds prompt + optional file/chat context |
| Org API key storage | No | Yes (encrypted) |
| Codebase file listing/reads for chat AI | No | Yes (`agenttools.Harness` as AI staff user) |
| IDE chat context | No | Yes (`space.chat.read_recent` tool) |

## HTTP client contract (env-tunable)

Hyperspeed’s `internal/cursor` client sends **OpenAI-style** chat completion requests so operators can target:

1. **Cursor’s documented HTTP API** (set base URL + path to match current docs), or  
2. A **local OpenAI-compatible bridge** (e.g. community tooling that fronts the Cursor CLI with `POST /v1/chat/completions`).

Environment variables (API server process):

| Variable | Default | Purpose |
|----------|---------|---------|
| `CURSOR_API_BASE_URL` | `https://api.cursor.com` | Scheme + host (+ optional path prefix if your deployment uses one) |
| `CURSOR_CHAT_COMPLETIONS_PATH` | `/v1/chat/completions` | Path appended to base URL (chat-completions-shaped integration only) |
| `CURSOR_COMPLETION_MODEL` | `auto` | `model` field in JSON body (use a concrete id or gateway-specific token if `auto` is rejected) |
| `CURSOR_HTTP_AUTH` | `bearer` | `bearer` → `Authorization: Bearer <key>`; `basic` → HTTP Basic (username = key, password empty), matching **Cloud Agents v0** style |
| `CURSOR_AGENTS_BASE_URL` | *(empty)* | Origin for **Cloud Agents v0** (`/v0/...`). Defaults to **`CURSOR_API_BASE_URL`** when unset. |

**Auth:** default is **Bearer** for OpenAI-compatible gateways. Set **`CURSOR_HTTP_AUTH=basic`** when the upstream expects **Basic** auth (as documented for **Cloud Agents v0** above).

**Timeouts:** the chat AI worker uses a long context timeout (on the order of **25 minutes**) so Cloud Agent runs can poll to completion.

**Note:** Hyperspeed’s **Cursor-backed chat staff** use **`CURSOR_AGENTS_BASE_URL`** + **`/v0/agents`** (Basic auth) regardless of `CURSOR_CHAT_COMPLETIONS_PATH`; the chat-completions env vars apply only if you point other tooling at an OpenAI-compatible bridge.

**Errors:** the client maps `401`/`403` → auth, `429` → rate limit, context deadline → timeout, other non-2xx → upstream (with truncated body message).

## Deferred: write approvals (Phase 3)

Automating `space.file.propose_patch` (or other mutating tools) from a Cursor agent run requires a **Hyperspeed-side approval queue** (pending proposal → user approves in web UI → harness executes) plus **audit logging** (run id, tool name, args hash). This is intentionally **out of v1**; the chat pipeline only performs **read-only** harness calls for context.

## Optional: self-hosted Cursor worker

For teams that run Cursor’s **self-hosted cloud agent worker** next to Hyperspeed:

1. Follow Cursor’s current install/run instructions (CLI `agent`, worker subcommand as documented).
2. Run the worker on a host or sidecar with **outbound HTTPS** to Cursor; no inbound exposure required for the worker registration flow beyond what Cursor documents.
3. Keep **org API keys** in Hyperspeed; the worker is an execution plane — org mapping and secrets remain in the control plane (Hyperspeed API + Postgres).
4. Co-locate with Hyperspeed using the same **Docker Compose** or **Kubernetes** stack: add a `cursor-worker` service / Deployment alongside `api`, `web`, Postgres, Redis, and object storage; inject Cursor-provided env vars for worker identity and registration from Cursor’s docs.

See repository `docker-compose.yml` for the baseline Hyperspeed topology; add the worker container only when using self-hosted agents.
