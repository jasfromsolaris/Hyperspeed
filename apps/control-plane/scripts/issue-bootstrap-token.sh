#!/bin/sh
# Issue a one-time PROVISIONING_BOOTSTRAP_TOKEN for a customer (Hyperspeed operator).
# Loads apps/control-plane/.env when present.
#
# Requires either CONTROL_PLANE_PUBLIC_URL + CONTROL_PLANE_BEARER_TOKEN, or WORKER_ADMIN_TOKEN.
#
# Usage:
#   ./scripts/issue-bootstrap-token.sh
#   TTL_SEC=1800 ./scripts/issue-bootstrap-token.sh
#   INSTALL_ID=my-id ./scripts/issue-bootstrap-token.sh

set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

load_env() {
	[ ! -f "$1" ] && return
	while IFS= read -r line || [ -n "$line" ]; do
		line=$(echo "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
		case "$line" in
		\#*|"") continue ;;
		esac
		key="${line%%=*}"
		val="${line#*=}"
		key=$(echo "$key" | sed 's/[[:space:]]*$//')
		if [ -z "${key}" ]; then continue; fi
		if eval "[ -n \"\${${key}+x}\" ]" 2>/dev/null; then
			if eval "[ -n \"\$${key}\" ]" 2>/dev/null; then continue; fi
		fi
		export "${key}=${val}"
	done < "$1"
}

load_env ".env"

TTL_SEC="${TTL_SEC:-900}"
INSTALL_ID="${INSTALL_ID:-}"

BODY="{\"ttl_sec\":${TTL_SEC}"
if [ -n "$INSTALL_ID" ]; then
	BODY="${BODY},\"install_id\":\"${INSTALL_ID}\""
fi
BODY="${BODY}}"

CP_URL="${CONTROL_PLANE_PUBLIC_URL:-}"
BEARER="${CONTROL_PLANE_BEARER_TOKEN:-}"
WORKER_URL="${WORKER_ADMIN_URL:-https://provision-gw.hyperspeedapp.com}"
WORKER_ADMIN="${WORKER_ADMIN_TOKEN:-}"

if [ -n "$CP_URL" ] && [ -n "$BEARER" ]; then
	URI="${CP_URL%/}/v1/installs/bootstrap-token"
	echo "Calling control plane: $URI" >&2
	RESP=$(curl -sS -X POST "$URI" \
		-H "Authorization: Bearer ${BEARER}" \
		-H "Content-Type: application/json" \
		-d "$BODY")
elif [ -n "$WORKER_ADMIN" ]; then
	URI="${WORKER_URL%/}/v1/admin/bootstrap-token"
	echo "Calling Worker admin: $URI" >&2
	RESP=$(curl -sS -X POST "$URI" \
		-H "Authorization: Bearer ${WORKER_ADMIN}" \
		-H "Content-Type: application/json" \
		-d "$BODY")
else
	echo "Set CONTROL_PLANE_PUBLIC_URL + CONTROL_PLANE_BEARER_TOKEN in .env, or WORKER_ADMIN_TOKEN." >&2
	exit 1
fi

TOKEN=""
if command -v python3 >/dev/null 2>&1; then
	TOKEN=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('provisioning_bootstrap_token',''))" 2>/dev/null || true)
fi
if [ -z "$TOKEN" ]; then
	TOKEN=$(echo "$RESP" | sed -n 's/.*"provisioning_bootstrap_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
fi
if [ -z "$TOKEN" ]; then
	echo "$RESP" >&2
	echo "Could not parse provisioning_bootstrap_token (see response above)." >&2
	exit 1
fi

echo ""
echo "Give the customer this one-time bootstrap token (paste in Workspace settings or PROVISIONING_BOOTSTRAP_TOKEN):"
echo "$TOKEN"
echo ""
