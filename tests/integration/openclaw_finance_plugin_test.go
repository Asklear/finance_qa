package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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
		`financeQuerySystemFacts`,
		`latestFinanceQuestionFromMessages`,
		`latestUserTextFromMessages`,
		`financeQuestionForPromptEvent`,
		`最新财务问题`,
		`previous model attempt failed`,
		`contract_continuity_candidates`,
		`来源和来源更新时间必须一致`,
		`same-project candidates/references`,
		`isFinanceQuestion`,
		`findFinanceQACwd`,
		`cwd: findFinanceQACwd(this.binaryPath)`,
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
		`api.on("cleanup"`,
		`Latest finance question that MUST be sent to finance-query`,
		`Current authoritative finance-query result`,
		`Use these current facts`,
		`You may rephrase the final wording`,
	} {
		if strings.Contains(pluginText, reject) {
			t.Fatalf("OpenClaw finance plugin should not hard-intercept model answers; found %q", reject)
		}
	}
	if !strings.Contains(pluginText, "不要沿用历史对话") {
		t.Fatalf("prompt hook should forbid stale repeated answers from conversation history")
	}
	if !strings.Contains(pluginText, "process.env.FINANCEQA_BIN") {
		t.Fatalf("OpenClaw finance plugin should resolve the Go MCP binary path from FINANCEQA_BIN")
	}
	if strings.Contains(pluginText, "async register(api)") {
		t.Fatalf("OpenClaw finance plugin register hook must stay synchronous; async register is ignored by OpenClaw")
	}
	if strings.Contains(pluginText, "void getMCPClient().catch") ||
		strings.Contains(pluginText, "Failed to pre-start MCP client") ||
		strings.Contains(pluginText, "Try to pre-start MCP client") {
		t.Fatalf("OpenClaw finance plugin must not pre-start financeqa during plugin registration")
	}
	if !strings.Contains(pluginText, "register(api) {") {
		t.Fatalf("OpenClaw finance plugin should expose a synchronous register(api) hook")
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

	var packageDoc map[string]any
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
		if got := doc["version"]; got != "2.0.11" {
			t.Fatalf("%s version = %v, want 2.0.11", path, got)
		}
		if strings.HasSuffix(path, "package.json") {
			packageDoc = doc
		}
	}
	openclawConfig, ok := packageDoc["openclaw"].(map[string]any)
	if !ok {
		t.Fatalf("plugin package.json must contain openclaw metadata")
	}
	extensions, ok := openclawConfig["extensions"].([]any)
	if !ok {
		t.Fatalf("plugin package.json openclaw.extensions must be present for OpenClaw discovery")
	}
	if len(extensions) != 1 || extensions[0] != "./dist/index.esm.js" {
		t.Fatalf("plugin package.json openclaw.extensions = %v, want [./dist/index.esm.js]", extensions)
	}
	if _, ok := openclawConfig["entry"]; ok {
		t.Fatalf("plugin package.json should use openclaw.extensions, not legacy openclaw.entry")
	}
}

func TestOpenClawFinancePluginExposesFinalAnswerJsonForFinanceQuery(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for OpenClaw plugin runtime test")
	}

	tmp := t.TempDir()
	stubPath := filepath.Join(tmp, "financeqa_stub.mjs")
	stubScript := `#!/usr/bin/env node
import readline from "node:readline";

const rl = readline.createInterface({ input: process.stdin });
rl.on("line", (line) => {
  const request = JSON.parse(line);
  if (request.method === "initialize") {
    process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: request.id, result: { protocolVersion: "2024-11-05" } }) + "\n");
    return;
  }
  if (request.method === "tools/call") {
    const payload = {
      success: true,
      final_answer: "FINAL:2026年3月应收账款\n来源：《测试表》",
      data: { internal_detail: "SHOULD_NOT_BE_VISIBLE_TO_OPENCLAW_MODEL" }
    };
    process.stdout.write(JSON.stringify({
      jsonrpc: "2.0",
      id: request.id,
      result: { content: [{ type: "text", text: JSON.stringify(payload) }] }
    }) + "\n");
  }
});
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0o755); err != nil {
		t.Fatalf("write MCP stub: %v", err)
	}

	runnerPath := filepath.Join(tmp, "run_plugin.mjs")
	pluginPath, err := filepath.Abs(filepath.Join("..", "..", "plugin", "openclaw-finance", "dist", "index.esm.js"))
	if err != nil {
		t.Fatalf("resolve plugin path: %v", err)
	}
	runnerScript := `import { pathToFileURL } from "node:url";

