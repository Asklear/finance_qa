package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFinanceBridgeListToolsAndV2Response(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	dbPath := filepath.Join(tmp, "finance.db")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
all="$*"
if [[ "$cmd" == "query" ]]; then
  if [[ "$all" == *"FAIL_ME"* ]]; then
    echo "query failed for test" >&2
    exit 1
  fi
  cat <<'JSON'
{"success":true,"message":"ok","answer_method":"sql","data":{"metric":"人力成本","period":"2026-03","total":68600,"hr_breakdown":{"wage":64300,"social_security":3669,"housing_fund":639},"arithmetic_checks":{"status":"pass"},"intent_trace":{"final_intent":"human_cost"}},"executed_sql":["SELECT 1"],"calculation_logs":["calc-ok"]}
JSON
  exit 0
fi
if [[ "$cmd" == "host-data" ]]; then
  cat <<'JSON'
{"success":true,"message":"host","answer_method":"llm_payload","data":{"llm_payload":{"financial_tables":{"balance_detail":[{"period":"2026-03","account_code":"1122"}]}}},"executed_sql":["SELECT host"],"calculation_logs":["host-ok"]}
JSON
  exit 0
fi
if [[ "$cmd" == "import" ]]; then
  echo '{"success":true,"recordCount":1}'
  exit 0
fi
echo "unknown cmd: $cmd" >&2
exit 2
`
	if err := os.WriteFile(stubBin, []byte(stubScript), 0o755); err != nil {
		t.Fatalf("write stub bin: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte(`name: finance
description: "version 1.2.3"
`), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	listReq := `{"action":"list"}`
	listRaw := runBridge(t, stubBin, dbPath, skillPath, listReq)
	var listObj map[string]any
	if err := json.Unmarshal([]byte(listRaw), &listObj); err != nil {
		t.Fatalf("unmarshal list: %v; raw=%s", err, listRaw)
	}
	tools, ok := listObj["tools"].([]any)
	if !ok || len(tools) < 3 {
		t.Fatalf("expected >=3 tools, got %v", listObj["tools"])
	}

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"2026年3月人力成本多少"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)

	if v := mustMapValue(t, payload, "success"); v != true {
		t.Fatalf("success should be true, got %v", v)
	}
	if method := mustMapValue(t, payload, "answer_method"); method != "sql" {
		t.Fatalf("answer_method should be sql, got %v", method)
	}
	bridgeMeta := mustMapMap(t, payload, "bridge_meta")
	if pv := bridgeMeta["protocol_version"]; pv != "v2" {
		t.Fatalf("protocol_version should be v2, got %v", pv)
	}
	data := mustMapMap(t, payload, "data")
	if _, ok := data["trace"].(map[string]any); !ok {
		t.Fatalf("trace should exist in data, got %T", data["trace"])
	}
	exposed := mustMapMap(t, data, "exposed_fields")
	if _, ok := exposed["hr_breakdown"].(map[string]any); !ok {
		t.Fatalf("exposed_fields.hr_breakdown should exist, got %T", exposed["hr_breakdown"])
	}
	if _, ok := exposed["arithmetic_checks"].(map[string]any); !ok {
		t.Fatalf("exposed_fields.arithmetic_checks should exist, got %T", exposed["arithmetic_checks"])
	}
}

func TestFinanceBridgeFallbackToHostData(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	dbPath := filepath.Join(tmp, "finance.db")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "query" ]]; then
  echo "query failed for test" >&2
  exit 1
fi
if [[ "$cmd" == "host-data" ]]; then
  cat <<'JSON'
{"success":true,"message":"host","answer_method":"llm_payload","data":{"llm_payload":{"financial_tables":{"balance_detail":[{"period":"2026-03"}]}}},"executed_sql":["SELECT host"],"calculation_logs":["host-ok"]}
JSON
  exit 0
fi
echo '{"success":true}'
`
	if err := os.WriteFile(stubBin, []byte(stubScript), 0o755); err != nil {
		t.Fatalf("write stub bin: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("name: finance\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"FAIL_ME"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)
	if v := mustMapValue(t, payload, "success"); v != false {
		t.Fatalf("success should be false in fallback, got %v", v)
	}
	if method := mustMapValue(t, payload, "answer_method"); method != "llm_payload" {
		t.Fatalf("answer_method should be llm_payload, got %v", method)
	}
	data := mustMapMap(t, payload, "data")
	if _, ok := data["llm_payload"].(map[string]any); !ok {
		t.Fatalf("fallback should include llm_payload, got %T", data["llm_payload"])
	}
	trace := mustMapMap(t, data, "trace")
	logs, ok := trace["calculation_logs"].([]any)
	if !ok || len(logs) == 0 {
		t.Fatalf("fallback should include trace.calculation_logs, got %v", trace["calculation_logs"])
	}
}

func runBridge(t *testing.T, binPath, dbPath, skillPath, reqJSON string) string {
	t.Helper()
	bridgePath := filepath.Join("..", "..", "plugin", "openclaw-finance", "server", "finance_bridge.py")
	cmd := exec.Command("python3", bridgePath)
	cmd.Env = append(os.Environ(),
		"FINANCEQA_BIN="+binPath,
		"FINANCEQA_DB="+dbPath,
		"FINANCEQA_SKILL_PATH="+skillPath,
	)
	cmd.Stdin = strings.NewReader(reqJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run bridge failed: %v, stderr=%s, stdout=%s", err, stderr.String(), stdout.String())
	}
	return stdout.String()
}

func parseBridgeContentPayload(t *testing.T, raw string) map[string]any {
	t.Helper()
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		t.Fatalf("unmarshal wrapper failed: %v; raw=%s", err, raw)
	}
	content, ok := wrapper["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("missing content in wrapper: %v", wrapper)
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] should be object, got %T", content[0])
	}
	text, _ := first["text"].(string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v; text=%s", err, text)
	}
	return payload
}

func mustMapValue(t *testing.T, m map[string]any, key string) any {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %s", key)
	}
	return v
}

func mustMapMap(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v := mustMapValue(t, m, key)
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("key %s should be object, got %T", key, v)
	}
	return obj
}

