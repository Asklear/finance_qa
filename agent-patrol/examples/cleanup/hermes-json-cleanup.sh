#!/usr/bin/env bash
set -euo pipefail

SESSION_DIR="${AGENT_PATROL_HERMES_SESSION_DIR:-/root/.hermes/sessions}"
RETENTION_DAYS="${AGENT_PATROL_SESSION_RETENTION_DAYS:-3}"
PATTERN="${AGENT_PATROL_HERMES_SESSION_PATTERN:-patrol-*.json}"

if [[ ! "$RETENTION_DAYS" =~ ^[0-9]+$ ]]; then
  echo "$(date -Is) skip Hermes cleanup: invalid retention_days=$RETENTION_DAYS"
  exit 0
fi
if [[ ! -d "$SESSION_DIR" ]]; then
  echo "$(date -Is) skip Hermes cleanup: missing session_dir=$SESSION_DIR"
  exit 0
fi

if [[ "${AGENT_PATROL_CLEANUP_DRY_RUN:-0}" == "1" ]]; then
  count="$(find "$SESSION_DIR" -maxdepth 1 -type f -name "$PATTERN" -mtime +"$RETENTION_DAYS" -print | wc -l | tr -d ' ')"
  echo "$(date -Is) dry-run Hermes cleanup count=$count pattern=$PATTERN retention_days=$RETENTION_DAYS session_dir=$SESSION_DIR"
  exit 0
fi

deleted="$(find "$SESSION_DIR" -maxdepth 1 -type f -name "$PATTERN" -mtime +"$RETENTION_DAYS" -print -delete | wc -l | tr -d ' ')"
echo "$(date -Is) cleaned Hermes sessions deleted=$deleted pattern=$PATTERN retention_days=$RETENTION_DAYS session_dir=$SESSION_DIR"
