# Hyperspeed DNS control plane

Hyperspeed-operated service that holds **Cloudflare API credentials** and upserts **A** records for `{slug}.{BASE_DOMAIN}` (default `hyperspeedapp.com`). This binary is **not** part of customer self-host `docker compose`; deploy it only on infrastructure you control.

Put **`CLOUDFLARE_API_TOKEN`** and **`CLOUDFLARE_ZONE_ID`** only here (env vars or secrets on the host that runs this service). Do **not** put them in the open-source API `.env`. Customer stacks call the **provisioning gateway** with `PROVISIONING_BASE_URL` + install credentials; only the gateway (and this service) use `CONTROL_PLANE_BEARER_TOKEN`. See [`.env.example`](.env.example) and [`workers/provisioning-gateway/README.md`](../../workers/provisioning-gateway/README.md).

## Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `CONTROL_PLANE_BEARER_TOKEN` | yes | Bearer the **provisioning gateway Worker** sends to this service (not customer API) |
| `CLOUDFLARE_API_TOKEN` | yes | Zone **DNS Edit** (or sufficient scope to create/update A records) |
| `CLOUDFLARE_ZONE_ID` | yes | Zone ID for `hyperspeedapp.com` |
| `BASE_DOMAIN` | no | Default `hyperspeedapp.com` |
| `HTTP_ADDR` | no | Listen address; default `:8787`. On **Render** / similar hosts, set **`PORT`** (injected) and omit `HTTP_ADDR` — the server binds to `:$PORT`. |
| `PORT` | no | Used when `HTTP_ADDR` is unset (e.g. Render). |
| `AUDIT_DB_PATH` | no | SQLite path for claim audit (default `./data/control-plane.sqlite`) |
| `CLOUDFLARE_PROXIED` | no | Set `true` to orange-cloud records (usually `false` so customer terminates TLS) |
| `WORKER_ADMIN_URL` | no | Worker origin for bootstrap token issuance (default `https://provision-gw.hyperspeedapp.com`) |
| `WORKER_ADMIN_TOKEN` | no | Shared server-to-server token for Worker `POST /v1/admin/bootstrap-token` |
| `PROVISIONING_BASE_URL` | no | Returned in bootstrap responses (default `https://provision-gw.hyperspeedapp.com`) |

## API

- `GET /health` — no auth
- `POST /v1/claims` — JSON `{"slug":"acme","ipv4":"203.0.113.10"}` with Bearer auth
- `DELETE /v1/claims/{slug}` — remove DNS record (Bearer auth)
- `POST /v1/installs/bootstrap-token` — issues one-time bootstrap token for customer API auto-bootstrap (Bearer auth)

## Run locally

```bash
cd apps/control-plane
export CONTROL_PLANE_BEARER_TOKEN=dev-token
export CLOUDFLARE_API_TOKEN=...
export CLOUDFLARE_ZONE_ID=...
go run ./cmd/server
```

## Deploy on Render (private repo)

**Option A — full Hyperspeed monorepo (private):** connect the repo in Render, set **Root Directory** to `apps/control-plane`, or use the Blueprint at repository root [`render.yaml`](../../render.yaml).

**Option B — control-plane-only private repo:** from the Hyperspeed repo root run:

- **Windows:** `.\scripts\export-control-plane-mirror.ps1`
- **macOS/Linux:** `./scripts/export-control-plane-mirror.sh`

This writes a sibling folder `hyperspeed-control-plane-private` (or set `CONTROL_PLANE_MIRROR_DST`) with a copy of this directory plus `render.yaml` from [`render.standalone.yaml`](render.standalone.yaml). Initialize git there, push to your private remote, then create a **Web Service** on Render from that repo.

Set **environment variables** in the Render dashboard for `CONTROL_PLANE_BEARER_TOKEN`, `CLOUDFLARE_API_TOKEN`, and `CLOUDFLARE_ZONE_ID` (Blueprint uses `sync: false` for those). After deploy, set `WORKER_CONTROL_PLANE_URL` in your local `apps/control-plane/.env` to the Render HTTPS URL (no trailing slash) and run `npm run cf:secrets` in `workers/provisioning-gateway`.

`AUDIT_DB_PATH` defaults to `/tmp/control-plane.sqlite` in the Blueprint so SQLite works on Render’s ephemeral disk without an extra volume.

## OSS integration

Customer Hyperspeed API signs requests to the public gateway with `PROVISIONING_INSTALL_ID` and `PROVISIONING_INSTALL_SECRET` (see main project `README_SELF_HOST.md` and `workers/provisioning-gateway/`).
