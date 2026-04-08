# Hyperspeed

Self-hosted team workspace: board, chat, files, IDE-style editing, and automations—packaged as a **Docker Compose** stack you run on your own infrastructure.

- **Quick start:** use the **full repository root**, then **`docker compose up --build`** (open **http://localhost:18080**). Configuration is in the repo root **[`.env`](.env)** (comment-free `KEY=value` lines for Docker Compose and hosting panels). Details in **[README_SELF_HOST.md](README_SELF_HOST.md)**.
- **Vision and product boundaries:** see **[VISION.md](VISION.md)**.
- **Domains and TLS:** **[docs/ops/custom-domains-and-subdomains.md](docs/ops/custom-domains-and-subdomains.md)**.

This repository is the open-source application (API, web UI, Postgres, Redis, MinIO, Caddy) for self-hosted deployments using domains you control. See [README_SELF_HOST.md](README_SELF_HOST.md).

## License

[MIT](LICENSE)
