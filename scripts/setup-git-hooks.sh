#!/bin/sh
# Point this repo at .githooks (pre-push blocks accidental push to public OSS Hyperspeed).
# Run once per clone from the repository root:  ./scripts/setup-git-hooks.sh

set -e
cd "$(dirname "$0")/.."
git config core.hooksPath .githooks
echo "core.hooksPath set to .githooks — pre-push protection active for this clone."
