# Hyperspeed End Vision (Self‑Hosted, One‑Click Deploy)

This document describes the **north-star goal** for Hyperspeed so that every feature, refactor, and product decision is made with the same destination in mind.

## Goal
- **Open source, self-hosted** by teams on their own infrastructure.
- **“Deploy and chill”**: a small team can run Hyperspeed reliably without a platform team.
- **One-click deployable** on common hosting providers (e.g. VPS dashboards) and container managers.

Non-goals (for now):
- Building a consumer-grade multi-tenant SaaS first.
- Requiring Kubernetes.

## Open source scope
- The **product teams run** is the open-source stack (API, web UI, Postgres, object storage) on **their** infrastructure. That is what belongs in the public repo and default `docker compose` flows. Self-hosting does not depend on any Hyperspeed-operated DNS or side services.

## What “deploy and chill” means
Hyperspeed should be operable by a team that can:
- run `docker compose up -d` (or equivalent),
- set a few secrets,
- point a domain at it,
- and have predictable backups + upgrades.

## Domains and tenancy (summary)
- **Canonical marketing / examples zone:** **`hyperspeedapp.com`** (do not use `hyperspeed.com` in product or docs for that purpose).
- **Marketing** (e.g. landing on `www.hyperspeedapp.com`) is separate from the **application origin** teams use day to day (**bring your own hostname** and DNS). The app always runs on infrastructure **the customer** controls.
- **Open-source stack:** **one organization per database** — there is no separate “SaaS” or multi-tenant deployment mode in the OSS tree; behavior is always single-org-per-Postgres.
- **CEO / first install:** The first account on an empty database runs a **setup wizard** (name, email, password, workspace name, then a **hostname / go-live** step). First-time access is often via `http://localhost` or a LAN IP without public DNS; the product must not hard-block setup until a public FQDN exists. Teams record an optional **intended public URL** for later, then align `CORS_ORIGIN`, TLS, and `PUBLIC_API_BASE_URL` when DNS is ready ([docs/ops/custom-domains-and-subdomains.md](docs/ops/custom-domains-and-subdomains.md)).
- **Returning users:** Sign in or register on the same instance. **Register** does not create a second organization — the singleton org already exists. **Staff access policy:** workspace admins can allow **open sign-ups** (users land in a **pending approval** queue) or turn them off so only **invite links** add people (existing users can always sign in).
- Full DNS, TLS, and `CORS_ORIGIN` / `PUBLIC_API_BASE_URL` guidance: [docs/ops/custom-domains-and-subdomains.md](docs/ops/custom-domains-and-subdomains.md).

## Published architecture (recommended default)

```mermaid
flowchart LR
  UserBrowser[UserBrowser] -->|HTTPS| ReverseProxy[ReverseProxy\nCaddy/Traefik/Nginx]
  ReverseProxy -->|/| Web[Web_UI\n(static + SPA)]
  ReverseProxy -->|/api| Api[API_Service]
  ReverseProxy -->|/ws| Api

  Api -->|SQL| Postgres[(Postgres\npersistent_volume)]
  Api -->|S3_API| Obj[(ObjectStorage\nMinIO_or_S3\npersistent_volume_if_MinIO)]
  UserBrowser -->|PUT/GET via presigned_URL| Obj
```

### Data ownership rules
- **Postgres is the source of truth** for:
  - users/orgs/projects/memberships
  - tasks/boards/chat/file metadata
  - references to file blobs (storage keys)
- **Object storage holds file bytes** (not Postgres).
  - In self-hosted mode, MinIO is a sane default.
  - Teams can swap to AWS S3 / Cloudflare R2 / Backblaze B2 by env vars.

## Dev architecture (current and acceptable)
- Dev can run everything locally via Docker Compose.
- Dev should mirror production semantics:
  - same API paths
  - same “object storage + presigned URL” approach
  - migrations run automatically on API start

