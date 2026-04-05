# Hyperspeed

Self-hosted team workspace: board, chat, files, IDE-style editing, and automations—packaged as a **Docker Compose** stack you run on your own infrastructure.

- **Quick start:** copy [`.env.example`](.env.example) to `.env`, set secrets, then run `docker compose up --build`. Full steps are in **[README_SELF_HOST.md](README_SELF_HOST.md)**.
- **Vision and product boundaries:** see **[VISION.md](VISION.md)**.
- **Domains, TLS, and optional gifted `*.hyperspeedapp.com` DNS:** **[docs/ops/custom-domains-and-subdomains.md](docs/ops/custom-domains-and-subdomains.md)**.

This repository is the open-source application (API, web UI, Postgres, Redis, MinIO, Caddy). Hyperspeed-operated DNS for company-provided subdomains uses a separate control plane that is **not** shipped here; the API can still integrate via `PROVISIONING_BASE_URL` and `CONTROL_PLANE_BEARER_TOKEN` when Hyperspeed provides those endpoints for your team.

## License

[MIT](LICENSE)
