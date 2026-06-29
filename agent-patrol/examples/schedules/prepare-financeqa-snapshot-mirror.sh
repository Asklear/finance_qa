#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$ROOT"

SNAPSHOT="${FINANCEQA_REFERENCE_SNAPSHOT:-}"
MIRROR_OUTPUT="${FINANCEQA_SQLITE_MIRROR_OUTPUT:-}"
SERVICE="${FINANCEQA_MCP_SERVICE:-financeqa-mcp.service}"
BACKUP_DIR="${FINANCEQA_SQLITE_MIRROR_BACKUP_DIR:-tmp/financeqa-sqlite-backups}"

if [[ -z "$SNAPSHOT" ]]; then
  echo "FINANCEQA_REFERENCE_SNAPSHOT is required" >&2
  exit 2
fi
if [[ -z "$MIRROR_OUTPUT" ]]; then
  echo "FINANCEQA_SQLITE_MIRROR_OUTPUT is required" >&2
  exit 2
fi
if [[ ! -f "$SNAPSHOT" ]]; then
  echo "snapshot not found: $SNAPSHOT" >&2
  exit 2
fi

mkdir -p "$(dirname "$MIRROR_OUTPUT")" "$BACKUP_DIR"
next="${MIRROR_OUTPUT}.next"

node examples/golden/financeqa_snapshot_to_sqlite.mjs \
  --snapshot "$SNAPSHOT" \
  --output "$next"

integrity="$(sqlite3 "$next" 'PRAGMA integrity_check;')"
if [[ "$integrity" != "ok" ]]; then
  echo "sqlite integrity check failed: $integrity" >&2
  exit 1
fi

row_count="$(sqlite3 "$next" "SELECT (SELECT COUNT(*) FROM fin_fund_income) + (SELECT COUNT(*) FROM fin_cost_settlements) + (SELECT COUNT(*) FROM fin_file_mappings);")"
if [[ "$row_count" -le 0 ]]; then
  echo "sqlite mirror has no FinanceQA rows" >&2
  exit 1
fi

if [[ -f "$MIRROR_OUTPUT" ]]; then
  backup="$BACKUP_DIR/$(basename "$MIRROR_OUTPUT").$(date +%Y%m%dT%H%M%S).bak"
  cp "$MIRROR_OUTPUT" "$backup"
  echo "$(date -Is) backed up FinanceQA SQLite mirror: $backup"
fi

mv "$next" "$MIRROR_OUTPUT"
echo "$(date -Is) installed FinanceQA SQLite mirror: $MIRROR_OUTPUT rows=$row_count"

if [[ -n "$SERVICE" ]]; then
  systemctl restart "$SERVICE"
  systemctl is-active --quiet "$SERVICE"
  echo "$(date -Is) restarted FinanceQA MCP service: $SERVICE"
fi
