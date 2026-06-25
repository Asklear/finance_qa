#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$DEFAULT_ROOT"

ENV_FILE="${AGENT_PATROL_ENV_FILE:-examples/schedules/financeqa-daily.env}"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

ROOT="${AGENT_PATROL_ROOT:-$DEFAULT_ROOT}"
cd "$ROOT"

if [[ "${AGENT_PATROL_LIVE:-}" != "1" ]]; then
  echo "Refusing dry-run: set AGENT_PATROL_LIVE=1 after manual validation." >&2
  exit 2
fi

: "${OPENCLAW_AGENT_CMD:?OPENCLAW_AGENT_CMD is required}"
: "${FINANCEQA_MCP_URL:?FINANCEQA_MCP_URL is required}"

if [[ -z "${FINANCEQA_MCP_READ_TOKEN:-}" && -n "${FINANCEQA_MCP_READ_TOKEN_FILE:-}" && -r "$FINANCEQA_MCP_READ_TOKEN_FILE" ]]; then
  FINANCEQA_MCP_READ_TOKEN="$(<"$FINANCEQA_MCP_READ_TOKEN_FILE")"
  export FINANCEQA_MCP_READ_TOKEN
fi

: "${FINANCEQA_MCP_READ_TOKEN:?FINANCEQA_MCP_READ_TOKEN or FINANCEQA_MCP_READ_TOKEN_FILE is required}"

OUT_BASE="${AGENT_PATROL_OUTPUT_DIR:-tmp/financeqa-dry-run}"
SUITE="${AGENT_PATROL_SUITE:-smoke}"
SEED="${AGENT_PATROL_SEED:-$(date +%F-%H%M)}"
RUN_ID="${AGENT_PATROL_RUN_ID:-$(date +%Y%m%dT%H%M%S)}"
LOG_FILE="${AGENT_PATROL_LOG_FILE:-$OUT_BASE/dry-run.log}"
LOCK_FILE="${AGENT_PATROL_LOCK_FILE:-$OUT_BASE/.financeqa-dry-run.lock}"

mkdir -p "$OUT_BASE"

if command -v flock >/dev/null 2>&1; then
  exec 9>"$LOCK_FILE"
  if ! flock -n 9; then
    echo "$(date -Is) another FinanceQA dry-run is already running; skip" >> "$LOG_FILE"
    exit 0
  fi
fi

{
  echo "$(date -Is) start FinanceQA dry-run suite=$SUITE seed=$SEED"
  npm run start -- run \
    --config presets/financeqa.yaml \
    --suite "${AGENT_PATROL_SUITE:-smoke}" \
    --seed "$SEED" \
    --out "$OUT_BASE/$RUN_ID"
  echo "$(date -Is) finish FinanceQA dry-run suite=$SUITE seed=$SEED out=$OUT_BASE/$RUN_ID"
} >> "$LOG_FILE" 2>&1
