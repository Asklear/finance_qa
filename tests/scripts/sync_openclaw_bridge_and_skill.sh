#!/usr/bin/env bash
set -euo pipefail

# Sync Go MCP binary + SKILL.md + appendix + OpenClaw plugin runtime to server.
# Defaults match current production host; can be overridden via env vars.

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SERVER="${SERVER:-root@8.129.14.124}"
KEY_PATH="${KEY_PATH:-$HOME/Downloads/未命名文件夹 2/lzh-key.pem}"
REMOTE_HOME="${REMOTE_HOME:-$(ssh -i "$KEY_PATH" "$SERVER" 'printf %s "$HOME"')}"

LOCAL_SKILL="$ROOT_DIR/SKILL.md"
LOCAL_APPENDIX="$ROOT_DIR/docs/SKILL_APPENDIX_FULL.md"
LOCAL_PLUGIN_DIST="$ROOT_DIR/plugin/openclaw-finance/dist/index.esm.js"
LOCAL_PLUGIN_MANIFEST="$ROOT_DIR/plugin/openclaw-finance/openclaw.plugin.json"
LOCAL_PLUGIN_PACKAGE="$ROOT_DIR/plugin/openclaw-finance/package.json"
LOCAL_CLAUDE_WRAPPER="$ROOT_DIR/tests/scripts/claude_finance_final_answer.sh"
LOCAL_ONLINE_CHECKER="$ROOT_DIR/tests/scripts/run_online_agent_final_answer_check.py"

REMOTE_REPO_DIR="${REMOTE_REPO_DIR:-$REMOTE_HOME/finance_qa}"
REMOTE_OPENCLAW_PLUGIN_DIR="${REMOTE_OPENCLAW_PLUGIN_DIR:-$REMOTE_HOME/.openclaw/extensions/openclaw-finance}"
REMOTE_OPENCLAW_SKILL_DIR="${REMOTE_OPENCLAW_SKILL_DIR:-$REMOTE_HOME/.openclaw/skills/finance}"
REMOTE_OPENCLAW_EXT_SKILL_DIR="${REMOTE_OPENCLAW_EXT_SKILL_DIR:-$REMOTE_HOME/.openclaw/extensions/openclaw-finance/skills/finance}"
REMOTE_CLAUDE_SKILL_DIR="${REMOTE_CLAUDE_SKILL_DIR:-$REMOTE_HOME/.claude/skills/finance}"
REMOTE_OPENCLAW_SESSION_STORE="${REMOTE_OPENCLAW_SESSION_STORE:-$REMOTE_HOME/.openclaw/agents/main/sessions/sessions.json}"
REMOTE_OPENCLAW_CONFIG_PATH="${REMOTE_OPENCLAW_CONFIG_PATH:-$REMOTE_HOME/.openclaw/openclaw.json}"

if [[ ! -f "$LOCAL_SKILL" ]]; then
  echo "missing local SKILL.md: $LOCAL_SKILL" >&2
  exit 1
fi
if [[ ! -f "$LOCAL_APPENDIX" ]]; then
  echo "missing local appendix: $LOCAL_APPENDIX" >&2
  exit 1
fi
if [[ ! -f "$LOCAL_PLUGIN_DIST" ]]; then
  echo "missing local OpenClaw plugin runtime: $LOCAL_PLUGIN_DIST" >&2
  exit 1
fi
if [[ ! -f "$LOCAL_PLUGIN_MANIFEST" ]]; then
  echo "missing local OpenClaw plugin manifest: $LOCAL_PLUGIN_MANIFEST" >&2
  exit 1
fi
if [[ ! -f "$LOCAL_PLUGIN_PACKAGE" ]]; then
  echo "missing local OpenClaw plugin package: $LOCAL_PLUGIN_PACKAGE" >&2
  exit 1
fi
if [[ ! -f "$LOCAL_CLAUDE_WRAPPER" ]]; then
  echo "missing local Claude finance wrapper: $LOCAL_CLAUDE_WRAPPER" >&2
  exit 1
fi
if [[ ! -f "$LOCAL_ONLINE_CHECKER" ]]; then
  echo "missing local online agent checker: $LOCAL_ONLINE_CHECKER" >&2
  exit 1
fi

echo "[1/7] upload SKILL.md to ${SERVER}:${REMOTE_REPO_DIR}/SKILL.md"
scp -i "$KEY_PATH" "$LOCAL_SKILL" "$SERVER:$REMOTE_REPO_DIR/SKILL.md"

echo "[2/7] upload appendix to ${SERVER}:${REMOTE_REPO_DIR}/docs/SKILL_APPENDIX_FULL.md"
ssh -i "$KEY_PATH" "$SERVER" "mkdir -p '$REMOTE_REPO_DIR/docs'"
scp -i "$KEY_PATH" "$LOCAL_APPENDIX" "$SERVER:$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md"

echo "[3/7] upload OpenClaw plugin runtime into repo"
ssh -i "$KEY_PATH" "$SERVER" "mkdir -p '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist'"
scp -i "$KEY_PATH" "$LOCAL_PLUGIN_DIST" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js"
scp -i "$KEY_PATH" "$LOCAL_PLUGIN_MANIFEST" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json"
scp -i "$KEY_PATH" "$LOCAL_PLUGIN_PACKAGE" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json"

echo "[4/7] upload Claude finance final_answer wrapper"
ssh -i "$KEY_PATH" "$SERVER" "mkdir -p '$REMOTE_REPO_DIR/tests/scripts'"
scp -i "$KEY_PATH" "$LOCAL_CLAUDE_WRAPPER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh"
scp -i "$KEY_PATH" "$LOCAL_ONLINE_CHECKER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py"
ssh -i "$KEY_PATH" "$SERVER" "chmod 755 '$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh' '$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py'"

