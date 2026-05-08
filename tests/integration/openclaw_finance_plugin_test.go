package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenClawFinancePluginLetsModelUseFinanceToolWithoutHardIntercept(t *testing.T) {
	t.Parallel()

	pluginPath := filepath.Join("..", "..", "plugin", "openclaw-finance", "dist", "index.esm.js")
	plugin, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("read OpenClaw finance plugin runtime: %v", err)
	}
	pluginText := string(plugin)

	for _, want := range []string{
		`api.on("before_prompt_build"`,
		`finance-query`,
		`数据(出来|有了|有没有|情况|多少)`,
		`mustCallFinanceQuerySystemContext`,
		`contract_continuity_candidates`,
		`do not omit the source note`,
		`same-project candidates/references`,
		`isFinanceQuestion`,
	} {
		if !strings.Contains(pluginText, want) {
			t.Fatalf("OpenClaw finance plugin should contain %q", want)
		}
	}
	for _, reject := range []string{
		`api.on("before_dispatch"`,
		`api.on("before_message_write"`,
		`FINANCE_QUERY_FINAL_ANSWER_START`,
		`FINANCE_QUERY_PAYLOAD_START`,
		`FINANCE_BRIDGE_PATH`,
		`/root/.openclaw/extensions/openclaw-finance/server/finance_bridge.py`,
		`forcedAnswersBySessionKey`,
		`isBridgeFallbackPayload`,
		`finance-query has already been executed`,
		`prependContext`,
	} {
		if strings.Contains(pluginText, reject) {
			t.Fatalf("OpenClaw finance plugin should not hard-intercept model answers; found %q", reject)
		}
	}
	if !strings.Contains(pluginText, "Do not answer from prior conversation history") {
		t.Fatalf("prompt hook should forbid stale repeated answers from conversation history")
	}
	if !strings.Contains(pluginText, "process.env.FINANCEQA_BIN") {
		t.Fatalf("OpenClaw finance plugin should resolve the Go MCP binary path from FINANCEQA_BIN")
	}
	if !strings.Contains(pluginText, `process.env.HOME || ""`) ||
		!strings.Contains(pluginText, `"finance_qa/bin/financeqa"`) {
		t.Fatalf("OpenClaw finance plugin should prefer the fixed server binary path under $HOME/finance_qa/bin/financeqa")
	}
	if !strings.Contains(pluginText, "existsSync(candidate)") {
		t.Fatalf("OpenClaw finance plugin should check binary candidates before selecting one")
	}
	for _, forbidden := range []string{
		`path.resolve(repoRoot, "financeqa")`,
		`path.resolve(process.cwd(), "financeqa")`,
		`"/usr/local/bin/financeqa"`,
		`"/usr/bin/financeqa"`,
	} {
		if strings.Contains(pluginText, forbidden) {
			t.Fatalf("OpenClaw finance plugin should not fall back to non-canonical financeqa binary path %q", forbidden)
		}
	}
	if !strings.Contains(pluginText, `spawn(this.binaryPath, ["serve"]`) {
		t.Fatalf("OpenClaw finance plugin should start financeqa serve as the Go MCP server")
	}
}

func TestOpenClawFinancePluginMetadataUsesCurrentMajorVersion(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		filepath.Join("..", "..", "plugin", "openclaw-finance", "package.json"),
		filepath.Join("..", "..", "plugin", "openclaw-finance", "openclaw.plugin.json"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read plugin metadata %s: %v", path, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse plugin metadata %s: %v", path, err)
		}
		if got := doc["version"]; got != "2.0.1" {
			t.Fatalf("%s version = %v, want 2.0.1", path, got)
		}
	}
}

