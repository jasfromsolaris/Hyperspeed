# OpenRouter integration (Hyperspeed)

Org-scoped OpenRouter API keys power **OpenRouter-backed AI staff** in chat: when a service account’s provider is **OpenRouter**, the chat worker calls OpenRouter’s **OpenAI-compatible** `POST /api/v1/chat/completions` endpoint using the org key.

## Official references

- [OpenRouter API](https://openrouter.ai/docs) — models and authentication
- [Chat completions](https://openrouter.ai/docs/api-reference/chat-completion) — request shape

## Hyperspeed model

- **REST (org `org.manage`):** `GET|PUT|DELETE /api/v1/organizations/{orgID}/integrations/openrouter` (legacy alias: `.../openrouter-integration`). Same encryption pattern as other org secrets (`HS_SSH_ENCRYPTION_KEY`).
- **Per staff member:** service accounts set **provider** `openrouter` and an **openrouter_model** (e.g. `openai/gpt-4o-mini`). The worker sends that model id in the JSON body.
- **Failures:** assistant-visible errors only (no silent fallback).

## Environment variables (API process)

| Variable | Default | Purpose |
|----------|---------|---------|
| `OPENROUTER_API_BASE_URL` | `https://openrouter.ai/api/v1` | Scheme + host + API prefix |
| `OPENROUTER_CHAT_COMPLETIONS_PATH` | `/chat/completions` | Path appended to the base URL |

Auth is always **`Authorization: Bearer <org key>`** (OpenRouter’s documented style).

## Troubleshooting

- **`GET .../integrations/openrouter` returns 404** with body `404 page not found`: the running API binary was built **before** OpenRouter routes existed. Rebuild and restart (`apps/api`: `go build -o ./tmp/main ./cmd/server`, Docker: `docker compose build api`, or restart Air). After deploy, `GET /healthz` should include `"dual_provider_ai_staff":true` in the JSON.

## Operations

1. Create an API key in the [OpenRouter dashboard](https://openrouter.ai/).
2. As an org admin, open **Workspace settings** and paste the key under **OpenRouter integration**.
3. Under **AI staff (service accounts)**, create or edit a member with provider **OpenRouter** and a concrete **model id** matching OpenRouter’s catalog.
