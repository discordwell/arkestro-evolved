#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_BASE_URL="${EVO_API_BASE_URL:-http://127.0.0.1:8080}"
SMOKE_EMAIL="${EVO_SMOKE_EMAIL:-admin@evo.local}"
SMOKE_PASSWORD="${EVO_SMOKE_PASSWORD:-changeme}"
STAMP="$(date +%Y%m%d%H%M%S)"
WORKSPACE_NAME="${EVO_SMOKE_WORKSPACE_NAME:-Smoke Ops ${STAMP}}"
WORKSPACE_SLUG="${EVO_SMOKE_WORKSPACE_SLUG:-smoke-${STAMP}}"

export XDG_CONFIG_HOME
XDG_CONFIG_HOME="$(mktemp -d "${TMPDIR:-/tmp}/evo-smoke.XXXXXX")"

cleanup() {
  (
    cd "$ROOT_DIR"
    EVO_API_BASE_URL="$API_BASE_URL" node packages/cli/dist/index.js auth logout --json >/dev/null 2>&1
  ) || true
  rm -rf "$XDG_CONFIG_HOME"
}

trap cleanup EXIT

cd "$ROOT_DIR"

if [[ ! -f packages/cli/dist/index.js ]]; then
  pnpm --filter @evo/sdk build >/dev/null
  pnpm --filter @evo/cli build >/dev/null
fi

echo "Using temporary XDG_CONFIG_HOME=$XDG_CONFIG_HOME"

EVO_API_BASE_URL="$API_BASE_URL" node packages/cli/dist/index.js auth login \
  --email "$SMOKE_EMAIL" \
  --password "$SMOKE_PASSWORD" \
  --json

EVO_API_BASE_URL="$API_BASE_URL" node packages/cli/dist/index.js auth whoami --json

WORKSPACE_JSON="$(
  EVO_API_BASE_URL="$API_BASE_URL" node packages/cli/dist/index.js workspace create \
    --name "$WORKSPACE_NAME" \
    --slug "$WORKSPACE_SLUG" \
    --description "ephemeral smoke workspace" \
    --json
)"

echo "$WORKSPACE_JSON"

WORKSPACE_ID="$(
  printf '%s' "$WORKSPACE_JSON" | node -e 'let data="";process.stdin.on("data", (chunk) => data += chunk);process.stdin.on("end", () => process.stdout.write(JSON.parse(data).id));'
)"

echo "Smoke test created workspace $WORKSPACE_ID and will remove local auth state on exit."
