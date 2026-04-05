#!/usr/bin/env bash
# Live-reload API (Air) + DEBUG logs. Requires: Go, Air, Docker (Postgres + Redis).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if ! command -v air >/dev/null 2>&1; then
  echo "Air not found. Install with: go install github.com/air-verse/air@latest"
  exit 1
fi

echo "Starting postgres + redis..."
docker compose up -d postgres redis

export DEBUG="${DEBUG:-true}"
export DATABASE_URL="${DATABASE_URL:-postgres://hyperspeed:hyperspeed@localhost:5432/hyperspeed?sslmode=disable}"
export REDIS_URL="${REDIS_URL:-redis://localhost:6379/0}"
if [ -z "${HS_GIT_WORKDIR_BASE:-}" ]; then
  export HS_GIT_WORKDIR_BASE="${TMPDIR:-/tmp}/hyperspeed-git"
  mkdir -p "$HS_GIT_WORKDIR_BASE"
fi

cd "$ROOT/apps/api"
echo "Air watching Go files (DEBUG=true). Ctrl+C to stop."
exec air
