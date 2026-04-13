#!/usr/bin/env bash
# Backup Hyperspeed Docker data: Postgres dump (required) + optional object mirror.
# Run from anywhere: ./scripts/backup-hyperspeed.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"
BUCKET_NAME="${BUCKET_NAME:-hyperspeed-files}"
RETAIN_COUNT="${RETAIN_COUNT:-}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="$BACKUP_DIR/hyperspeed-${STAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"

echo "Writing Postgres backup to: $OUT_FILE"
docker compose exec -T postgres pg_dump -U hyperspeed -d hyperspeed | gzip -c > "$OUT_FILE"

BYTES="$(wc -c < "$OUT_FILE" | tr -d '[:space:]')"
echo "Postgres backup complete (${BYTES} bytes)"

if [ -n "$RETAIN_COUNT" ]; then
  if [[ "$RETAIN_COUNT" =~ ^[0-9]+$ ]]; then
    mapfile -t backups < <(ls -1 "$BACKUP_DIR"/hyperspeed-*.sql.gz 2>/dev/null | sort -r)
    if [ "${#backups[@]}" -gt "$RETAIN_COUNT" ]; then
      for old in "${backups[@]:$RETAIN_COUNT}"; do
        rm -f "$old"
        echo "Pruned old backup: $old"
      done
    fi
  else
    echo "RETAIN_COUNT is not numeric; skipping prune"
  fi
fi

if command -v mc >/dev/null 2>&1 && [ -n "${MC_SOURCE_ALIAS:-}" ] && [ -n "${MC_TARGET_ALIAS:-}" ]; then
  echo "Mirroring object storage bucket: ${MC_SOURCE_ALIAS}/${BUCKET_NAME} -> ${MC_TARGET_ALIAS}/${BUCKET_NAME}"
  mc mirror --overwrite "${MC_SOURCE_ALIAS}/${BUCKET_NAME}" "${MC_TARGET_ALIAS}/${BUCKET_NAME}"
  echo "Object mirror complete"
else
  echo "Object mirror skipped. Set MC_SOURCE_ALIAS and MC_TARGET_ALIAS (and configure mc aliases) to enable."
fi

echo "Done. Restore Postgres with:"
echo "  gunzip -c \"$OUT_FILE\" | docker compose exec -T postgres psql -U hyperspeed -d hyperspeed"
