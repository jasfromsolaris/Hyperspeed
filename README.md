# Hyperspeed

Self-hosted team workspace: board, chat, files, IDE-style editing, and automations—packaged as a **Docker Compose** stack you run on your own infrastructure.

- **Quick start:** use the **full repository root**, then **`docker compose up --build`** (defaults work without a `.env` file; open **http://localhost:18080**). Production hardening and ports are in **[README_SELF_HOST.md](README_SELF_HOST.md)**.
- **Vision and product boundaries:** see **[VISION.md](VISION.md)**.
- **Domains, TLS, and optional gifted `*.hyperspeedapp.com` DNS:** **[docs/ops/custom-domains-and-subdomains.md](docs/ops/custom-domains-and-subdomains.md)**.

This repository is the open-source application (API, web UI, Postgres, Redis, MinIO, Caddy). Hyperspeed-operated DNS for company-provided subdomains uses a **separate private control plane** that is **not** shipped here. When Hyperspeed provides a provisioning gateway for your team, configure the API with **`PROVISIONING_BASE_URL`**, **`PROVISIONING_INSTALL_ID`**, and **`PROVISIONING_INSTALL_SECRET`** (never the control-plane bearer or Cloudflare tokens on the customer stack). See [README_SELF_HOST.md](README_SELF_HOST.md).

## License

[MIT](LICENSE)
