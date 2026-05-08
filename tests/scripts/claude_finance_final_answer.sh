#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <question>" >&2
  exit 2
fi

QUESTION="$*"

FINANCEQA_BIN="${FINANCEQA_BIN:-$ROOT_DIR/bin/financeqa}" \
FINANCEQA_SKILL_PATH="${FINANCEQA_SKILL_PATH:-$ROOT_DIR/SKILL.md}" \
FINANCEQA_APPENDIX_PATH="${FINANCEQA_APPENDIX_PATH:-$ROOT_DIR/docs/SKILL_APPENDIX_FULL.md}" \
QUESTION="$QUESTION" python3 - <<'PY'
import json
import os
import subprocess
import sys

question = os.environ["QUESTION"]
financeqa_bin = os.environ["FINANCEQA_BIN"]
skill_path = os.environ["FINANCEQA_SKILL_PATH"]
appendix_path = os.environ["FINANCEQA_APPENDIX_PATH"]
requests = [
    {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}},
    {
        "jsonrpc": "2.0",
        "id": 2,
        "method": "tools/call",
        "params": {"name": "finance-query", "arguments": {"query": question}},
    },
]

proc = subprocess.run(
    [financeqa_bin, "serve", "--skill", skill_path, "--appendix", appendix_path],
    input="\n".join(json.dumps(request, ensure_ascii=False) for request in requests) + "\n",
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    universal_newlines=True,
)
if proc.returncode != 0:
    sys.stderr.write(proc.stderr or proc.stdout)
    sys.exit(proc.returncode)

tool_result = None
for line in proc.stdout.splitlines():
    if not line.strip():
        continue
    message = json.loads(line)
    if message.get("id") == 2:
        tool_result = message.get("result")
        break

if not isinstance(tool_result, dict):
    sys.stderr.write("MCP response did not contain tools/call result\n")
    sys.exit(1)

payload = tool_result
content = tool_result.get("content")
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
    sys.stderr.write("MCP response did not contain final_answer/boss_reply_text/message\n")
    sys.exit(1)

sys.stdout.write(answer)
sys.stdout.write("\n")
PY
