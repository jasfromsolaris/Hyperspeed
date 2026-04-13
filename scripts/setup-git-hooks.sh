#!/bin/sh
# Point this repo at .githooks for shared local hooks (optional).
# Run once per clone from the repository root:  ./scripts/setup-git-hooks.sh

set -e
cd "$(dirname "$0")/.."
git config core.hooksPath .githooks
echo "core.hooksPath set to .githooks"
