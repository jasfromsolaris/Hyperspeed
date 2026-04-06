# Custom domains and Hyperspeed-provided subdomains (self-host only)

This document describes how **public hostnames** work for teams that **always run Hyperspeed on their own infrastructure** (Docker on a VPS or equivalent). Hyperspeed Inc does not host customer application stacks.

**Canonical zone:** Use **`hyperspeedapp.com`** for Hyperspeed-operated examples (marketing site, gifted subdomains like `acme.hyperspeedapp.com`). Do not use **`hyperspeed.com`** in docs or product copy; it is a different domain.

## Product rules

| Rule | Detail |
|------|--------|
| Hosting | The customer always runs the Hyperspeed stack (API, web, Postgres, object storage, etc.). |
| Bring-your-own (BYO) domain | Supported and recommended when the team already controls DNS (e.g. `app.customer.com`). |
| Gifted subdomain | Optional: e.g. `acme.hyperspeedapp.com` **only** when the customer does not have (or does not want to use) their own domain. DNS under `hyperspeedapp.com` points at **their** public IP. |

## Marketing site vs application origin

- **Marketing** (e.g. Framer) may live at `www.hyperspeedapp.com` or the domain apex. That site is **not** the Hyperspeed app.
- **Application** is whatever HTTPS origin users open to use Hyperspeed: either a **BYO hostname** or a **gifted subdomain** that resolves to the customer’s reverse proxy.

