#!/usr/bin/env bash
set -euo pipefail

# Sync finance bridge script + SKILL.md + appendix to server, then wire OpenClaw symlinks.
# Defaults match current production host; can be overridden via env vars.

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SERVER="${SERVER:-root@8.129.14.124}"
KEY_PATH="${KEY_PATH:-$HOME/Downloads/未命名文件夹 2/lzh-key.pem}"

LOCAL_SKILL="$ROOT_DIR/SKILL.md"
LOCAL_APPENDIX="$ROOT_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md"
LOCAL_BRIDGE="$ROOT_DIR/plugin/openclaw-finance/server/finance_bridge.py"

REMOTE_REPO_DIR="${REMOTE_REPO_DIR:-/root/finance_qa}"
REMOTE_OPENCLAW_EXT_DIR="${REMOTE_OPENCLAW_EXT_DIR:-/root/.openclaw/extensions/openclaw-finance/server}"
REMOTE_OPENCLAW_SKILL_DIR="${REMOTE_OPENCLAW_SKILL_DIR:-/root/.openclaw/skills/finance}"
REMOTE_REPO_BRIDGE_DIR="${REMOTE_REPO_BRIDGE_DIR:-$REMOTE_REPO_DIR/plugin/openclaw-finance/server}"

if [[ ! -f "$LOCAL_SKILL" ]]; then
  echo "missing local SKILL.md: $LOCAL_SKILL" >&2
  exit 1
fi
if [[ ! -f "$LOCAL_APPENDIX" ]]; then
  echo "missing local appendix: $LOCAL_APPENDIX" >&2
  exit 1
fi
if [[ ! -f "$LOCAL_BRIDGE" ]]; then
  echo "missing local bridge file: $LOCAL_BRIDGE" >&2
  exit 1
fi

echo "[1/5] upload SKILL.md to ${SERVER}:${REMOTE_REPO_DIR}/SKILL.md"
scp -i "$KEY_PATH" "$LOCAL_SKILL" "$SERVER:$REMOTE_REPO_DIR/SKILL.md"

echo "[2/5] upload appendix to ${SERVER}:${REMOTE_REPO_DIR}/docs/SKILL_APPENDIX_FULL_2026-04-15.md"
ssh -i "$KEY_PATH" "$SERVER" "mkdir -p '$REMOTE_REPO_DIR/docs'"
scp -i "$KEY_PATH" "$LOCAL_APPENDIX" "$SERVER:$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md"

echo "[3/5] upload finance_bridge.py to ${SERVER}:${REMOTE_REPO_BRIDGE_DIR}/finance_bridge.py"
ssh -i "$KEY_PATH" "$SERVER" "mkdir -p '$REMOTE_REPO_BRIDGE_DIR'"
scp -i "$KEY_PATH" "$LOCAL_BRIDGE" "$SERVER:$REMOTE_REPO_BRIDGE_DIR/finance_bridge.py"

echo "[4/5] create OpenClaw skills/bridge symlinks to repo files"
ssh -i "$KEY_PATH" "$SERVER" "set -e; \
  mkdir -p '$REMOTE_OPENCLAW_EXT_DIR'; \
  mkdir -p '$REMOTE_OPENCLAW_SKILL_DIR'; \
  mkdir -p '$REMOTE_OPENCLAW_SKILL_DIR/docs'; \
  ln -sfn '$REMOTE_REPO_BRIDGE_DIR/finance_bridge.py' '$REMOTE_OPENCLAW_EXT_DIR/finance_bridge.py'; \
  ln -sfn '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md'; \
  ln -sfn '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md'; \
  chmod 444 '$REMOTE_REPO_DIR/SKILL.md'"

echo "[5/5] verify skill path and bridge candidates on server"
ssh -i "$KEY_PATH" "$SERVER" "set -e; \
  ls -l '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md' '$REMOTE_REPO_BRIDGE_DIR/finance_bridge.py' '$REMOTE_OPENCLAW_EXT_DIR/finance_bridge.py'; \
  grep -n 'SKILL_APPENDIX_FULL_2026-04-15.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md'; \
  python3 '$REMOTE_OPENCLAW_EXT_DIR/finance_bridge.py' <<< '{\"action\":\"list\"}' >/tmp/finance_bridge_list.json; \
  cat /tmp/finance_bridge_list.json; \
  sed -n '1,30p' '$REMOTE_OPENCLAW_EXT_DIR/finance_bridge.py'"

echo "done."
