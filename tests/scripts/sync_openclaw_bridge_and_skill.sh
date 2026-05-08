#!/usr/bin/env bash
set -euo pipefail

# Sync Go MCP binary + SKILL.md + appendix + OpenClaw plugin runtime to server.
# SERVER defaults to the local SSH config alias. KEY_PATH is optional; set it
# only when the SSH config alias is not enough.

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SERVER="${SERVER:-lzh}"
SSH_OPTS=()
SCP_OPTS=()
if [[ -n "${KEY_PATH:-}" ]]; then
  SSH_OPTS=(-i "$KEY_PATH")
  SCP_OPTS=(-i "$KEY_PATH")
fi
REMOTE_HOME="${REMOTE_HOME:-$(ssh "${SSH_OPTS[@]}" "$SERVER" 'printf %s "$HOME"')}"

LOCAL_SKILL="$ROOT_DIR/SKILL.md"
LOCAL_APPENDIX="$ROOT_DIR/docs/SKILL_APPENDIX_FULL.md"
LOCAL_PLUGIN_DIST="$ROOT_DIR/plugin/openclaw-finance/dist/index.esm.js"
LOCAL_PLUGIN_MANIFEST="$ROOT_DIR/plugin/openclaw-finance/openclaw.plugin.json"
LOCAL_PLUGIN_PACKAGE="$ROOT_DIR/plugin/openclaw-finance/package.json"
LOCAL_CLAUDE_WRAPPER="$ROOT_DIR/tests/scripts/claude_finance_final_answer.sh"
LOCAL_ONLINE_CHECKER="$ROOT_DIR/tests/scripts/run_online_agent_final_answer_check.py"
LOCAL_FINANCEQA_BIN="$(mktemp "${TMPDIR:-/tmp}/financeqa-linux-amd64.XXXXXX")"
trap 'rm -f "$LOCAL_FINANCEQA_BIN"' EXIT

REMOTE_REPO_DIR="${REMOTE_REPO_DIR:-$REMOTE_HOME/finance_qa}"
REMOTE_FINANCEQA_BIN="${REMOTE_FINANCEQA_BIN:-$REMOTE_REPO_DIR/bin/financeqa}"
REMOTE_FINANCEQA_BIN_DIR="$(dirname "$REMOTE_FINANCEQA_BIN")"
REMOTE_FINANCEQA_SERVE_PATTERN="${REMOTE_FINANCEQA_SERVE_PATTERN:-$(printf '%s' "$REMOTE_FINANCEQA_BIN" | sed 's#/#[/]#g') serve}"
REMOTE_OPENCLAW_PLUGIN_DIR="${REMOTE_OPENCLAW_PLUGIN_DIR:-$REMOTE_HOME/.openclaw/extensions/openclaw-finance}"
REMOTE_OPENCLAW_SKILL_DIR="${REMOTE_OPENCLAW_SKILL_DIR:-$REMOTE_HOME/.openclaw/skills/finance}"
REMOTE_OPENCLAW_EXT_SKILL_DIR="${REMOTE_OPENCLAW_EXT_SKILL_DIR:-$REMOTE_HOME/.openclaw/extensions/openclaw-finance/skills/finance}"
REMOTE_CLAUDE_SKILL_DIR="${REMOTE_CLAUDE_SKILL_DIR:-$REMOTE_HOME/.claude/skills/finance}"
REMOTE_OPENCLAW_SKILL_PARENT="$(dirname "$REMOTE_OPENCLAW_SKILL_DIR")"
REMOTE_CLAUDE_SKILL_PARENT="$(dirname "$REMOTE_CLAUDE_SKILL_DIR")"
REMOTE_OPENCLAW_SESSION_STORE="${REMOTE_OPENCLAW_SESSION_STORE:-$REMOTE_HOME/.openclaw/agents/main/sessions/sessions.json}"
REMOTE_OPENCLAW_CONFIG_PATH="${REMOTE_OPENCLAW_CONFIG_PATH:-$REMOTE_HOME/.openclaw/openclaw.json}"
RESTART_OPENCLAW_GATEWAY="${RESTART_OPENCLAW_GATEWAY:-1}"

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

echo "[1/8] upload SKILL.md to ${SERVER}:${REMOTE_REPO_DIR}/SKILL.md"
scp "${SCP_OPTS[@]}" "$LOCAL_SKILL" "$SERVER:$REMOTE_REPO_DIR/SKILL.md"

echo "[2/8] upload appendix to ${SERVER}:${REMOTE_REPO_DIR}/docs/SKILL_APPENDIX_FULL.md"
ssh "${SSH_OPTS[@]}" "$SERVER" "mkdir -p '$REMOTE_REPO_DIR/docs'"
scp "${SCP_OPTS[@]}" "$LOCAL_APPENDIX" "$SERVER:$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md"

