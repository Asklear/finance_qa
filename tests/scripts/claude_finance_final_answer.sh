#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
BRIDGE_PATH="${FINANCE_BRIDGE_PATH:-}"
if [[ -z "$BRIDGE_PATH" ]]; then
  BRIDGE_PATH="$ROOT_DIR/plugin/openclaw-finance/server/finance_bridge.py"
fi

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <question>" >&2
  exit 2
fi

QUESTION="$*"

FINANCEQA_BIN="${FINANCEQA_BIN:-$ROOT_DIR/financeqa}" \
FINANCEQA_SKILL_PATH="${FINANCEQA_SKILL_PATH:-$ROOT_DIR/SKILL.md}" \
QUESTION="$QUESTION" BRIDGE_PATH="$BRIDGE_PATH" python3 - <<'PY'
import json
import os
import subprocess
import sys

question = os.environ["QUESTION"]
bridge_path = os.environ["BRIDGE_PATH"]
request = {
    "action": "call",
    "name": "finance-query",
    "arguments": {"query": question},
}

proc = subprocess.run(
    ["python3", bridge_path],
    input=json.dumps(request, ensure_ascii=False),
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    universal_newlines=True,
)
if proc.returncode != 0:
    sys.stderr.write(proc.stderr or proc.stdout)
    sys.exit(proc.returncode)

outer = json.loads(proc.stdout)
payload = outer
content = outer.get("content") if isinstance(outer, dict) else None
if isinstance(content, list):
    for item in content:
        if isinstance(item, dict) and item.get("type") == "text" and isinstance(item.get("text"), str):
            payload = json.loads(item["text"])
            break

answer = ""
if isinstance(payload, dict):
    for key in ("final_answer", "boss_reply_text", "message"):
        value = payload.get(key)
        if isinstance(value, str) and value.strip():
            answer = value.strip()
            break
    if not answer and isinstance(payload.get("boss_reply"), dict):
        answer = "\n\n".join(
            str(payload["boss_reply"].get(key) or "").strip()
            for key in ("结论", "原因", "建议")
            if str(payload["boss_reply"].get(key) or "").strip()
        )

if not answer:
    sys.stderr.write("bridge response did not contain final_answer/boss_reply_text/message\n")
    sys.exit(1)

sys.stdout.write(answer)
sys.stdout.write("\n")
PY
