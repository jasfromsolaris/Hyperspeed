## Upgrades (self-host)

### How schema migrations work
The API runs database migrations automatically on startup. Migrations are stored in:
- `apps/api/internal/migrate/*.up.sql`

### Recommended upgrade flow (Docker Compose)
1) **Back up Postgres and object storage** (see `docs/ops/backups.md`).
2) Pull/build the new images:

```bash
docker compose pull
docker compose up -d --build
```

3) Check logs and health:

```bash
docker compose logs -f api
docker compose ps
```

### Rollback guidance
- If you need to roll back, prefer restoring from backups taken before the upgrade.
- Avoid partial rollbacks (e.g. rolling back only the API but not the DB) unless you know the migrations are backward compatible.

