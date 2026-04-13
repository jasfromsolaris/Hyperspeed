# Hyperspeed — deployment and operations (Docker)

This stack runs Postgres, Redis, MinIO, the Go API, and the static web UI. **Caddy** listens on **HTTP** inside the container. The default compose file maps **host port `18080` → container `80`** so a first deploy does not require binding host **80/443** (often already used by nginx or another edge proxy). The [`Caddyfile`](Caddyfile) uses an `http://` site block so **any Host** works (localhost, server IP, or a hostname) without extra configuration.

**Important:** Run Compose from the **repository root**—the folder that contains `docker-compose.yml`, `Caddyfile`, `Dockerfile.caddy`, and `apps/`. The API and web images are **built** from `./apps/api` and `./apps/web`; **Caddy** is built from [`Dockerfile.caddy`](Dockerfile.caddy). After editing `Caddyfile`, rebuild: `docker compose up -d --build caddy`.

## Prerequisites

- Docker and Docker Compose v2 (supports `depends_on` health conditions used in `docker-compose.yml`)

## First run (local or server)

From the **repository root**:

```bash
docker compose up --build
```

Open **[http://localhost:18080](http://localhost:18080)** (or **`http://YOUR_SERVER_IP:18080`**). The repo includes a root **[`.env`](.env)** with safe defaults and slots for secrets (no `#` comment lines). **Before production**, set a strong **`JWT_SECRET`**, **`HS_SSH_ENCRYPTION_KEY`**, **`DEBUG=0`**, and **`CORS_ORIGIN`** / **`PUBLIC_API_BASE_URL`** to your real public HTTPS origin. Do **not** commit production secrets to git; use your host’s secret store or CI-injected env.

**Register** opens the **setup wizard** when the database has no organization yet. Realtime uses WebSockets under `/api/...` (proxied by Caddy).

## CI / VPS / panel deploy

Typical flow:

1. **Full repository** — The build needs the **entire** tree (contexts are `./apps/api` and `./apps/web`).
2. **Working directory** — The same folder as `docker-compose.yml`.
3. **GitHub/panel projects** — Keep one stable Compose project for this deployment and avoid Delete actions that remove volumes. The compose file pins volume names (`hyperspeed_pgdata`, `hyperspeed_miniodata`, etc.) so rebuilds and updates reuse the same data volumes.
4. **Ports** — No env vars are strictly required for the stack to start; default **HTTP on the host is port `18080`**. If that port is taken (e.g. by another reverse proxy), set **`CADDY_HTTP_PORT`** in `.env` to another free port and point your edge proxy at it.
5. **HTTPS** — The stock `Caddyfile` serves **HTTP** only. Terminate TLS on nginx, Traefik, or your cloud edge, or add a hostname + TLS block in Caddy (see [custom domains](docs/ops/custom-domains-and-subdomains.md)).

### Root `.env` (single file)

The repository root **[`.env`](.env)** is the env template: `KEY=value` lines only (no `#`), which works with many env-import UIs.

| Variable | Why it may be blank in the template |
|----------|-------------------------------------|
| `JWT_SECRET`, `HS_SSH_ENCRYPTION_KEY` | You must set these for production (Compose still supplies dev defaults if empty—see table below). |
| `UPDATE_MANIFEST_URL` | Optional alternative to `UPSTREAM_GITHUB_REPO` for optional in-app update metadata; leave empty if unused. |

**`UPSTREAM_GITHUB_REPO`** — Optional. If set (`owner/name`), the Dashboard can show optional update hints; users opt in before any outbound request. Leave empty or point at an internal repo if you use this feature. (Legacy templates referenced a public repo name; we no longer treat that as the default story.)

### Workspace limit (one organization per database)

This deployment allows **at most one organization** in the database. The **first user** creates that workspace **during registration** (wizard); **additional people** join via **invites** or **open registration** with **admin approval** (configurable under workspace settings). If more than one org row exists (for example after a legacy migration), the API will not create additional orgs until only one remains (see server logs for a warning).

## Configuration

| Variable | Role |
|----------|------|
| `JWT_SECRET` | HMAC key for access tokens (required in production) |
| `HS_SSH_ENCRYPTION_KEY` | Base64, 32 bytes—required if you use Terminal SSH storage (see root `.env`) |
| `DEBUG` | Default `1` for first deploy; set **`0`** in production and tighten CORS. |
| `CORS_ORIGIN` | Default `http://localhost:18080` in compose. When **`DEBUG=0`**, must match the **exact** browser origin (scheme + host + port). **Local Vite:** e.g. `http://localhost:5173` |
| `PUBLIC_APP_URL` | Optional. Public HTTPS URL hint during onboarding (e.g. `https://app.example.com`) |
| `PUBLIC_API_BASE_URL` | Same scheme+host users use in the browser when the API must emit absolute URLs (see [custom domains doc](docs/ops/custom-domains-and-subdomains.md)) |
| `CADDY_HTTP_PORT` | Host port mapped to Caddy’s **80** (default **18080**). Use **`127.0.0.1:18080`** to bind Caddy only on loopback when using **[docker-compose.traefik.yml](docker-compose.traefik.yml)**. |
| `HYPERSPEED_TRAEFIK_HOST` | FQDN for Traefik `Host()` routing when using **docker-compose.traefik.yml** (no scheme). |
| `CADDY_EMAIL` | Used by Caddy global config; relevant if you add TLS site blocks later |

With Docker, the web image is built with **`VITE_API_URL` empty**; the SPA calls **`/api/...` on the same origin** as the page (via Caddy). When **`DEBUG=0`**, **`CORS_ORIGIN`** must match that origin.

### Upgrading from older releases

- **`DEPLOYMENT_MODE` removed:** The API no longer reads `DEPLOYMENT_MODE`. Behavior always matches the former **`self_host`** model (single org per database). Remove this variable from `.env` and Docker Compose.
- **Multiple organizations in one database** were only supported when `DEPLOYMENT_MODE=saas`. That mode is removed. Before upgrading, consolidate to **one** `organizations` row per database (export/archive extra orgs or split databases), or the API will warn at startup and refuse to create additional orgs.

### Version metadata and optional update notices

- **Build args** (Docker): pass `VERSION` and `GIT_SHA` when building the API image so `GET /health` and `GET /api/v1/public/instance` report real values (defaults are `dev` / empty). With Compose you can set `HYPERSPEED_VERSION` / `HYPERSPEED_GIT_SHA` in `.env` if your `docker-compose.yml` forwards them as build args (see repository `docker-compose.yml`).
- **Optional UI banner**: set **either** `UPSTREAM_GITHUB_REPO` (`owner/name`) **or** `UPDATE_MANIFEST_URL` (HTTPS JSON manifest). Users must **opt in** on the Dashboard before the browser contacts GitHub or your manifest host.

Details: **[docs/ops/self-host-updates.md](docs/ops/self-host-updates.md)**.

## Domains (production)

Use a domain you control and point DNS at your server, then terminate TLS on your edge or reverse proxy. Set `CORS_ORIGIN` and `PUBLIC_API_BASE_URL` to match the public HTTPS origin users open in the browser.

See **[docs/ops/custom-domains-and-subdomains.md](docs/ops/custom-domains-and-subdomains.md)** for DNS, TLS, and validation steps.

### Hostinger Docker Manager + Traefik

If you use Hostinger’s **Traefik** template as the single entry on ports **80/443** ([Hostinger guide](https://www.hostinger.com/support/connecting-multiple-docker-compose-projects-using-traefik-in-hostinger-docker-manager/)):

1. Deploy the Traefik project first so the external Docker network exists (often **`traefik-proxy`**; confirm with `docker network ls` and edit **`docker-compose.traefik.yml`** if yours differs).
2. Add **`HYPERSPEED_TRAEFIK_HOST`** to your root **`.env`** (FQDN only, no `https://`), e.g. `www.team.example.com`. See **[env.traefik.example](env.traefik.example)**.
3. Set **`CORS_ORIGIN`**, **`PUBLIC_API_BASE_URL`**, and optionally **`PUBLIC_APP_URL`** to **`https://<same FQDN>`** (with **`DEBUG=0`** in production).
4. Start Hyperspeed with both compose files:

```bash
docker compose -f docker-compose.yml -f docker-compose.traefik.yml up -d --build
```

Labels in **[docker-compose.traefik.yml](docker-compose.traefik.yml)** assume Traefik entrypoint **`websecure`** and cert resolver **`letsencrypt`** (Hostinger’s defaults). Rename them in that file if your Traefik static config differs.

To avoid exposing Caddy on the public host port, set **`CADDY_HTTP_PORT=127.0.0.1:18080`** in **`.env`** so only Traefik (on the shared Docker network) reaches Caddy; otherwise firewall **18080** if you leave the default mapping.

## Local development (without Docker for apps)

- Start Postgres and Redis (e.g. `docker compose up postgres redis`).
- Run the API: set `DATABASE_URL` and `REDIS_URL` to localhost and run `go run ./cmd/server` from `apps/api`.
- Run the UI: `npm install` and `npm run dev` in `apps/web`, with `CORS_ORIGIN=http://localhost:5173` on the API.

## Smoke check

With the stack up, **`GET /health`** through Caddy (e.g. `curl -s http://localhost:18080/health`) should return JSON including `"status":"ok"`, plus `version` and `git_sha` when the binary was built with those ldflags (see `apps/api/Dockerfile`).

## Backups (do this before upgrades)

State is not only containers. Durable data lives in Postgres + object storage.  
Before upgrades, run a backup and keep an off-server copy:

```bash
./scripts/backup-hyperspeed.sh
```

This writes a Postgres dump and can optionally mirror object storage when `mc` aliases are configured. Full guide: **[docs/ops/backups.md](docs/ops/backups.md)**.