const plugin = (await import(pathToFileURL(process.argv[2]).href)).default;
const tools = new Map();
const hooks = new Map();
const api = {
  registerTool(tool, options) {
    tools.set(options?.name || tool.name, tool);
  },
  on(name, handler) {
    hooks.set(name, handler);
  }
};

plugin.register(api);
const tool = tools.get("finance-query");
if (!tool) {
  console.error("missing finance-query tool");
  process.exit(1);
}
const result = await tool.execute("test-call", { query: "2026年3月应收账款多少？" });
const text = result?.content?.[0]?.text || "";
let payload;
try {
  payload = JSON.parse(text);
} catch (err) {
  console.error("tool output should remain JSON:", text);
  process.exit(1);
}
if (payload.final_answer !== "FINAL:2026年3月应收账款\n来源：《测试表》") {
  console.error("unexpected final_answer:", JSON.stringify(payload.final_answer));
  process.exit(1);
}
if (payload.data?.internal_detail !== "SHOULD_NOT_BE_VISIBLE_TO_OPENCLAW_MODEL") {
  console.error("unexpected payload passthrough:", JSON.stringify(payload.data));
  process.exit(1);
}
const promptHook = hooks.get("before_prompt_build");
if (!promptHook) {
  console.error("missing before_prompt_build hook");
  process.exit(1);
}
const directHookResult = await promptHook({ prompt: "2026年3月应收账款多少（已开票未收款）？", messages: [] });
if (!directHookResult?.prependSystemContext?.includes("最新财务问题：2026年3月应收账款多少（已开票未收款）？")) {
  console.error("direct finance prompt should inject latest finance question context:", JSON.stringify(directHookResult));
  process.exit(1);
}
if (Object.prototype.hasOwnProperty.call(directHookResult, "prependContext")) {
  console.error("direct finance prompt must not inject visible prependContext:", JSON.stringify(directHookResult));
  process.exit(1);
}
if (!directHookResult?.prependSystemContext?.includes("FINAL:2026年3月应收账款")) {
  console.error("direct finance prompt should inject current finance-query facts into hidden system context:", JSON.stringify(directHookResult));
  process.exit(1);
}
for (const forbidden of ["Current authoritative finance-query result", "Use these current facts", "You may rephrase the final wording", "Latest finance question that MUST"]) {
  if (directHookResult?.prependSystemContext?.includes(forbidden)) {
    console.error("direct finance prompt should not include leak-prone English context marker:", forbidden, JSON.stringify(directHookResult));
    process.exit(1);
  }
}
const messageOnlyHookResult = await promptHook({
  prompt: "",
  messages: [
    { role: "user", content: [{ type: "text", text: "[Fri 2026-05-08 11:57 GMT+8] 2026年3月应收账款多少（已开票未收款）？" }] }
  ]
});
if (!messageOnlyHookResult?.prependSystemContext?.includes("2026年3月应收账款多少（已开票未收款）？")) {
  console.error("message-only prompt event should recover latest finance question:", JSON.stringify(messageOnlyHookResult));
  process.exit(1);
}
const retryHookResult = await promptHook({
  prompt: "Continue where you left off. The previous model attempt failed or timed out.",
  messages: [
    { role: "assistant", content: [{ type: "text", text: "old stale answer" }] },
    { role: "user", content: [{ type: "text", text: "[Fri 2026-05-08 11:48 GMT+8] 2026年3月应收账款多少（已开票未收款）？" }] }
  ]
});
if (!retryHookResult?.prependSystemContext?.includes("2026年3月应收账款多少（已开票未收款）？")) {
  console.error("retry fallback prompt should recover latest finance question from messages:", JSON.stringify(retryHookResult));
  process.exit(1);
}
process.exit(0);
`
	if err := os.WriteFile(runnerPath, []byte(runnerScript), 0o644); err != nil {
		t.Fatalf("write plugin runner: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", runnerPath, pluginPath)
	cmd.Env = append(os.Environ(), "FINANCEQA_BIN="+stubPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("OpenClaw finance-query should expose final_answer JSON to the model: %v\n%s", err, string(out))
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
		`SERVER="${SERVER:-lzh}"`,
		`SSH_OPTS=()`,
		`SCP_OPTS=()`,
		`if [[ -n "${KEY_PATH:-}" ]]; then`,
		`REMOTE_HOME="${REMOTE_HOME:-$(ssh "${SSH_OPTS[@]}" "$SERVER" 'printf %s "$HOME"')}"`,
		`LOCAL_PLUGIN_DIST="$ROOT_DIR/plugin/openclaw-finance/dist/index.esm.js"`,
		`LOCAL_PLUGIN_MANIFEST="$ROOT_DIR/plugin/openclaw-finance/openclaw.plugin.json"`,
		`LOCAL_CLAUDE_WRAPPER="$ROOT_DIR/tests/scripts/claude_finance_final_answer.sh"`,
		`LOCAL_ONLINE_CHECKER="$ROOT_DIR/tests/scripts/run_online_agent_final_answer_check.py"`,
		`scp "${SCP_OPTS[@]}" "$LOCAL_PLUGIN_DIST" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js"`,
		`scp "${SCP_OPTS[@]}" "$LOCAL_PLUGIN_MANIFEST" "$SERVER:$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json"`,
		`scp "${SCP_OPTS[@]}" "$LOCAL_CLAUDE_WRAPPER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/claude_finance_final_answer.sh"`,
		`scp "${SCP_OPTS[@]}" "$LOCAL_ONLINE_CHECKER" "$SERVER:$REMOTE_REPO_DIR/tests/scripts/run_online_agent_final_answer_check.py"`,
		`if [ -L '$REMOTE_OPENCLAW_PLUGIN_DIR' ]; then rm -f '$REMOTE_OPENCLAW_PLUGIN_DIR'; fi;`,
		`cp '$REMOTE_REPO_DIR/plugin/openclaw-finance/dist/index.esm.js' '$REMOTE_OPENCLAW_PLUGIN_DIR/dist/index.esm.js'`,
		`cp '$REMOTE_REPO_DIR/plugin/openclaw-finance/openclaw.plugin.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/openclaw.plugin.json'`,
		`cp '$REMOTE_REPO_DIR/plugin/openclaw-finance/package.json' '$REMOTE_OPENCLAW_PLUGIN_DIR/package.json'`,
		`REMOTE_FINANCEQA_BIN="${REMOTE_FINANCEQA_BIN:-$REMOTE_REPO_DIR/bin/financeqa}"`,
		`LOCAL_FINANCEQA_BIN="$(mktemp`,
		`GOOS=linux GOARCH=amd64 go build -o "$LOCAL_FINANCEQA_BIN" ./cmd/financeqa/...`,
		`REMOTE_FINANCEQA_SERVE_PATTERN`,
		`pgrep -f '$REMOTE_FINANCEQA_SERVE_PATTERN'`,
		`scp "${SCP_OPTS[@]}" "$LOCAL_FINANCEQA_BIN" "$SERVER:$REMOTE_FINANCEQA_BIN"`,
		`rm -f '$REMOTE_REPO_DIR/financeqa'`,
		`'$REMOTE_FINANCEQA_BIN' serve`,
		`pkg.openclaw?.extensions`,
		`verify OpenClaw config references the finance plugin and skill path`,
		`cfg.plugins?.entries?.['openclaw-finance']?.enabled === true`,
		`cfg.plugins?.entries?.['openclaw-finance']?.hooks?.allowPromptInjection === true`,
		`cfg.plugins.installs['openclaw-finance'].version = pluginVersion`,
		`cfg.plugins.installs['openclaw-finance'].installedAt = new Date().toISOString()`,
		`RESTART_OPENCLAW_GATEWAY="${RESTART_OPENCLAW_GATEWAY:-1}"`,
		`openclaw gateway restart`,
		`grep -q 'RPC probe: ok' /tmp/openclaw_gateway_status.txt`,
		`grep -q 'Runtime: running' /tmp/openclaw_gateway_status.txt`,
	} {
		if !strings.Contains(scriptText, want) {
			t.Fatalf("sync script should publish OpenClaw plugin runtime and prompt hook config; missing %q", want)
		}
	}
	if strings.Contains(scriptText, `cfg.plugins.entries['openclaw-finance'].enabled = true`) ||
		strings.Contains(scriptText, `cfg.plugins.entries['openclaw-finance'].hooks.allowPromptInjection = true`) {
		t.Fatalf("sync script should verify existing OpenClaw runtime config by default, not rewrite plugin entry settings")
	}
	if strings.Contains(scriptText, `$SERVER:$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts`) {
		t.Fatalf("sync script should not publish plugin source entrypoint into the OpenClaw extension")
	}
	if strings.Contains(scriptText, `ln -sfn '$REMOTE_REPO_DIR/plugin/openclaw-finance' '$REMOTE_OPENCLAW_PLUGIN_DIR'`) ||
		strings.Contains(scriptText, `ln -sfn '$REMOTE_REPO_DIR' '$REMOTE_OPENCLAW_PLUGIN_DIR'`) {
		t.Fatalf("sync script should keep OpenClaw extension as copied runtime files, not a directory symlink")
	}
	if strings.Contains(scriptText, "LOCAL_PLUGIN_INDEX") ||
		strings.Contains(scriptText, "plugin/openclaw-finance/index.ts' '$REMOTE_OPENCLAW_PLUGIN_DIR/index.ts") {
		t.Fatalf("sync script should not publish plugin source entrypoint into the OpenClaw extension")
	}
	if strings.Contains(scriptText, `go build -o '$REMOTE_REPO_DIR/financeqa'`) {
		t.Fatalf("sync script should not build the canonical server binary at repo root")
	}
	if strings.Contains(scriptText, `cd '$REMOTE_REPO_DIR' && mkdir -p '$(dirname "$REMOTE_FINANCEQA_BIN")' && go build`) {
		t.Fatalf("sync script should build the server binary from local source before uploading")
	}
	if strings.Contains(scriptText, `ln -sfn '$REMOTE_FINANCEQA_BIN' '$REMOTE_REPO_DIR/financeqa'`) {
		t.Fatalf("sync script should not keep a second repo-root financeqa entrypoint")
	}
	if strings.Contains(scriptText, "finance_bridge.py") || strings.Contains(scriptText, "LOCAL_BRIDGE") {
		t.Fatalf("sync script should publish Go MCP only, got Python bridge references")
	}
}