echo "[5/7] build remote financeqa Go MCP binary"
ssh -i "$KEY_PATH" "$SERVER" "cd '$REMOTE_REPO_DIR' && go build -o '$REMOTE_REPO_DIR/financeqa' ./cmd/financeqa/..."

echo "[6/7] publish OpenClaw extension runtime and skill symlinks"
ssh -i "$KEY_PATH" "$SERVER" "set -e; \
  if [ -L '$REMOTE_OPENCLAW_PLUGIN_DIR' ]; then rm -f '$REMOTE_OPENCLAW_PLUGIN_DIR'; fi; \
  mkdir -p '$REMOTE_OPENCLAW_SKILL_DIR/docs'; \
  mkdir -p '$REMOTE_CLAUDE_SKILL_DIR/docs'; \
  mkdir -p '$REMOTE_OPENCLAW_PLUGIN_DIR/dist'; \
  rm -f '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md'; \
  rm -f '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md' '$REMOTE_CLAUDE_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md'; \
  rm -rf '$REMOTE_OPENCLAW_EXT_SKILL_DIR'; \
  rm -f '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'; \
  ln -sfn '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md'; \
  ln -sfn '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md'; \
  ln -sfn '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md'; \
  ln -sfn '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_CLAUDE_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md'; \
  ln -sfn '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  ln -sfn '$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json'; \
  ln -sfn '$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'; \
  node -e \"const fs=require('fs'); const financeSkillDir = process.argv[2]; const configPath = process.argv[3]; const cfg=JSON.parse(fs.readFileSync(configPath,'utf8')); cfg.skills = cfg.skills && typeof cfg.skills === 'object' ? cfg.skills : {}; cfg.skills.load = cfg.skills.load && typeof cfg.skills.load === 'object' ? cfg.skills.load : {}; const existing = Array.isArray(cfg.skills.load.extraDirs) ? cfg.skills.load.extraDirs.filter((dir) => typeof dir === 'string' && dir.trim()) : []; cfg.skills.load.extraDirs = [financeSkillDir, ...existing.filter((dir) => dir !== financeSkillDir)]; cfg.plugins = cfg.plugins && typeof cfg.plugins === 'object' ? cfg.plugins : {}; cfg.plugins.entries = cfg.plugins.entries && typeof cfg.plugins.entries === 'object' ? cfg.plugins.entries : {}; cfg.plugins.entries['openclaw-finance'] = cfg.plugins.entries['openclaw-finance'] && typeof cfg.plugins.entries['openclaw-finance'] === 'object' ? cfg.plugins.entries['openclaw-finance'] : {}; cfg.plugins.entries['openclaw-finance'].enabled = true; cfg.plugins.entries['openclaw-finance'].hooks = cfg.plugins.entries['openclaw-finance'].hooks && typeof cfg.plugins.entries['openclaw-finance'].hooks === 'object' ? cfg.plugins.entries['openclaw-finance'].hooks : {}; cfg.plugins.entries['openclaw-finance'].hooks.allowPromptInjection = true; const tmp = configPath + '.tmp-' + process.pid; fs.writeFileSync(tmp, JSON.stringify(cfg, null, 2) + String.fromCharCode(10)); fs.renameSync(tmp, configPath);\" _ '$REMOTE_OPENCLAW_SKILL_DIR' '$REMOTE_OPENCLAW_CONFIG_PATH'; \
  echo 'patched skills.load.extraDirs for finance'; \
  node -e \"const fs=require('fs'); const sessionStorePath=process.argv[2]; if (fs.existsSync(sessionStorePath)) { const sessions=JSON.parse(fs.readFileSync(sessionStorePath,'utf8')); let cleared=0; for (const entry of Object.values(sessions)) { if (entry && typeof entry === 'object' && entry.skillsSnapshot) { delete entry.skillsSnapshot; cleared += 1; } } if (cleared > 0) { const tmp = sessionStorePath + '.tmp-' + process.pid; fs.writeFileSync(tmp, JSON.stringify(sessions, null, 2) + String.fromCharCode(10)); fs.renameSync(tmp, sessionStorePath); } console.log('cleared skillsSnapshot caches: ' + cleared); }\" _ '$REMOTE_OPENCLAW_SESSION_STORE'; \
  chmod 444 '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js' '$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json' '$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json'"

echo "[7/7] verify skill path, plugin runtime, Claude wrapper, and Go MCP server on server"
ssh -i "$KEY_PATH" "$SERVER" "set -e; \
  ls -l '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_REPO_DIR/financeqa' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md' '$REMOTE_CLAUDE_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json' '$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh' '$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py'; \
  test ! -e '$REMOTE_OPENCLAW_EXT_SKILL_DIR'; \
  test ! -e '$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md')\" = '$REMOTE_REPO_DIR/SKILL.md'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md')\" = '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md'; \
  test \"\$(readlink -f '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md')\" = '$REMOTE_REPO_DIR/SKILL.md'; \
  test \"\$(readlink -f '$REMOTE_CLAUDE_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md')\" = '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js')\" = '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json')\" = '$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json')\" = '$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json'; \
  grep -n 'SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md'; \
  grep -n 'SKILL_APPENDIX_FULL.md' '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md'; \
  grep -n 'before_prompt_build' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  grep -n 'mustCallFinanceQuerySystemContext' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  grep -n 'FINANCEQA_BIN' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  grep -n 'final_answer' '$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh'; \
  grep -n 'claude_finance_final_answer.sh' '$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py'; \
  printf '%s\n' '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}' | '$REMOTE_REPO_DIR/financeqa' serve --skill '$REMOTE_REPO_DIR/SKILL.md' --appendix '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' >/tmp/finance_mcp_init.json; \
  cat /tmp/finance_mcp_init.json"

echo "done."
