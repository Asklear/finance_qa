#!/usr/bin/env bash
set -euo pipefail

SESSION_DIR="${AGENT_PATROL_CLAUDE_SESSION_DIR:-/root/.claude/sessions}"
RETENTION_DAYS="${AGENT_PATROL_SESSION_RETENTION_DAYS:-3}"

if [[ ! "$RETENTION_DAYS" =~ ^[0-9]+$ ]]; then
  echo "$(date -Is) skip Claude cleanup: invalid retention_days=$RETENTION_DAYS"
  exit 0
fi
if [[ ! -d "$SESSION_DIR" ]]; then
  echo "$(date -Is) skip Claude cleanup: missing session_dir=$SESSION_DIR"
  exit 0
fi

find_patrol_files() {
  find "$SESSION_DIR" -maxdepth 1 -type f \( -name 'patrol-*.jsonl' -o -name 'patrol-*.json' \) -mtime +"$RETENTION_DAYS" -print
}

if [[ "${AGENT_PATROL_CLEANUP_DRY_RUN:-0}" == "1" ]]; then
  count="$(find_patrol_files | wc -l | tr -d ' ')"
  echo "$(date -Is) dry-run Claude cleanup count=$count retention_days=$RETENTION_DAYS session_dir=$SESSION_DIR"
  exit 0
fi

deleted="$(find_patrol_files | while IFS= read -r file; do rm -f -- "$file" && printf '.\n'; done | wc -l | tr -d ' ')"
echo "$(date -Is) cleaned Claude sessions deleted=$deleted retention_days=$RETENTION_DAYS session_dir=$SESSION_DIR"