func TestDeploymentDocsAndSyncScriptDoNotExposeProductionNetworkTargets(t *testing.T) {
	t.Parallel()

	checkedFiles := []string{
		filepath.Join("..", "..", "README.md"),
		filepath.Join("..", "..", "docs", "architecture", "03-deployment-runtime.md"),
		filepath.Join("..", "..", "plugin", "openclaw-finance", "server", "README.md"),
		filepath.Join("..", "..", "tests", "scripts", "deploy_openclaw.sh"),
		filepath.Join("..", "..", "tests", "scripts", "sync_openclaw_bridge_and_skill.sh"),
	}
	publicIPv4 := regexp.MustCompile(`\b(?:[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])\.(?:\d{1,3})\.(?:\d{1,3})\.(?:\d{1,3})\b`)
	privateOrLocalIPv4 := regexp.MustCompile(`^(?:127\.|10\.|192\.168\.|172\.(?:1[6-9]|2\d|3[0-1])\.)`)

	for _, path := range checkedFiles {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(raw)
		for _, match := range publicIPv4.FindAllString(text, -1) {
			if privateOrLocalIPv4.MatchString(match) {
				continue
			}
			t.Fatalf("%s exposes public IPv4 target %q; use placeholders or env vars", path, match)
		}
		if strings.Contains(text, `KEY_PATH="${KEY_PATH:-`) || strings.Contains(text, `KEY="${HOME}`) {
			t.Fatalf("%s must not provide a default private key path", path)
		}
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
