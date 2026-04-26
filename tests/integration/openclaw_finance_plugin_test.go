package integration_test

import (
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
		`LOCAL_PLUGIN_INDEX="$ROOT_DIR/plugin/openclaw-finance/index.ts"`,
		`LOCAL_PLUGIN_MANIFEST="$ROOT_DIR/plugin/openclaw-finance/openclaw.plugin.json"`,
		`LOCAL_CLAUDE_WRAPPER="$ROOT_DIR/tests/scripts/claude_finance_final_answer.sh"`,
		`LOCAL_ONLINE_CHECKER="$ROOT_DIR/tests/scripts/run_online_agent_final_answer_check.py"`,
		`scp -i "$KEY_PATH" "$LOCAL_PLUGIN_DIST" "$SERVER:$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js"`,
		`scp -i "$KEY_PATH" "$LOCAL_PLUGIN_INDEX" "$SERVER:$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts"`,
		`scp -i "$KEY_PATH" "$LOCAL_PLUGIN_MANIFEST" "$SERVER:$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json"`,
		`scp -i "$KEY_PATH" "$LOCAL_CLAUDE_WRAPPER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh"`,
		`scp -i "$KEY_PATH" "$LOCAL_ONLINE_CHECKER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py"`,
		`go build -o '$REMOTE_REPO_DIR/financeqa' ./cmd/financeqa/...`,
		`plugins.entries['openclaw-finance'].hooks.allowPromptInjection = true`,
	} {
		if !strings.Contains(scriptText, want) {
			t.Fatalf("sync script should publish OpenClaw plugin runtime and prompt hook config; missing %q", want)
		}
	}
}
