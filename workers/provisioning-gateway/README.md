# Provisioning gateway (Cloudflare Worker)

Public edge for **gifted** `*.hyperspeedapp.com` DNS. Verifies per-install HMAC, applies rate limits, and forwards to the **private** Hyperspeed control plane using `CONTROL_PLANE_BEARER_TOKEN`.

Self-hosted Hyperspeed APIs never receive the control-plane bearer. They use `PROVISIONING_INSTALL_ID` + `PROVISIONING_INSTALL_SECRET` and call this Worker at `PROVISIONING_BASE_URL`.

## Hyperspeed operations

Wrangler loads operator secrets via `scripts/with-cp-env.mjs` from the first file that exists:

1. Path in **`HYPERSPEED_OPERATOR_ENV`** (optional), or  
2. **`workers/provisioning-gateway/.env`** — copy from [`.env.example`](.env.example), or  
3. **`apps/control-plane/.env`** (private monorepo layout only).

**Token shape:** A **zone DNS-only** token is **not** enough for Workers/KV. For `npm run cf:bootstrap` / `cf:deploy`, use a token with at least **Account → Workers Scripts → Edit**, **Workers KV Storage → Edit**, and **User → User Details → Read** (Wrangler calls `/memberships`). The “Edit Cloudflare Workers” API token template is a good starting point. You can set **`CLOUDFLARE_WORKERS_API_TOKEN`** in the same env file if DNS and Workers need different tokens.

1. **One-time KV + `wrangler.toml`:** from `workers/provisioning-gateway`:

   ```bash
   npm run cf:bootstrap
   ```

   This creates two KV namespaces and writes their ids into `wrangler.toml`.

2. **Worker secrets** (same operator env file as above):

   - `WORKER_CONTROL_PLANE_URL` — URL the Worker will `fetch` (no trailing slash).
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

3b. **Bootstrap tokens (optional, one-time exchange):** The API can call `POST /v1/bootstrap` with `Authorization: Bearer <opaque_token>` instead of shipping install ID/secret in plain env. Store a KV entry keyed by `bootstrap:<sha256-hex-of-token>` where the value is JSON:

   `{"provisioning_install_id":"...","provisioning_install_secret":"...","provisioning_base_url":"https://provision.hyperspeedapp.com"}`

   (`provisioning_base_url` is optional; it defaults to the production gateway URL.) The key is **deleted** on successful exchange. Compute the hex digest of the token (SHA-256) when writing the key.

4. **Deploy:**

   ```bash
   npm run cf:deploy
   ```

### Manual Wrangler (any command)

```bash
node scripts/with-cp-env.mjs -- npx wrangler <subcommand> ...
```

## Local dev

1. Run the **control plane** on `http://127.0.0.1:8787` (private monorepo: `docker compose -f docker-compose.yml -f docker-compose.provisioning.yml`, or a remote URL).
2. `npm install && npm run dev` (Worker on port **8789** by default).
3. Put a dev install secret in local KV:

   ```bash
   npx wrangler kv key put --local --binding=INSTALL_SECRETS "install:dev" "your-dev-secret"
   ```

4. Set local secrets (or use `.dev.vars` — do not commit):

   ```
   CONTROL_PLANE_URL=http://127.0.0.1:8787
   CONTROL_PLANE_BEARER_TOKEN=<same as control plane service>
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
