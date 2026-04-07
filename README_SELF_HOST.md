# Hyperspeed — self-hosted Docker

This stack runs Postgres, Redis, MinIO, the Go API, and the static web UI. **Caddy** listens on **HTTP** inside the container. The default compose file maps **host port `18080` → container `80`** so a **first deploy does not need host 80/443** (often already taken on shared hosting). The [`Caddyfile`](Caddyfile) uses an `http://` site block so **any Host** works (localhost, server IP, or a hostname) without extra configuration.

**Important:** Run Compose from the **repository root**—the folder that contains `docker-compose.yml`, `Caddyfile`, `Dockerfile.caddy`, and `apps/`. The API and web images are **built** from `./apps/api` and `./apps/web`; **Caddy** is built from [`Dockerfile.caddy`](Dockerfile.caddy). After editing `Caddyfile`, rebuild: `docker compose up -d --build caddy`.

## Prerequisites

- Docker and Docker Compose v2 (supports `depends_on` health conditions used in `docker-compose.yml`)

## First run (no `.env` required)

From the **repository root**:

```bash
docker compose up --build
```

Open **[http://localhost:18080](http://localhost:18080)** (or **`http://YOUR_SERVER_IP:18080`**). Defaults include **`DEBUG=1`** (permissive CORS for getting started) and placeholder secrets—**before real production use**, copy `.env.example` to `.env`, set a strong **`JWT_SECRET`**, **`HS_SSH_ENCRYPTION_KEY`**, set **`DEBUG=0`**, and set **`CORS_ORIGIN`** / **`PUBLIC_API_BASE_URL`** to your real public HTTPS origin.

**Register** opens the **setup wizard** when the database has no organization yet. Realtime uses WebSockets under `/api/...` (proxied by Caddy).

## VPS, cloud, or hosting panel (paste GitHub URL)

1. **Full repository** — The panel must clone or copy the **entire** repo (build contexts are `./apps/api` and `./apps/web`).
2. **Root directory** — The same folder as `docker-compose.yml`.
3. **No environment variables required** for the stack to start; default **HTTP on the host is port `18080`**. If that port is taken, set **`CADDY_HTTP_PORT`** in `.env` to another free port.
4. **HTTPS** — The stock `Caddyfile` serves **HTTP** only. Put a reverse proxy or load balancer in front for TLS, or replace the `http://` block with a hostname + TLS (see [custom domains](docs/ops/custom-domains-and-subdomains.md)).

### Workspace limit (one organization per database)

The open-source stack allows **at most one organization** in the database. The **first user** creates that workspace **during registration** (wizard); **additional people** join via **invites** or **open registration** with **admin approval** (configurable under workspace settings). If more than one org row exists (for example after a legacy migration), the API will not create additional orgs until only one remains (see server logs for a warning).

## Configuration

| Variable | Role |
|----------|------|
| `JWT_SECRET` | HMAC key for access tokens (required in production) |
| `HS_SSH_ENCRYPTION_KEY` | Base64, 32 bytes—required if you use Terminal SSH storage (see `.env.example`) |
| `DEBUG` | Default `1` for first deploy; set **`0`** in production and tighten CORS. |
| `CORS_ORIGIN` | Default `http://localhost:18080` in compose. When **`DEBUG=0`**, must match the **exact** browser origin (scheme + host + port). **Local Vite:** e.g. `http://localhost:5173` |
| `PUBLIC_APP_URL` | Optional. Public HTTPS URL hint during onboarding (e.g. `https://hyperspeed.example.com`) |
| `PUBLIC_API_BASE_URL` | Same scheme+host users use in the browser when the API must emit absolute URLs (see [custom domains doc](docs/ops/custom-domains-and-subdomains.md)) |
| `CADDY_HTTP_PORT` | Host port mapped to Caddy’s **80** (default **18080**). |
| `CADDY_EMAIL` | Used by Caddy global config; relevant if you add TLS site blocks later |

With Docker, the web image is built with **`VITE_API_URL` empty**; the SPA calls **`/api/...` on the same origin** as the page (via Caddy). When **`DEBUG=0`**, **`CORS_ORIGIN`** must match that origin.

### Hyperspeed-hosted subdomain (optional)

If Hyperspeed operates DNS for **`*.hyperspeedapp.com`**, Hyperspeed runs a **provisioning gateway** (edge service) that verifies **per-install HMAC** and talks to Hyperspeed’s **private control plane**. Your API never receives the control-plane bearer or Cloudflare tokens—only install-scoped credentials Hyperspeed gives you:

| Variable | Role |
|----------|------|
| `PROVISIONING_BASE_URL` | HTTPS origin of the gateway (no path), e.g. `https://provision-gw.hyperspeedapp.com` |
| `PROVISIONING_INSTALL_ID` | Install identifier; Hyperspeed stores the matching secret on the gateway |
| `PROVISIONING_INSTALL_SECRET` | Shared secret used to sign requests to the gateway (not the control-plane bearer) |

When all three are set, `GET /api/v1/public/instance` reports `provisioning_enabled: true` and `provisioning_base_domain: "hyperspeedapp.com"`. Authenticated users may call `POST /api/v1/provisioning/claim` during **first-time setup** (optional) or from workspace settings. Leave these unset if you only use BYO domains or manual DNS.

### Upgrading from older releases

- **`DEPLOYMENT_MODE` removed:** The API no longer reads `DEPLOYMENT_MODE`. Behavior always matches the former **`self_host`** model (single org per database). Remove this variable from `.env` and Docker Compose.
- **Multiple organizations in one database** were only supported when `DEPLOYMENT_MODE=saas`. That mode is removed. Before upgrading, consolidate to **one** `organizations` row per database (export/archive extra orgs or split databases), or the API will warn at startup and refuse to create additional orgs.

### Version metadata and optional update notices

- **Build args** (Docker): pass `VERSION` and `GIT_SHA` when building the API image so `GET /health` and `GET /api/v1/public/instance` report real values (defaults are `dev` / empty). With Compose you can set `HYPERSPEED_VERSION` / `HYPERSPEED_GIT_SHA` in `.env` if your `docker-compose.yml` forwards them as build args (see repository `docker-compose.yml`).
- **Optional UI banner**: set **either** `UPSTREAM_GITHUB_REPO` (`owner/name`) **or** `UPDATE_MANIFEST_URL` (HTTPS JSON manifest). Users must **opt in** on the Dashboard before the browser contacts GitHub or your manifest host.

Details: **[docs/ops/self-host-updates.md](docs/ops/self-host-updates.md)**.

## Domains (production)

Teams **always self-host** the stack. They may use a **custom domain** (recommended when they already have DNS) or, if they have no domain, a **subdomain under hyperspeedapp.com** provisioned by Hyperspeed (DNS points at their server; TLS still runs on their machine). Set `CORS_ORIGIN` and `PUBLIC_API_BASE_URL` to match the public HTTPS origin users open in the browser.

See **[docs/ops/custom-domains-and-subdomains.md](docs/ops/custom-domains-and-subdomains.md)** for BYO vs gifted subdomains, TLS behind Caddy/Traefik, and an end-to-end validation checklist.

## Local development (without Docker for apps)

- Start Postgres and Redis (e.g. `docker compose up postgres redis`).
- Run the API: set `DATABASE_URL` and `REDIS_URL` to localhost and run `go run ./cmd/server` from `apps/api`.
- Run the UI: `npm install` and `npm run dev` in `apps/web`, with `CORS_ORIGIN=http://localhost:5173` on the API.

## Smoke check

With the stack up, **`GET /health`** through Caddy (e.g. `curl -s http://localhost:18080/health`) should return JSON including `"status":"ok"`, plus `version` and `git_sha` when the binary was built with those ldflags (see `apps/api/Dockerfile`).
