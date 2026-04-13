# Custom domains for deployed Hyperspeed

This document covers hostname setup for Hyperspeed running on infrastructure we control (Docker on a VPS or similar).

## Product rule

Hyperspeed supports **custom domains you control** (for example `app.customer.com`). DNS records are managed at your DNS provider.

## Domain checklist

1. **DNS**  
   Point your hostname to the server running Hyperspeed:
   - `A`/`AAAA` to your public IP, or
   - `CNAME` to another hostname that resolves to that server.

2. **TLS**  
   Terminate HTTPS on your edge (Caddy, Traefik, nginx, or cloud load balancer) with a valid certificate.

3. **Reverse proxy**  
   Route `/api` (including WebSocket upgrades) to the API service and serve the web UI on `/`.

4. **Environment**  
   - Set `CORS_ORIGIN` to the exact browser origin (for example `https://app.customer.com`).
   - Set `PUBLIC_API_BASE_URL` when the API must emit absolute URLs (for example preview URLs).

5. **Web build**  
   Keep `VITE_API_URL` empty for same-origin `/api` calls when web and API share a host.

## Hostinger Docker Manager (Traefik)

[Hostinger’s Traefik template](https://www.hostinger.com/support/connecting-multiple-docker-compose-projects-using-traefik-in-hostinger-docker-manager/) listens on **80/443** and routes by hostname over a shared Docker network (`traefik-proxy`). Hyperspeed ships an optional Compose overlay **[docker-compose.traefik.yml](../../docker-compose.traefik.yml)** that attaches **Caddy** to that network and adds Traefik labels (`websecure` + `letsencrypt`, matching Hostinger’s defaults).

1. Deploy Traefik from Hostinger’s catalog so the shared network exists (often **`traefik-proxy`**; check `docker network ls` and adjust **`docker-compose.traefik.yml`** if the name differs).
2. Set **`HYPERSPEED_TRAEFIK_HOST`** in root **`.env`** to your public FQDN (see **[env.traefik.example](../../env.traefik.example)**).
3. Set **`CORS_ORIGIN`**, **`PUBLIC_API_BASE_URL`**, and **`PUBLIC_APP_URL`** to **`https://<that-hostname>`** for production (`DEBUG=0`).
4. Run: `docker compose -f docker-compose.yml -f docker-compose.traefik.yml up -d --build`

If your Traefik uses different entrypoint or certificate resolver names than **`websecure`** / **`letsencrypt`**, edit the labels in **`docker-compose.traefik.yml`**.

## Reference Caddy host block

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

## Validation playbook

Use this checklist after switching to a public hostname:

| Step | What to verify |
|------|----------------|
| DNS | `dig` / `nslookup` resolves to the expected public IP. |
| TLS | Browser shows a valid certificate for your hostname. |
| Same-origin API | DevTools requests go to `https://<hostname>/api/...` with no CORS failures. |
| Previews | If using preview features, `PUBLIC_API_BASE_URL` is set and preview iframe URLs load. |
| WebSocket | Realtime connects via `/api/v1/organizations/.../ws` over HTTPS/WSS. |
| Health | `GET /health` is reachable through your edge routing. |

Repeat checks after DNS, IP, TLS, or proxy changes.
