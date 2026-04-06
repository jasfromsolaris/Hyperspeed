#!/usr/bin/env bash
# Sync apps/control-plane into a sibling directory for a small private Git repo (Render, etc.).
# Usage: from repo root: ./scripts/export-control-plane-mirror.sh
# Override destination: CONTROL_PLANE_MIRROR_DST=/path/to/repo ./scripts/export-control-plane-mirror.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/apps/control-plane"
DST="${CONTROL_PLANE_MIRROR_DST:-$ROOT/../hyperspeed-control-plane-private}"
mkdir -p "$DST"
rsync -a --delete \
  --exclude '.env' \
  --exclude 'data/' \
  --exclude '*.sqlite' \
  --exclude 'render.standalone.yaml' \
  --exclude 'README.mirror.md' \
  "$SRC/" "$DST/"
cp "$SRC/render.standalone.yaml" "$DST/render.yaml"
cp "$SRC/README.mirror.md" "$DST/README.md"
echo "Exported to $DST"
echo "Next: cd \"$DST\" && git init && git remote add origin <your-private-repo> && git add -A && git commit -m \"control plane mirror\" && git push -u origin main"
