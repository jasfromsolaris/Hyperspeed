## Backups (Docker deployments)

Hyperspeed state lives in two places:

- **Postgres**: workspace metadata (users, orgs, projects, tasks, chat, file tree, etc.)
- **Object storage** (MinIO by default): raw file bytes (attachments and file contents)

For a full restore, back up both.

## Quick start (recommended)

From the repository root on the server:

```bash
./scripts/backup-hyperspeed.sh
```

This script always creates a Postgres dump (`.sql.gz`) and can optionally mirror object storage if `mc` is installed and aliases are configured.

Useful environment variables:

- `BACKUP_DIR`: output directory for SQL backups (default `./backups`)
- `RETAIN_COUNT`: keep only the newest N SQL backups (example: `RETAIN_COUNT=14`)
- `BUCKET_NAME`: object bucket to mirror (default `hyperspeed-files`)
- `MC_SOURCE_ALIAS` / `MC_TARGET_ALIAS`: enable optional object mirror (`mc mirror`)

Windows/Docker Desktop operators can use:

```powershell
.\scripts\backup-hyperspeed.ps1
```

## Manual backup and restore (Postgres)

- **Backup**:

```bash
docker compose exec -T postgres pg_dump -U hyperspeed -d hyperspeed > hyperspeed.pg.sql
```

- **Restore** (into a fresh database):

```bash
cat hyperspeed.pg.sql | docker compose exec -T postgres psql -U hyperspeed -d hyperspeed
```

## Object storage backups (MinIO / S3)

You have two practical options:

- **Volume snapshot** (simple)
  - Stop the stack, snapshot the host disk/volume, start again.
- **Bucket mirror** (recommended for online backups)
  - Mirror `hyperspeed-files` to a second bucket/location.

Example (`mc`) mirror to a backup target:

```bash
# Configure aliases (example only)
mc alias set local http://localhost:9000 minioadmin minioadmin
# mc alias set backup https://s3.example.com ACCESS_KEY SECRET_KEY

mc mirror --overwrite local/hyperspeed-files backup/hyperspeed-files
```

## Scheduling

Daily cron example (2:30 UTC) on Linux:

```bash
30 2 * * * cd /opt/hyperspeed && BACKUP_DIR=/var/backups/hyperspeed RETAIN_COUNT=14 ./scripts/backup-hyperspeed.sh >> /var/log/hyperspeed-backup.log 2>&1
```

If your hosting panel supports scheduled commands, run the same script from the repo root.

## Off-server copies (strongly recommended)

Backups on the same server are not enough for disaster recovery. Copy backups to another location:

- `scp` / `rsync` to your PC or NAS
- `rclone` / object replication to another cloud bucket (B2, S3, etc.)

Treat dumps as sensitive data. Use encryption at rest and restrict access.

## Restore checklist

1. Restore Postgres dump into the target deployment.
2. Restore object bucket (`hyperspeed-files`) from your mirror/snapshot.
3. Start stack and validate health (`/health` through Caddy).
4. Verify a small sample of files opens correctly in the UI.

## Test restores periodically

At least monthly:

- Restore a recent SQL backup into a throwaway database/container.
- Confirm expected org/users/projects are present.
- Retrieve a few files from the restored bucket copy.

Evidence of successful restore tests is more valuable than backup logs alone.

## Notes on other data stores

- **Redis**: ephemeral in the default compose file (no named volume); do not treat as canonical backup source.
- **`gitwork` volume**: stores local clone/workdir state for tooling. Usually optional for disaster recovery, but you can snapshot it if warm clone state matters to you.

## Safe teardown reminder

Normal updates (`docker compose up -d --build`) keep named volumes.  
Avoid destructive teardown in production:

- Safe: `docker compose down` (without `-v`)
- Destructive: `docker compose down -v`, `docker volume rm`, `docker volume prune`