echo "[3/8] upload OpenClaw plugin runtime into repo"
ssh "${SSH_OPTS[@]}" "$SERVER" "mkdir -p '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist'"
scp "${SCP_OPTS[@]}" "$LOCAL_PLUGIN_DIST" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js"
scp "${SCP_OPTS[@]}" "$LOCAL_PLUGIN_MANIFEST" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json"
scp "${SCP_OPTS[@]}" "$LOCAL_PLUGIN_PACKAGE" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json"

echo "[4/8] upload Claude finance final_answer wrapper"
ssh "${SSH_OPTS[@]}" "$SERVER" "mkdir -p '$REMOTE_REPO_DIR/tests/scripts'"
scp "${SCP_OPTS[@]}" "$LOCAL_CLAUDE_WRAPPER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh"
scp "${SCP_OPTS[@]}" "$LOCAL_ONLINE_CHECKER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py"
ssh "${SSH_OPTS[@]}" "$SERVER" "chmod 755 '$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh' '$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py'"

echo "[5/8] build local Linux financeqa Go MCP binary and upload"
(
  cd "$ROOT_DIR"
  GOOS=linux GOARCH=amd64 go build -o "$LOCAL_FINANCEQA_BIN" ./cmd/financeqa/...
)
ssh "${SSH_OPTS[@]}" "$SERVER" "set -e; \
  if command -v pgrep >/dev/null 2>&1; then \
    pgrep -f '$REMOTE_FINANCEQA_SERVE_PATTERN' | xargs -r kill; \
  fi; \
  mkdir -p '$REMOTE_FINANCEQA_BIN_DIR' && rm -f '$REMOTE_REPO_DIR/financeqa'"
scp "${SCP_OPTS[@]}" "$LOCAL_FINANCEQA_BIN" "$SERVER:$REMOTE_FINANCEQA_BIN"
ssh "${SSH_OPTS[@]}" "$SERVER" "chmod 755 '$REMOTE_FINANCEQA_BIN'"

echo "[6/8] publish OpenClaw extension runtime files and skill symlinks"
ssh "${SSH_OPTS[@]}" "$SERVER" "set -e; \
  if [ -L '$REMOTE_OPENCLAW_PLUGIN_DIR' ]; then rm -f '$REMOTE_OPENCLAW_PLUGIN_DIR'; fi; \
  mkdir -p '$REMOTE_OPENCLAW_SKILL_PARENT' '$REMOTE_CLAUDE_SKILL_PARENT'; \
  mkdir -p '$REMOTE_OPENCLAW_PLUGIN_DIR/dist'; \
  rm -rf '$REMOTE_OPENCLAW_SKILL_DIR' '$REMOTE_CLAUDE_SKILL_DIR' '$REMOTE_OPENCLAW_EXT_SKILL_DIR'; \
  rm -f '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'; \
  ln -sfn '$REMOTE_REPO_DIR' '$REMOTE_OPENCLAW_SKILL_DIR'; \
  ln -sfn '$REMOTE_REPO_DIR' '$REMOTE_CLAUDE_SKILL_DIR'; \
  cp '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  cp '$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json'; \
  cp '$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'; \
  chmod 444 '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'; \
  node -e \"const fs=require('fs'); const financeSkillDir = process.argv[2]; const configPath = process.argv[3]; const packagePath = process.argv[4]; const cfg=JSON.parse(fs.readFileSync(configPath,'utf8')); const pkg=JSON.parse(fs.readFileSync(packagePath,'utf8')); const pluginVersion = pkg.version; const packageExtensions = Array.isArray(pkg.openclaw?.extensions) ? pkg.openclaw.extensions : []; const extraDirs = Array.isArray(cfg.skills?.load?.extraDirs) ? cfg.skills.load.extraDirs : []; const pluginEnabled = cfg.plugins?.entries?.['openclaw-finance']?.enabled === true; const promptHookEnabled = cfg.plugins?.entries?.['openclaw-finance']?.hooks?.allowPromptInjection === true; const install = cfg.plugins?.installs?.['openclaw-finance']; const missing = []; if (!packageExtensions.includes('./dist/index.esm.js')) missing.push('package.json openclaw.extensions must include ./dist/index.esm.js'); if (!extraDirs.includes(financeSkillDir)) missing.push('skills.load.extraDirs must include ' + financeSkillDir); if (!pluginEnabled) missing.push('plugins.entries.openclaw-finance.enabled must be true'); if (!promptHookEnabled) missing.push('plugins.entries.openclaw-finance.hooks.allowPromptInjection must be true'); if (!install || typeof install !== 'object') missing.push('plugins.installs.openclaw-finance must exist'); if (missing.length) { console.error('OpenClaw config is not ready for finance runtime:'); for (const item of missing) console.error('- ' + item); process.exit(1); } cfg.plugins.installs['openclaw-finance'].version = pluginVersion; cfg.plugins.installs['openclaw-finance'].installedAt = new Date().toISOString(); const tmp = configPath + '.tmp-' + process.pid; fs.writeFileSync(tmp, JSON.stringify(cfg, null, 2) + String.fromCharCode(10)); fs.renameSync(tmp, configPath); console.log('verify OpenClaw config references the finance plugin and skill path'); console.log('updated OpenClaw install metadata for openclaw-finance to ' + pluginVersion);\" _ '$REMOTE_OPENCLAW_SKILL_DIR' '$REMOTE_OPENCLAW_CONFIG_PATH' '$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json'; \
  node -e \"const fs=require('fs'); const sessionStorePath=process.argv[2]; if (fs.existsSync(sessionStorePath)) { const sessions=JSON.parse(fs.readFileSync(sessionStorePath,'utf8')); let cleared=0; for (const entry of Object.values(sessions)) { if (entry && typeof entry === 'object' && entry.skillsSnapshot) { delete entry.skillsSnapshot; cleared += 1; } } if (cleared > 0) { const tmp = sessionStorePath + '.tmp-' + process.pid; fs.writeFileSync(tmp, JSON.stringify(sessions, null, 2) + String.fromCharCode(10)); fs.renameSync(tmp, sessionStorePath); } console.log('cleared skillsSnapshot caches: ' + cleared); }\" _ '$REMOTE_OPENCLAW_SESSION_STORE'; \
  chmod 444 '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js' '$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json' '$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json'"

