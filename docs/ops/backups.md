## Backups (self-host)

Hyperspeed state lives in two places:

- **Postgres**: all application metadata (users/orgs/projects/tasks/chat/file tree, etc.)
- **Object storage** (MinIO by default): the actual file bytes

If you back up **both**, you can restore the system.

### Postgres backup/restore (Docker Compose)

- **Backup**:

```bash
docker compose exec -T postgres pg_dump -U hyperspeed -d hyperspeed > hyperspeed.pg.sql
```

- **Restore** (into a fresh database):

```bash
cat hyperspeed.pg.sql | docker compose exec -T postgres psql -U hyperspeed -d hyperspeed
```

### MinIO / S3 backups

You have two practical options:

- **Back up the MinIO volume** (simple, VM-level snapshots)
  - Stop the stack, snapshot the host disk/volume, start again.
- **Mirror the bucket** (recommended if you want online backups)
  - Use MinIO Client (`mc`) or your provider’s S3 tooling to replicate `hyperspeed-files` to another bucket.

Example (MinIO client) – mirror to another S3 target:

```bash
# Configure aliases (example only)
mc alias set local http://localhost:9000 minioadmin minioadmin
# mc alias set backup https://s3.example.com ACCESS_KEY SECRET_KEY

mc mirror --overwrite local/hyperspeed-files backup/hyperspeed-files
```

### What to test periodically
- You can restore Postgres into a clean environment.
- You can retrieve objects from the backup bucket/volume.

