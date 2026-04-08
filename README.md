# Hyperspeed

Self-hosted team workspace: board, chat, files, IDE-style editing, and automations—packaged as a **Docker Compose** stack you run on your own infrastructure.

- **Quick start:** use the **full repository root**, then **`docker compose up --build`** (defaults work without a `.env` file; open **http://localhost:18080**). For **hosting panels**, import **[`deploy.env`](deploy.env)** (comment-free); for local secrets use a gitignored **`.env`**. Details in **[README_SELF_HOST.md](README_SELF_HOST.md)**.
- **Vision and product boundaries:** see **[VISION.md](VISION.md)**.
- **Domains, TLS, and optional gifted `*.hyperspeedapp.com` DNS:** **[docs/ops/custom-domains-and-subdomains.md](docs/ops/custom-domains-and-subdomains.md)**.

This repository is the open-source application (API, web UI, Postgres, Redis, MinIO, Caddy). Hyperspeed-operated DNS for company-provided subdomains uses a **separate private control plane** that is **not** shipped here. One-click customer linking uses a one-time **`PROVISIONING_BOOTSTRAP_TOKEN`** (with optional **`PROVISIONING_BASE_URL`** override), then persists install-scoped credentials on first API boot. See [README_SELF_HOST.md](README_SELF_HOST.md).

## License

[MIT](LICENSE)