func TestSyncScriptPublishesOpenClawFinancePluginRuntime(t *testing.T) {
	t.Parallel()

	syncScriptPath := filepath.Join("..", "..", "tests", "scripts", "sync_openclaw_bridge_and_skill.sh")
	syncScript, err := os.ReadFile(syncScriptPath)
	if err != nil {
		t.Fatalf("read sync script: %v", err)
	}
	scriptText := string(syncScript)

	for _, want := range []string{
		`LOCAL_PLUGIN_DIST="$ROOT_DIR/plugin/openclaw-finance/dist/index.esm.js"`,
		`LOCAL_PLUGIN_MANIFEST="$ROOT_DIR/plugin/openclaw-finance/openclaw.plugin.json"`,
		`LOCAL_CLAUDE_WRAPPER="$ROOT_DIR/tests/scripts/claude_finance_final_answer.sh"`,
		`LOCAL_ONLINE_CHECKER="$ROOT_DIR/tests/scripts/run_online_agent_final_answer_check.py"`,
		`scp -i "$KEY_PATH" "$LOCAL_PLUGIN_DIST" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js"`,
		`scp -i "$KEY_PATH" "$LOCAL_PLUGIN_MANIFEST" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json"`,
		`scp -i "$KEY_PATH" "$LOCAL_CLAUDE_WRAPPER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh"`,
		`scp -i "$KEY_PATH" "$LOCAL_ONLINE_CHECKER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py"`,
		`if [ -L '$REMOTE_OPENCLAW_PLUGIN_DIR' ]; then rm -f '$REMOTE_OPENCLAW_PLUGIN_DIR'; fi;`,
		`ln -sfn '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'`,
		`ln -sfn '$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json'`,
		`ln -sfn '$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'`,
		`REMOTE_FINANCEQA_BIN="${REMOTE_FINANCEQA_BIN:-$REMOTE_REPO_DIR/bin/financeqa}"`,
		`rm -f '$REMOTE_REPO_DIR/financeqa'`,
		`'$REMOTE_FINANCEQA_BIN' serve`,
		`verify OpenClaw config references the finance plugin and skill path`,
		`cfg.plugins?.entries?.['openclaw-finance']?.enabled === true`,
		`cfg.plugins?.entries?.['openclaw-finance']?.hooks?.allowPromptInjection === true`,
		`cfg.plugins.installs['openclaw-finance'].version = pluginVersion`,
		`cfg.plugins.installs['openclaw-finance'].installedAt = new Date().toISOString()`,
	} {
		if !strings.Contains(scriptText, want) {
			t.Fatalf("sync script should publish OpenClaw plugin runtime and prompt hook config; missing %q", want)
		}
	}
	if strings.Contains(scriptText, `cfg.plugins.entries['openclaw-finance'].enabled = true`) ||
		strings.Contains(scriptText, `cfg.plugins.entries['openclaw-finance'].hooks.allowPromptInjection = true`) {
		t.Fatalf("sync script should verify existing OpenClaw runtime config by default, not rewrite plugin entry settings")
	}
	if strings.Contains(scriptText, `$SERVER:$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js`) ||
		strings.Contains(scriptText, `$SERVER:$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts`) ||
		strings.Contains(scriptText, `$SERVER:$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json`) {
		t.Fatalf("sync script should not copy plugin runtime directly into OpenClaw extension; use repo-backed symlinks")
	}
	if strings.Contains(scriptText, "LOCAL_PLUGIN_INDEX") ||
		strings.Contains(scriptText, "plugin/openclaw-finance/index.ts' '$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts") {
		t.Fatalf("sync script should not publish plugin source entrypoint into the OpenClaw extension")
	}
	if strings.Contains(scriptText, `go build -o '$REMOTE_REPO_DIR/financeqa'`) {
		t.Fatalf("sync script should not build the canonical server binary at repo root")
	}
	if strings.Contains(scriptText, `ln -sfn '$REMOTE_FINANCEQA_BIN' '$REMOTE_REPO_DIR/financeqa'`) {
		t.Fatalf("sync script should not keep a second repo-root financeqa entrypoint")
	}
	if strings.Contains(scriptText, "finance_bridge.py") || strings.Contains(scriptText, "LOCAL_BRIDGE") {
		t.Fatalf("sync script should publish Go MCP only, got Python bridge references")
	}
}

func TestFinanceFinalAnswerWrapperDoesNotHardcodeServerBridgePath(t *testing.T) {
	t.Parallel()

	wrapperPath := filepath.Join("..", "..", "tests", "scripts", "claude_finance_final_answer.sh")
	wrapper, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("read final answer wrapper: %v", err)
	}
	wrapperText := string(wrapper)
	if strings.Contains(wrapperText, "/root/.openclaw/extensions/openclaw-finance/server/finance_bridge.py") {
		t.Fatalf("final answer wrapper should not hardcode the server bridge path")
	}
	if strings.Contains(wrapperText, "FINANCE_BRIDGE_PATH") || strings.Contains(wrapperText, "BRIDGE_PATH") || strings.Contains(wrapperText, "finance_bridge.py") {
		t.Fatalf("final answer wrapper should not call the Python bridge")
	}
	if !strings.Contains(wrapperText, `FINANCEQA_BIN="${FINANCEQA_BIN:-$ROOT_DIR/bin/financeqa}"`) {
		t.Fatalf("final answer wrapper should default to the canonical repo bin/financeqa binary")
	}
	if !strings.Contains(wrapperText, `financeqa_bin, "serve"`) {
		t.Fatalf("final answer wrapper should call financeqa serve")
	}
}
