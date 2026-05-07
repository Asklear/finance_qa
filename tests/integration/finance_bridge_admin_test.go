package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFinanceBridgeSyncToolReturnsStructuredPayload(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-sync.sqlite")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "sync" ]]; then
  cat <<'JSON'
{"processed":[{"file":"a.xlsx","recordCount":12}],"processedCount":1,"importedCount":12,"successCount":1,"failedCount":0}
JSON
  exit 0
fi
if [[ "$cmd" == "query" || "$cmd" == "host-data" ]]; then
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

	callReq := `{"action":"call","name":"finance-sync","arguments":{"directoryPath":"/tmp/reports","incremental":true,"company":"南京优集数据科技有限公司"}}`
	callRaw := runBridge(t, stubBin, dbPath, skillPath, callReq)
	payload := parseBridgeContentPayload(t, callRaw)

	if payload["success"] != true {
		t.Fatalf("sync payload should mark success=true, got %v", payload["success"])
	}
	if payload["answer_method"] != "mcp_json" {
		t.Fatalf("sync answer_method should be mcp_json, got %v", payload["answer_method"])
	}
	if payload["tool_name"] != "finance-sync" {
		t.Fatalf("sync tool_name should be finance-sync, got %v", payload["tool_name"])
	}
	bridgeMeta := mustMapMap(t, payload, "bridge_meta")
	if bridgeMeta["tool_name"] != "finance-sync" {
		t.Fatalf("bridge_meta.tool_name should be finance-sync, got %v", bridgeMeta["tool_name"])
	}
	if bridgeMeta["tool_operation"] != "sync" {
		t.Fatalf("bridge_meta.tool_operation should be sync, got %v", bridgeMeta["tool_operation"])
	}
	capabilities := mustMapMap(t, bridgeMeta, "capabilities")
	if exposedTools, ok := capabilities["exposed_tools"].([]any); !ok || len(exposedTools) != 5 {
		t.Fatalf("bridge sync payload should include 5 exposed tools, got %v", capabilities["exposed_tools"])
	}
	processed, ok := payload["processed"].([]any)
	if !ok || len(processed) != 1 {
		t.Fatalf("sync payload should preserve processed entries, got %v", payload["processed"])
	}
}

func TestFinanceBridgeDimensionsToolSupportsJSONAndTextSubcommands(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "financeqa_stub.sh")
	skillPath := filepath.Join(tmp, "SKILL.md")
	appendixPath := filepath.Join(tmp, "docs", "SKILL_APPENDIX_FULL.md")
	dbPath := filepath.Join(tmp, "bridge-dimensions.sqlite")

	stubScript := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
sub="${1:-}"
if [[ "$cmd" == "dimensions" && "$sub" == "list" ]]; then
  cat <<'JSON'
{"data":[{"code":"project","name":"项目","type":"project"}],"total":1}
JSON
  exit 0
fi
if [[ "$cmd" == "dimensions" && "$sub" == "seed-standard" ]]; then
  echo "successfully seeded standard CAS rules for 南京优集数据科技有限公司"
  exit 0
fi
if [[ "$cmd" == "query" || "$cmd" == "host-data" ]]; then
  echo '{"success":true,"answer_method":"llm_payload","data":{"llm_payload":{}}}'
  exit 0
fi
echo "unknown cmd: $cmd/$sub" >&2
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

	listReq := `{"action":"call","name":"finance-dimensions","arguments":{"subcommand":"list"}}`
	listRaw := runBridge(t, stubBin, dbPath, skillPath, listReq)
	listPayload := parseBridgeContentPayload(t, listRaw)

	if listPayload["success"] != true {
		t.Fatalf("dimensions list should mark success=true, got %v", listPayload["success"])
	}
	if listPayload["answer_method"] != "mcp_json" {
		t.Fatalf("dimensions list answer_method should be mcp_json, got %v", listPayload["answer_method"])
	}
	data, ok := listPayload["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("dimensions list should preserve data array, got %v", listPayload["data"])
	}
	listMeta := mustMapMap(t, listPayload, "bridge_meta")
	if listMeta["tool_name"] != "finance-dimensions" {
		t.Fatalf("bridge_meta.tool_name should be finance-dimensions, got %v", listMeta["tool_name"])
	}
	if listMeta["tool_operation"] != "dimensions:list" {
		t.Fatalf("bridge_meta.tool_operation should be dimensions:list, got %v", listMeta["tool_operation"])
	}

	seedReq := `{"action":"call","name":"finance-dimensions","arguments":{"subcommand":"seed-standard","company":"南京优集数据科技有限公司"}}`
	seedRaw := runBridge(t, stubBin, dbPath, skillPath, seedReq)
	seedPayload := parseBridgeContentPayload(t, seedRaw)

	if seedPayload["answer_method"] != "mcp_text" {
		t.Fatalf("dimensions seed-standard answer_method should be mcp_text, got %v", seedPayload["answer_method"])
	}
	msg, _ := seedPayload["message"].(string)
	if !strings.Contains(msg, "successfully seeded standard CAS rules") {
		t.Fatalf("seed-standard message should preserve stdout text, got %s", msg)
	}
	seedMeta := mustMapMap(t, seedPayload, "bridge_meta")
	if seedMeta["tool_operation"] != "dimensions:seed-standard" {
		t.Fatalf("bridge_meta.tool_operation should be dimensions:seed-standard, got %v", seedMeta["tool_operation"])
	}
}
