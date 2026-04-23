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
	if db := bridgeMeta["db"]; db != "sqlite(local)" {
		t.Fatalf("bridge_meta.db should be redacted sqlite(local), got %v", db)
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

func TestFinanceBridgeRedactsBridgeMetaDBForPostgres(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	pgDSN := "host=db.example.internal port=5432 user=finance password=super-secret dbname=finance_prod search_path=tenant_uhub,public"

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "query" ]]; then
  cat <<'JSON'
{"success":true,"message":"ok","answer_method":"sql","data":{"metric":"营收","period":"2026-03","total":3106310.34},"executed_sql":["SELECT 1"],"calculation_logs":["calc-ok"]}
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

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"2026年3月营收是多少"}}`
	callRaw := runBridgeWithDBTarget(t, stubBin, pgDSN, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)
	bridgeMeta := mustMapMap(t, payload, "bridge_meta")

	db, _ := bridgeMeta["db"].(string)
	if db != "postgresql(schema=tenant_uhub)" {
		t.Fatalf("bridge_meta.db should be redacted postgresql(schema=tenant_uhub), got %q", db)
	}
	for _, forbidden := range []string{"db.example.internal", "super-secret", "finance_prod", "user=finance"} {
		if strings.Contains(db, forbidden) {
			t.Fatalf("bridge_meta.db should not leak %q, got %q", forbidden, db)
		}
	}
}

func TestFinanceBridgeBossReplyPrefersContractRevenueSummary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-contract-revenue.sqlite")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "query" ]]; then
  cat <<'JSON'
{"success":true,"message":"[飞未云科（深圳）技术有限公司] 2026-01~2026-12 先看现金口径：实际到账 1667200.00 元。再看财务口径：合同台账结算 1667200.00 元，开票 1667200.00 元。","answer_method":"sql","data":{"entity":"飞未云科（深圳）技术有限公司","role":"customer_contract","period":"2026-01~2026-12","asked_topic":"revenue","source_tables":["tenant_uhub.fin_contracts","tenant_uhub.fin_fund_income"],"cash_view":{"received_amount":1667200,"view":"bank_cash_collection"},"book_view":{"settlement_amount":1667200,"invoice_amount":1667200,"view":"contract_ledger"},"contracts":[{"contract_id":"C007","customer_name":"飞未云科（深圳）技术有限公司","contract_content":"全品类商品价格数据-京东"}],"query_spec":{"query_family":"contract_dimension","metric_kind":"revenue","needs_contract_dimension":true}},"executed_sql":["SELECT contract"],"calculation_logs":["contract-ok"]}
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

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"飞未云科(深圳)技术有限公司2026年营收多少？"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)

	contract := mustMapMap(t, payload, "host_summary_contract")
	if kind := contract["kind"]; kind != "contract_dimension" {
		t.Fatalf("host_summary_contract.kind should be contract_dimension, got %v", kind)
	}
	if topic := contract["asked_topic"]; topic != "revenue" {
		t.Fatalf("host_summary_contract.asked_topic should be revenue, got %v", topic)
	}
	sourceTables, ok := contract["source_tables"].([]any)
	if !ok || len(sourceTables) == 0 || sourceTables[0] != "tenant_uhub.fin_contracts" {
		t.Fatalf("host_summary_contract.source_tables should start with tenant_uhub.fin_contracts, got %#v", contract["source_tables"])
	}

	bossReply := mustMapMap(t, payload, "boss_reply")
	conclusion, _ := bossReply["结论"].(string)
	reason, _ := bossReply["原因"].(string)
	if !strings.Contains(conclusion, "合同到账 1667200.00 元") {
		t.Fatalf("boss reply should use contract cash summary, got %s", conclusion)
	}
	if !strings.Contains(conclusion, "合同结算 1667200.00 元") {
		t.Fatalf("boss reply should use contract settlement summary, got %s", conclusion)
	}
	if !strings.Contains(reason, "tenant_uhub.fin_contracts") {
		t.Fatalf("boss reply reason should mention tenant_uhub.fin_contracts, got %s", reason)
	}
	if strings.Contains(conclusion, "账上确认收入") {
		t.Fatalf("boss reply should not fall back to generic counterparty summary, got %s", conclusion)
	}
}

func TestFinanceBridgeBossReplyPrefersContractProfitSummary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-contract-profit.sqlite")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "query" ]]; then
  cat <<'JSON'
{"success":true,"message":"[飞未云科（深圳）技术有限公司] 2026-01~2026-12 当前合同台账只匹配到收入/回款，未匹配到合同成本，暂不能直接给完整合同利润。先看现金口径：实际到账 1667200.00 元。再看经营口径：合同结算 1667200.00 元，开票 1667200.00 元。","answer_method":"sql","data":{"entity":"飞未云科（深圳）技术有限公司","role":"customer_contract","period":"2026-01~2026-12","asked_topic":"profit","source_tables":["tenant_uhub.fin_contracts","tenant_uhub.fin_fund_income"],"cash_view":{"received_amount":1667200,"view":"bank_cash_collection"},"book_view":{"settlement_amount":1667200,"invoice_amount":1667200,"view":"contract_ledger"},"contracts":[{"contract_id":"C007","customer_name":"飞未云科（深圳）技术有限公司","contract_content":"全品类商品价格数据-京东"}],"query_spec":{"query_family":"contract_dimension","metric_kind":"profit","needs_contract_dimension":true}},"executed_sql":["SELECT contract"],"calculation_logs":["contract-ok"]}
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

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"飞未云科(深圳)技术有限公司2026年利润多少？"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)

	bossReply := mustMapMap(t, payload, "boss_reply")
	conclusion, _ := bossReply["结论"].(string)
	reason, _ := bossReply["原因"].(string)
	if !strings.Contains(conclusion, "暂不能直接给完整合同利润") {
		t.Fatalf("boss reply should preserve contract profit caution, got %s", conclusion)
	}
	if !strings.Contains(reason, "tenant_uhub.fin_contracts") {
		t.Fatalf("boss reply reason should mention tenant_uhub.fin_contracts, got %s", reason)
	}
	if strings.Contains(conclusion, "经营口径 291291.55 元") || strings.Contains(conclusion, "现金口径 -") {
		t.Fatalf("boss reply should not fall back to generic core-metric profit summary, got %s", conclusion)
	}
}

func TestFinanceBridgeBossReplyPrefersContractAggregateRevenueSummary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-contract-aggregate-revenue.sqlite")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "query" ]]; then
  cat <<'JSON'
{"success":true,"message":"2026-01~2026-03 老板口径先看合同/项目汇总：营收 12976135.11 元。补充合同现金到账 8031718.17 元，已开票 8165893.84 元。","answer_method":"sql","data":{"period":"2026-01~2026-03","metric":"收入","requested_metrics":["收入"],"source_priority":"contract_first","source_tables":["tenant_uhub.fin_contracts","tenant_uhub.fin_fund_income"],"money_view":{"说明":"合同现金口径","到账":8031718.17},"account_view":{"说明":"合同经营口径","营收":12976135.11,"已开票":8165893.84},"contract_summary":{"scope":"company","contract_count":18,"revenue_settlement":12976135.11,"revenue_received":8031718.17,"invoice_amount":8165893.84,"coverage":{"收入":true}},"source_note":"来源：《优集资金收入计算表-副本.xlsx》的【25年Q4收入明细】和【26年Q1收入明细】；补充参考：《合同信息表》","query_spec":{"query_family":"core_metric","metric_kind":"revenue","prefer_contract_aggregate":true}},"executed_sql":["SELECT aggregate revenue"],"calculation_logs":["contract-aggregate-revenue-ok"]}
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

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"2026年Q1收入是多少？"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)

	contract := mustMapMap(t, payload, "host_summary_contract")
	if kind := contract["kind"]; kind != "contract_aggregate" {
		t.Fatalf("host_summary_contract.kind should be contract_aggregate, got %v", kind)
	}

	bossReply := mustMapMap(t, payload, "boss_reply")
	conclusion, _ := bossReply["结论"].(string)
	reason, _ := bossReply["原因"].(string)
	if !strings.Contains(conclusion, "合同到账 8031718.17 元") {
		t.Fatalf("boss reply should use contract aggregate cash receipts, got %s", conclusion)
	}
	if !strings.Contains(conclusion, "合同营收 12976135.11 元") {
		t.Fatalf("boss reply should use contract aggregate revenue, got %s", conclusion)
	}
	if strings.Contains(conclusion, "核心金额约") {
		t.Fatalf("boss reply should not fall back to generic total summary, got %s", conclusion)
	}
	if !strings.Contains(reason, "tenant_uhub.fin_contracts") && !strings.Contains(reason, "优集资金收入计算表") {
		t.Fatalf("boss reply reason should mention contract-first source, got %s", reason)
	}
}

func TestFinanceBridgeBossReplyPrefersContractAggregateProfitSummary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-contract-aggregate-profit.sqlite")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "query" ]]; then
  cat <<'JSON'
{"success":true,"message":"2026-01~2026-03 老板口径先看合同/项目汇总：利润 5127739.57 元。补充合同现金净额 1823322.63 元（回款 8031718.17 元，付款 6208395.54 元）。","answer_method":"sql","data":{"period":"2026-01~2026-03","metric":"利润","requested_metrics":["利润"],"source_priority":"contract_first","source_tables":["tenant_uhub.fin_contracts","tenant_uhub.fin_fund_income","tenant_uhub.fin_cost_settlements"],"money_view":{"说明":"合同现金口径","回款":8031718.17,"付款":6208395.54,"净现金":1823322.63},"account_view":{"说明":"合同经营口径","利润":5127739.57},"contract_summary":{"scope":"company","contract_count":18,"profit":5127739.57,"revenue_received":8031718.17,"cost_paid":6208395.54,"coverage":{"利润":true}},"source_note":"来源：《优集资金收入计算表-副本.xlsx》的【25年Q4收入明细】和【26年Q1收入明细】；《优集成本计算表-4.23-池.xlsx》的【成本-月度结算】；补充参考：《合同信息表》","query_spec":{"query_family":"core_metric","metric_kind":"profit","prefer_contract_aggregate":true}},"executed_sql":["SELECT aggregate profit"],"calculation_logs":["contract-aggregate-profit-ok"]}
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

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"2026年Q1利润是多少？"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)

	contract := mustMapMap(t, payload, "host_summary_contract")
	if kind := contract["kind"]; kind != "contract_aggregate" {
		t.Fatalf("host_summary_contract.kind should be contract_aggregate, got %v", kind)
	}

	bossReply := mustMapMap(t, payload, "boss_reply")
	conclusion, _ := bossReply["结论"].(string)
	if !strings.Contains(conclusion, "净现金 1823322.63 元") {
		t.Fatalf("boss reply should use contract aggregate net cash, got %s", conclusion)
	}
	if !strings.Contains(conclusion, "合同利润 5127739.57 元") {
		t.Fatalf("boss reply should use contract aggregate profit, got %s", conclusion)
	}
	if strings.Contains(conclusion, "核心金额约") {
		t.Fatalf("boss reply should not fall back to generic total summary, got %s", conclusion)
	}
}

func TestFinanceBridgeExposesJournalTaxDisclosureToHost(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-tax-note.sqlite")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "query" ]]; then
  cat <<'JSON'
{"success":true,"message":"2026-02利润约 123.45 元。补充：该结果来自序时账汇总，默认按凭证入账金额统计，不主动剔税；若税额未单独拆分，通常应视为含税口径，需结合进销项税/发票分录复核。","answer_method":"sql","data":{"metric":"利润","period":"2026-02","account_value":123.45,"tax_inclusion":"journal_entry_gross_amount_default","tax_inclusion_note":"该结果来自序时账汇总，默认按凭证入账金额统计，不主动剔税；若税额未单独拆分，通常应视为含税口径，需结合进销项税/发票分录复核。"},"executed_sql":["SELECT journal"],"calculation_logs":["journal-tax-note"]}
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

	callReq := `{"action":"call","name":"finance-query","arguments":{"query":"2026年2月利润是多少"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)

	data := mustMapMap(t, payload, "data")
	exposed := mustMapMap(t, data, "exposed_fields")
	if exposed["tax_inclusion"] != "journal_entry_gross_amount_default" {
		t.Fatalf("exposed_fields.tax_inclusion should be journal_entry_gross_amount_default, got %v", exposed["tax_inclusion"])
	}
	if note, _ := exposed["tax_inclusion_note"].(string); !strings.Contains(note, "通常应视为含税口径") {
		t.Fatalf("exposed_fields.tax_inclusion_note should preserve journal tax note, got %v", exposed["tax_inclusion_note"])
	}

	bossReply := mustMapMap(t, payload, "boss_reply")
	reason, _ := bossReply["原因"].(string)
	if !strings.Contains(reason, "序时账汇总") || !strings.Contains(reason, "含税口径") {
		t.Fatalf("boss reply reason should include journal tax disclosure, got %s", reason)
	}

	bridgeMeta := mustMapMap(t, payload, "bridge_meta")
	capabilities := mustMapMap(t, bridgeMeta, "capabilities")
	if capabilities["tax_disclosure"] != true {
		t.Fatalf("bridge_meta.capabilities.tax_disclosure should be true, got %v", capabilities["tax_disclosure"])
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

func runBridgeWithDBTarget(t *testing.T, binPath, dbTarget, skillPath, reqJSON string) string {
	t.Helper()
	bridgePath := filepath.Join("..", "..", "plugin", "openclaw-finance", "server", "finance_bridge.py")
	cmd := exec.Command("python3", bridgePath)
	cmd.Env = append(os.Environ(),
		"FINANCEQA_BIN="+binPath,
		"FINANCEQA_DB="+dbTarget,
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