echo "[7/8] verify skill path, plugin runtime, Claude wrapper, and Go MCP server on server"
ssh "${SSH_OPTS[@]}" "$SERVER" "set -e; \
  ls -ld '$REMOTE_OPENCLAW_SKILL_DIR' '$REMOTE_CLAUDE_SKILL_DIR'; \
  ls -l '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_FINANCEQA_BIN' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md' '$REMOTE_CLAUDE_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json' '$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh' '$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py'; \
  test ! -e '$REMOTE_REPO_DIR/financeqa'; \
  test ! -e '$REMOTE_OPENCLAW_EXT_SKILL_DIR'; \
  test ! -e '$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts'; \
  test -L '$REMOTE_OPENCLAW_SKILL_DIR'; \
  test -L '$REMOTE_CLAUDE_SKILL_DIR'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_SKILL_DIR')\" = '$REMOTE_REPO_DIR'; \
  test \"\$(readlink -f '$REMOTE_CLAUDE_SKILL_DIR')\" = '$REMOTE_REPO_DIR'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md')\" = '$REMOTE_REPO_DIR/SKILL.md'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md')\" = '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md'; \
  test \"\$(readlink -f '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md')\" = '$REMOTE_REPO_DIR/SKILL.md'; \
  test \"\$(readlink -f '$REMOTE_CLAUDE_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md')\" = '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md'; \
  test ! -L '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  test ! -L '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json'; \
  test ! -L '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js')\" = '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json')\" = '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json'; \
  test \"\$(readlink -f '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json')\" = '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'; \
  grep -n 'SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md'; \
  grep -n 'SKILL_APPENDIX_FULL.md' '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md'; \
  grep -n 'before_prompt_build' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  grep -n 'mustCallFinanceQuerySystemContext' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  grep -n 'FINANCEQA_BIN' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'; \
  grep -n 'final_answer' '$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh'; \
  grep -n 'claude_finance_final_answer.sh' '$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py'; \
  if command -v openclaw >/dev/null 2>&1; then openclaw skills list --json 2>&1 | grep -q '\"name\": \"finance\"'; fi; \
  '$REMOTE_FINANCEQA_BIN' version; \
  printf '%s\n' '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}' | '$REMOTE_FINANCEQA_BIN' serve --skill '$REMOTE_REPO_DIR/SKILL.md' --appendix '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' >/tmp/finance_mcp_init.json; \
  cat /tmp/finance_mcp_init.json"

if [[ "$RESTART_OPENCLAW_GATEWAY" == "1" ]]; then
  echo "[8/8] restart OpenClaw gateway so the updated extension runtime is loaded"
  ssh "${SSH_OPTS[@]}" "$SERVER" "set -e; \
    if command -v openclaw >/dev/null 2>&1; then \
      openclaw gateway restart; \
      sleep 3; \
      openclaw gateway status >/tmp/openclaw_gateway_status.txt; \
      grep -q 'RPC probe: ok' /tmp/openclaw_gateway_status.txt; \
      grep -q 'Runtime: running' /tmp/openclaw_gateway_status.txt; \
    fi"
else
  echo "[8/8] skip OpenClaw gateway restart (RESTART_OPENCLAW_GATEWAY=$RESTART_OPENCLAW_GATEWAY)"
fi

echo "done."