## AI agent architecture (decided direction)
### Core decisions
- **AI staff is a roster, not a single bot.** In org settings, admins **create AI staff members** (display name, avatar, etc.). Each member has a **backend** (“powered by”) chosen at creation time—e.g. **Cursor Cloud Agents** vs **OpenRouter**—and that choice stays **visible** wherever the staff member appears (chat, mentions, IDE tooling).
- **Multiple staff members are expected.** For example: one **Cursor-backed** staff member (distinct name) for repo-heavy engineering; several **OpenRouter-backed** staff members, each with a **different model** in the back so teams can prefer different models for different tasks.
- **Org-level API keys (encrypted, admin-rotated):** both **Cursor** and **OpenRouter** (and similar providers) are stored as **organization secrets**, not per-user secrets, unless we introduce a narrower scope later.
- **Cursor-backed staff and GitHub:** when an AI staff member is created with **Cursor** as the backend, the admin configures a **default GitHub repository** (and related defaults as needed) on that staff profile. Cloud Agent flows that require a repo use this binding unless overridden where the product allows.
- **OpenRouter-backed staff** use the org OpenRouter key and **per-staff model configuration** (set in settings) so routing to the right model is explicit.
- Agent workloads run on infrastructure controlled by the workspace (self-hosted deployment model for Hyperspeed; provider-side for Cursor/OpenRouter APIs).
- We do **not** require one shared cross-surface session between web chat and IDE.
- We do require **cross-context awareness** where we claim it:
  - chat can use Hyperspeed tools and space/file context (especially OpenRouter + harness paths),
  - Cursor-backed flows use **GitHub/repo context** per Cursor’s Cloud Agent model,
  - IDE can fetch chat context when explicitly requested (e.g. tools / MCP).

### Surface behavior
- **Chat:**
  - Users @mention **specific AI staff members**; behavior depends on that member’s backend (OpenRouter: fast multi-turn + tools toward Hyperspeed files; Cursor: repo-scoped Cloud Agent runs when that staff member is used and a repo-backed flow applies).
- **IDE:**
  - Remains coding-first; can align with Cursor mode + repo when using Cursor-oriented workflows; OpenRouter-backed use can still combine local files and Hyperspeed MCP where configured.

### Approval and safety model
- Tool use follows a **two-layer approval model**:
  - Hyperspeed-side approval policy for sensitive actions,
  - agent/runtime-side ask/plan confirmation where supported.
- On upstream agent/runtime failures (rate limit, outage, auth), default behavior is to **block and report clearly**.
- Budgets are not part of initial scope; focus on correctness, auditability, and operator control first.

## Operational requirements (must-have for self-hosted)
### Configuration & secrets
- All configuration comes from **environment variables** (12-factor style).
- **Org-stored provider keys** (Cursor, OpenRouter, etc.) are encrypted with the same app secret material as other tenant secrets (e.g. `HS_SSH_ENCRYPTION_KEY` or a documented alias), admin-only rotation in product settings.
- Secrets must be **required and validated** on startup (clear error messages), including:
  - `JWT_SECRET`
  - `HS_SSH_ENCRYPTION_KEY` (base64 32 bytes) for encrypting stored SSH secrets and org provider keys
  - database credentials
  - object store credentials (when not using MinIO defaults)

### Backups
- Clearly documented and easy:
  - Postgres dumps (and restores)
  - object storage backups (or bucket replication)

### Upgrades
- Safe upgrades should be routine:
  - schema migrations are versioned and idempotent
  - release notes call out any breaking config changes

### Security baseline
- TLS termination supported via reverse proxy.
- Least-privilege by default (no “open admin” endpoints).
- Rate limiting and basic abuse controls in place for public-facing deployments.

## Product principles (design constraints)
- **Simple defaults** beat complex optionality.
- Prefer **S3-compatible** primitives over bespoke file systems.
- Prefer **stateless API containers** (scale horizontally) with state in Postgres/Object Storage.
- Avoid features that require local host access (e.g., “run commands on the app machine”) unless explicitly scoped and secured.
- Keep agent integrations **adapter-based** (bring-your-own runtime/provider) with clear contracts.
- Treat context bridging as explicit product behavior: no hidden cross-surface state assumptions.
- Store tenant secrets (e.g., org-level **Cursor** and **OpenRouter** keys) encrypted at rest with strict admin-only rotation.

## One-click deployment targets
Hyperspeed should be easy to package for:
- Docker Compose (primary)
- Portainer templates
- Provider marketplaces that ingest Compose stacks
- Optional: Helm chart (later, not required)

## Definition of done (end vision)
Hyperspeed is “there” when a team can:
- deploy on a single VM in < 10 minutes,
- survive a reboot,
- restore from backups,
- upgrade between versions without data loss,
- and confidently use it for real work.

