# Provisioning gateway (Cloudflare Worker)

Public edge for **gifted** `*.hyperspeedapp.com` DNS. Verifies per-install HMAC, applies rate limits, and forwards to the **private** Hyperspeed control plane (`apps/control-plane`) using `CONTROL_PLANE_BEARER_TOKEN`.

Self-hosted Hyperspeed APIs never receive the control-plane bearer. They use `PROVISIONING_INSTALL_ID` + `PROVISIONING_INSTALL_SECRET` and call this Worker at `PROVISIONING_BASE_URL`.

## Hyperspeed operations

Use `CLOUDFLARE_API_TOKEN` from `apps/control-plane/.env`. Wrangler loads it via `scripts/with-cp-env.mjs` (no manual export).

**Token shape:** The **zone DNS-only** token used by `apps/control-plane` for Cloudflare DNS is **not** enough for Workers/KV. For `npm run cf:bootstrap` / `cf:deploy`, use a separate token (or broaden the existing one) with at least **Account → Workers Scripts → Edit**, **Workers KV Storage → Edit**, and **User → User Details → Read** (Wrangler calls `/memberships`). The “Edit Cloudflare Workers” API token template is a good starting point. You can keep two tokens in `.env` if you prefer: e.g. `CLOUDFLARE_API_TOKEN` for DNS (control plane) and `CLOUDFLARE_WORKERS_API_TOKEN` for Wrangler — see below.

1. **One-time KV + `wrangler.toml`:** from `workers/provisioning-gateway`:

   ```bash
   npm run cf:bootstrap
   ```

   This creates two KV namespaces and writes their ids into `wrangler.toml`.

2. **Worker secrets** (also sourced from `apps/control-plane/.env`):

   - Add `WORKER_CONTROL_PLANE_URL` there (URL the Worker will `fetch`, e.g. tunnel or public `https://…` to the control plane — no trailing slash).
   - `CONTROL_PLANE_BEARER_TOKEN` must match the control plane.

   Then:

   ```bash
   npm run cf:secrets
   ```

3. For each customer install, store the **same** secret the customer has in `PROVISIONING_INSTALL_SECRET`:

   ```bash
   npm run cf:whoami   # sanity check auth
   npx wrangler kv key put --binding=INSTALL_SECRETS "install:<PROVISIONING_INSTALL_ID>" "<secret>"
   ```

   (Prefix `node scripts/with-cp-env.mjs --` if you need the API token on the `kv key put` command.)

4. **Deploy:**

   ```bash
   npm run cf:deploy
   ```

### Manual Wrangler (any command)

```bash
node scripts/with-cp-env.mjs -- npx wrangler <subcommand> ...
```

## Local dev (with Docker control plane)

1. Run control plane (e.g. `docker compose -f docker-compose.yml -f docker-compose.provisioning.yml up`).
2. `npm install && npm run dev` (Worker on port **8789** by default).
3. Put a dev install secret in local KV:

   ```bash
   npx wrangler kv key put --local --binding=INSTALL_SECRETS "install:dev" "your-dev-secret"
   ```

4. Set local secrets (or use `.dev.vars` — do not commit):

   ```
   CONTROL_PLANE_URL=http://127.0.0.1:8787
   CONTROL_PLANE_BEARER_TOKEN=<same as apps/control-plane/.env>
   ```

5. Point the API at `http://host.docker.internal:8789` (from Docker) or `http://127.0.0.1:8789` (native API).

## Signing contract

Headers on every proxied request:

- `X-Hyperspeed-Install-Id`
- `X-Hyperspeed-Timestamp` — Unix seconds
- `X-Hyperspeed-Signature` — hex HMAC-SHA256 of  
  `{timestamp}\\n{METHOD}\\n{path}\\n{sha256Hex(body)}`  
  keyed by the install secret. `path` is the URL path (e.g. `/v1/claims`, `/v1/claims/acme`). Empty body uses SHA-256 of empty input.

Implemented in Go in `apps/api/internal/provisioning/gateway_hmac.go` (must stay in sync with `src/crypto.ts`).

## Tests

```bash
npm test
```
