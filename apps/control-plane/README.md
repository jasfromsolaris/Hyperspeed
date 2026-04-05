# Hyperspeed DNS control plane

Hyperspeed-operated service that holds **Cloudflare API credentials** and upserts **A** records for `{slug}.{BASE_DOMAIN}` (default `hyperspeedapp.com`). This binary is **not** part of customer self-host `docker compose`; deploy it only on infrastructure you control.

Put **`CLOUDFLARE_API_TOKEN`** and **`CLOUDFLARE_ZONE_ID`** only here (env vars or secrets on the host that runs this service). Do **not** put them in the open-source API `.env`—customer stacks use `PROVISIONING_BASE_URL` + `CONTROL_PLANE_BEARER_TOKEN` instead. See [`.env.example`](.env.example).

## Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `CONTROL_PLANE_BEARER_TOKEN` | yes | Bearer token clients must send in `Authorization: Bearer ...` |
| `CLOUDFLARE_API_TOKEN` | yes | Zone **DNS Edit** (or sufficient scope to create/update A records) |
| `CLOUDFLARE_ZONE_ID` | yes | Zone ID for `hyperspeedapp.com` |
| `BASE_DOMAIN` | no | Default `hyperspeedapp.com` |
| `HTTP_ADDR` | no | Default `:8787` |
| `AUDIT_DB_PATH` | no | SQLite path for claim audit (default `./data/control-plane.sqlite`) |
| `CLOUDFLARE_PROXIED` | no | Set `true` to orange-cloud records (usually `false` so customer terminates TLS) |

## API

- `GET /health` — no auth
- `POST /v1/claims` — JSON `{"slug":"acme","ipv4":"203.0.113.10"}` with Bearer auth
- `DELETE /v1/claims/{slug}` — remove DNS record (Bearer auth)

## Run locally

```bash
cd apps/control-plane
export CONTROL_PLANE_BEARER_TOKEN=dev-token
export CLOUDFLARE_API_TOKEN=...
export CLOUDFLARE_ZONE_ID=...
go run ./cmd/server
```

## OSS integration

Customer Hyperspeed API can forward claims using `PROVISIONING_BASE_URL` and `CONTROL_PLANE_BEARER_TOKEN` (see main project `README_SELF_HOST.md`).
