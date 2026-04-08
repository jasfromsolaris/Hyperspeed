# Custom domains for self-hosted Hyperspeed

This document covers public hostname setup for teams that run Hyperspeed on their own infrastructure (Docker on a VPS or similar).

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
