package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFinanceBridgeListToolsAndV2Response(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-contract.sqlite")

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
	if err := os.WriteFile(skillPath, []byte(sampleSkillMarkdown()), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(appendixPath), 0o755); err != nil {
		t.Fatalf("mkdir appendix dir: %v", err)
	}
	if err := os.WriteFile(appendixPath, []byte("# appendix"), 0o644); err != nil {
		t.Fatalf("write appendix: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}
	skillContractVersion, bridgeProtocolVersion := readSkillContractVersions(t, skillPath)

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
	if pv := bridgeMeta["protocol_version"]; pv != bridgeProtocolVersion {
		t.Fatalf("protocol_version should be %s, got %v", bridgeProtocolVersion, pv)
	}
	if sv := bridgeMeta["skill_contract_version"]; sv != skillContractVersion {
		t.Fatalf("skill_contract_version should be %s, got %v", skillContractVersion, sv)
	}
	if rel := bridgeMeta["skill_appendix_relative_path"]; rel != "docs/SKILL_APPENDIX_FULL.md" {
		t.Fatalf("skill_appendix_relative_path should be docs/SKILL_APPENDIX_FULL.md, got %v", rel)
	}
	resolvedAppendixPath, err := filepath.EvalSymlinks(appendixPath)
	if err != nil {
		t.Fatalf("eval appendix symlink: %v", err)
	}
	if abs := bridgeMeta["skill_appendix_path"]; abs != resolvedAppendixPath {
		t.Fatalf("skill_appendix_path should be %s, got %v", resolvedAppendixPath, abs)
	}
	if exists := bridgeMeta["skill_appendix_exists"]; exists != true {
		t.Fatalf("skill_appendix_exists should be true, got %v", exists)
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
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-rollforward.sqlite")

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
	if err := os.WriteFile(skillPath, []byte(sampleSkillMarkdown()), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(appendixPath), 0o755); err != nil {
		t.Fatalf("mkdir appendix dir: %v", err)
	}
	if err := os.WriteFile(appendixPath, []byte("# appendix"), 0o644); err != nil {
		t.Fatalf("write appendix: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}
	skillContractVersion, bridgeProtocolVersion := readSkillContractVersions(t, skillPath)

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
	bridgeMeta := mustMapMap(t, payload, "bridge_meta")
	if pv := bridgeMeta["protocol_version"]; pv != bridgeProtocolVersion {
		t.Fatalf("fallback protocol_version should be %s, got %v", bridgeProtocolVersion, pv)
	}
	if sv := bridgeMeta["skill_contract_version"]; sv != skillContractVersion {
		t.Fatalf("fallback skill_contract_version should be %s, got %v", skillContractVersion, sv)
	}
	if exists := bridgeMeta["skill_appendix_exists"]; exists != true {
		t.Fatalf("skill_appendix_exists should be true, got %v", exists)
	}
}

func TestFinanceBridgeReceiptBossReplyKeepsCumulativeAndSubPeriodSeparate(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-receipts.sqlite")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "query" ]]; then
  cat <<'JSON'
{"success":true,"message":"[金程]（识别为[customer]） 今年到账 6647398.33 元；其中3月到账 2130771.59 元。数据库能确认这类到账包含历史应收回款因素，不能直接当成当期新收入。","answer_method":"sql","data":{"entity":"金程","role":"customer","amount":6647398.33,"total":6647398.33,"bank_in":6647398.33,"sub_period":"2026-03","sub_period_receipts":2130771.59,"comparison_basis":"historical_receipt_and_current_revenue"},"executed_sql":["SELECT 1"],"calculation_logs":["calc-ok"]}
JSON
  exit 0
fi
if [[ "$cmd" == "host-data" ]]; then
  echo '{"success":true,"answer_method":"llm_payload","data":{"llm_payload":{}}}'
  exit 0
fi
echo "unknown cmd: $cmd" >&2
exit 2
`
	if err := os.WriteFile(stubBin, []byte(stubScript), 0o755); err != nil {
		t.Fatalf("write stub bin: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte(sampleSkillMarkdown()), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(appendixPath), 0o755); err != nil {
		t.Fatalf("mkdir appendix dir: %v", err)
	}
	if err := os.WriteFile(appendixPath, []byte("# appendix"), 0o644); err != nil {
		t.Fatalf("write appendix: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"金程今年回款多少？其中3月到账多少？"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)

	bossReply := mustMapMap(t, payload, "boss_reply")
	conclusion, _ := bossReply["结论"].(string)
	if !strings.Contains(conclusion, "6647398.33") {
		t.Fatalf("boss reply should preserve cumulative amount, got %s", conclusion)
	}
	if !strings.Contains(conclusion, "2130771.59") {
		t.Fatalf("boss reply should preserve sub-period amount, got %s", conclusion)
	}
	if strings.Contains(conclusion, "全部在 3 月到账") {
		t.Fatalf("boss reply should not collapse cumulative receipts into sub-period receipts, got %s", conclusion)
	}

	contract := mustMapMap(t, payload, "host_summary_contract")
	if kind := contract["kind"]; kind != "counterparty_receipts_with_subperiod" {
		t.Fatalf("host_summary_contract.kind should be counterparty_receipts_with_subperiod, got %v", kind)
	}
	if total := contract["total_amount"]; total != float64(6647398.33) {
		t.Fatalf("host_summary_contract.total_amount should be 6647398.33, got %v", total)
	}
	if sub := contract["sub_period_amount"]; sub != float64(2130771.59) {
		t.Fatalf("host_summary_contract.sub_period_amount should be 2130771.59, got %v", sub)
	}
}

func TestRepositorySkillDocumentPublishesContractVersions(t *testing.T) {
	t.Parallel()

	skillPath := filepath.Join("..", "..", "SKILL.md")
	skillContractVersion, bridgeProtocolVersion := readSkillContractVersions(t, skillPath)
	if strings.TrimSpace(skillContractVersion) == "" {
		t.Fatalf("skill_contract_version should not be empty")
	}
	if strings.TrimSpace(bridgeProtocolVersion) == "" {
		t.Fatalf("bridge_protocol_version should not be empty")
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

func sampleSkillMarkdown() string {
	return `---
name: finance
description: "Use when OpenClaw or Claude needs to call finance_qa."
---

# finance_qa 调用契约

## 0. 契约版本

1. ` + "`skill_contract_version`: `2026-04-20.1`" + `
2. ` + "`bridge_protocol_version`: `v2`" + `

## 附录

1. ` + "`docs/SKILL_APPENDIX_FULL.md`" + `
`
}

func readSkillContractVersions(t *testing.T, skillPath string) (string, string) {
	t.Helper()
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}
	return captureSkillVersion(t, string(raw), "skill_contract_version"), captureSkillVersion(t, string(raw), "bridge_protocol_version")
}

func captureSkillVersion(t *testing.T, skillDoc, key string) string {
	t.Helper()
	pattern := regexp.MustCompile("`" + regexp.QuoteMeta(key) + "`:\\s*`([^`]+)`")
	match := pattern.FindStringSubmatch(skillDoc)
	if len(match) != 2 {
		t.Fatalf("missing %s in skill doc", key)
	}
	return match[1]
}
