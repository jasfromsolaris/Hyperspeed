# Hyperspeed — self-hosted Docker

This stack runs Postgres, Redis, the Go API, and the static web UI (nginx proxies `/api` to the API so the browser uses a single origin).

## Prerequisites

- Docker and Docker Compose

## Quick start

1. Copy `.env.example` to `.env` and set `JWT_SECRET` to a long random string (at least 32 characters).

2. From the repository root:

   ```bash
   docker compose up --build
   ```

3. Open the app at [http://localhost:3000](http://localhost:3000). The API is reachable directly at [http://localhost:8080](http://localhost:8080) (for debugging).

4. **Register** opens the **setup wizard** when the database has no organization yet: you create the **singleton workspace** (org) as the first admin in one flow (name, email, password, workspace name, then hostname / go-live notes). After that, add a space (project) and use the board. If an org already exists, **Register** only requests access (pending admin approval when open sign-ups are enabled) or use an **invite** — you do not create another workspace from the dashboard. Realtime updates use WebSockets on `/api/v1/organizations/{orgId}/ws` (proxied through nginx when you use port 3000).

### Workspace limit (one organization per database)

The open-source stack allows **at most one organization** in the database. The **first user** creates that workspace **during registration** (wizard); **additional people** join via **invites** or **open registration** with **admin approval** (configurable under workspace settings). If more than one org row exists (for example after a legacy migration), the API will not create additional orgs until only one remains (see server logs for a warning).

## Configuration

| Variable | Role |
|----------|------|
| `JWT_SECRET` | HMAC key for access tokens (required in production) |
| `CORS_ORIGIN` | Browser origin allowed for direct API calls (e.g. local Vite on port 5173) |
| `PUBLIC_APP_URL` | Optional. Public HTTPS origin users will use (e.g. `https://hyperspeed.example.com`). Shown as a hint during onboarding; set when DNS is stable. |

When the UI is served behind nginx with `VITE_API_URL` empty at build time, the SPA calls `/api/...` on the same host, so CORS is not required for that path.

### Hyperspeed-hosted subdomain (optional)

If Hyperspeed operates DNS for **`*.hyperspeedapp.com`**, Hyperspeed runs a **provisioning gateway** (edge service) that verifies **per-install HMAC** and talks to Hyperspeed’s **private control plane**. Your API never receives the control-plane bearer or Cloudflare tokens—only install-scoped credentials Hyperspeed gives you:

| Variable | Role |
|----------|------|
| `PROVISIONING_BASE_URL` | HTTPS origin of the gateway (no path), e.g. `https://provision.hyperspeedapp.com` |
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

With the API up, `GET /health` should return JSON including `"status":"ok"`, plus `version` and `git_sha` when the binary was built with those ldflags (see `apps/api/Dockerfile`).