The SPA is designed to call `/api/...` on the **same origin** when built without a separate API URL (see [README_SELF_HOST.md](../../README_SELF_HOST.md)). The browser’s address bar origin must match how you configure the API (see [Configuration](#configuration) below).

## BYO domain checklist

1. **DNS**  
   Point the hostname at the server: typically an **A** (or **AAAA**) record to the server’s public IP, or a **CNAME** to another hostname that ultimately resolves to that server.

2. **TLS**  
   Terminate HTTPS on the customer’s edge (Caddy, Traefik, or nginx) with a valid certificate (Let’s Encrypt HTTP-01 or DNS-01 is typical).

3. **Reverse proxy**  
   Route `/api` (including WebSocket upgrades) to the API service and serve the static web UI on `/`. The repository [Caddyfile](../../Caddyfile) shows a minimal **HTTP** example on `:80` for local Compose; **production** should use a real hostname block with HTTPS, for example:

   ```caddy
   app.customer.com {
       encode gzip
       handle /api/* {
           reverse_proxy api:8080
       }
       handle /health {
           reverse_proxy api:8080
       }
       handle {
           reverse_proxy web:80
       }
   }
   ```

   Adjust service names and ports to match your Compose network.

4. **Configuration (environment variables)**  
   - **`CORS_ORIGIN`**: Set to the **exact** public origin users type in the browser, e.g. `https://app.customer.com` (scheme + host + port if non-default). The API reads this from [`apps/api/internal/config/config.go`](../../apps/api/internal/config/config.go).  
   - **`PUBLIC_API_BASE_URL`**: Set when the API must emit absolute URLs that match the public API origin (for example IDE **preview** iframe URLs). If the reverse proxy exposes the API at `https://app.customer.com/api`, set `PUBLIC_API_BASE_URL=https://app.customer.com` (no trailing path; paths are appended as `/api/...`). See [docs/adr/ide-preview-phase2.md](../adr/ide-preview-phase2.md).

5. **Web build**  
   For same-origin deployment, build the web app so the browser uses relative `/api` (e.g. `VITE_API_URL` empty when served behind the same host as in [README_SELF_HOST.md](../../README_SELF_HOST.md)).

## Gifted subdomain (e.g. `acme.hyperspeedapp.com`)

**Mechanics**

1. Customer deploys Hyperspeed and has a **stable public IP** (or updates DNS when it changes).
2. Hyperspeed (the company) creates a DNS **A** record: `acme.hyperspeedapp.com` → that IP. TLS is still obtained on **the customer’s server** (HTTP-01 to that IP for that hostname), because HTTPS terminates there.
3. Customer configures **`CORS_ORIGIN`** and **`PUBLIC_API_BASE_URL`** using `https://acme.hyperspeedapp.com` (same rules as BYO).

### Security boundary (non-negotiable)

- **Cloudflare API tokens** for the `hyperspeedapp.com` zone must live **only** on infrastructure Hyperspeed operates: the **control-plane** service (see below). They **must not** appear in customer `.env` for the open-source stack.
- The **control-plane bearer token** must not appear in customer `.env` either. Hyperspeed runs a **provisioning gateway** (Cloudflare Worker) that holds that bearer and talks to the private control plane. The self-hosted API uses **`PROVISIONING_INSTALL_ID`** + **`PROVISIONING_INSTALL_SECRET`** to sign requests to the gateway (scoped to that install).

### Hyperspeed-operated gateway and control plane

Hyperspeed runs a **provisioning gateway** (public edge) that validates install HMAC headers, rate-limits requests, and forwards `POST /v1/claims` and `DELETE /v1/claims/{slug}` to a **private control plane** that holds Cloudflare credentials. Customer stacks never see the control-plane bearer or zone tokens—only **`PROVISIONING_BASE_URL`**, **`PROVISIONING_INSTALL_ID`**, and **`PROVISIONING_INSTALL_SECRET`** on the self-hosted API. Gateway and control-plane source are **not** part of this open-source repository.

### OSS API integration

When the self-hosted API is configured with **`PROVISIONING_BASE_URL`**, **`PROVISIONING_INSTALL_ID`**, and **`PROVISIONING_INSTALL_SECRET`**, it exposes:

- **`GET /api/v1/public/instance`** — `provisioning_enabled`, and when provisioning is enabled `provisioning_base_domain` is always `hyperspeedapp.com` (gifted DNS is `*.hyperspeedapp.com`). No secrets in the response.
- **`POST /api/v1/provisioning/claim`** (authenticated) — signs and forwards `slug` and `ipv4` to `{PROVISIONING_BASE_URL}/v1/claims`. Stable error codes include `invalid_slug`, `invalid_ipv4`, `slug_taken`, `rate_limited`, `provisioning_unavailable`.
- **`PATCH /api/v1/organizations/{orgId}`** (org `manage`) — same as today for `intended_public_url`, plus optional **`provision_gifted_dns`**: when `true`, include **`public_ipv4`**. The server calls the gateway **first** (same as `POST .../provisioning/claim`), then saves `intended_public_url` only if the claim succeeds. The intended URL must be **`https://{slug}.hyperspeedapp.com`** for this path. This keeps a single “Save & create DNS” action in **Workspace settings** without exposing Cloudflare tokens.

The OSS stack **never** stores Cloudflare zone credentials or the control-plane bearer.

See [README_SELF_HOST.md](../../README_SELF_HOST.md).

### Operator or manual DNS

If provisioning is **not** configured, use a **manual or internal** process: customer requests a subdomain; operator verifies IP and slug, creates the **A** record, and points them to this document for TLS and env vars. The technical steps match the automated path once DNS exists.

### What stays out of the OSS repo (conceptual)

- **Wildcard** `*.hyperspeedapp.com` is optional for operations. A single hostname with a per-host certificate on the customer edge is enough.

### Future work (security)

Verify the customer controls the claimed IP (e.g. HTTP challenge, short-lived proof) to reduce **subdomain takeover** risk. Not required for the first iteration of automation.

---

## Validation playbook (E2E smoke)

Use this checklist after pointing a **public hostname** (BYO or gifted) at a self-hosted instance.

| Step | What to verify |
|------|----------------|
| **DNS** | `dig` / `nslookup`: the app hostname resolves to the expected public IP. |
| **TLS** | Browser shows a valid certificate for that hostname (no mixed-content or cert name warnings for the app origin). |
| **App same-origin** | Log in; open DevTools Network: API calls go to `https://<hostname>/api/...` with no CORS failures. |
| **Previews** | If using IDE Phase 2 preview: `PUBLIC_API_BASE_URL` is set; create a preview session and confirm the iframe loads (see [ide-preview-phase2.md](../adr/ide-preview-phase2.md)). |
| **WebSocket** | Open a space with realtime features; connection to `/api/v1/organizations/.../ws` succeeds (wss behind HTTPS). |
| **Health** | API exposes `GET /health` on the API process (JSON `{"status":"ok"}`). Ensure your edge proxy forwards `/health` to the API if you want it on the public hostname (the sample Caddyfile does this); the default web container only proxies `/api/` unless you add a route. |

Repeat after **any** change to DNS, IP, or TLS termination.
