# Hyperspeed DNS control plane (private mirror)

This repository is a **minimal export** of `apps/control-plane` from the Hyperspeed monorepo for hosting on **Render** or similar. Regenerate from upstream with `scripts/export-control-plane-mirror.ps1` or `.sh` in the main repo.

## Render

1. Create a **Web Service** from this Git repository.
2. Use **Docker**; `Dockerfile` is at the repo root.
3. Set environment variables: `CONTROL_PLANE_BEARER_TOKEN`, `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ZONE_ID` (and optional `BASE_DOMAIN`, `CLOUDFLARE_PROXIED`). Render injects **`PORT`**; do not set `HTTP_ADDR` unless you know you need it.
4. Optional: use [`render.yaml`](render.yaml) as a Blueprint.

After deploy, set **`WORKER_CONTROL_PLANE_URL`** to this service’s public `https://…` URL (no trailing slash) and run `npm run cf:secrets` in the provisioning gateway worker project.

See `.env.example` for variable descriptions.
