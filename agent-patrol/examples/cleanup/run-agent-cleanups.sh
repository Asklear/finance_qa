#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KINDS="${AGENT_PATROL_CLEANUP_KINDS:-}"

if [[ -z "${KINDS//[[:space:]]/}" ]]; then
  echo "$(date -Is) skip agent cleanup: no AGENT_PATROL_CLEANUP_KINDS"
  exit 0
fi

run_one() {
  local kind="$1"
  case "$kind" in
    openclaw)
      "$SCRIPT_DIR/openclaw-jsonl-cleanup.sh"
      ;;
    hermes)
      "$SCRIPT_DIR/hermes-json-cleanup.sh"
      ;;
    claude)
      "$SCRIPT_DIR/claude-session-cleanup.sh"
      ;;
    "")
      ;;
    *)
      echo "$(date -Is) skip unknown cleanup kind=$kind"
      ;;
  esac
}

IFS=',' read -r -a kinds <<< "$KINDS"
for kind in "${kinds[@]}"; do
  run_one "${kind//[[:space:]]/}"
done
