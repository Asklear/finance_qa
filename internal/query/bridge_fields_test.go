package query

import (
	"testing"

	_ "modernc.org/sqlite"
)

func TestHostSummaryContractFieldExists(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月应收账款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s", res.Message)
	}

	// 检查 final_answer 字段
	if _, ok := res.Data["final_answer"]; !ok {
		t.Errorf("final_answer field missing from Data")
	}

	// 检查 host_summary_contract 字段
	if _, ok := res.Data["host_summary_contract"]; !ok {
		t.Errorf("host_summary_contract field missing from Data")
	} else {
		hsc := res.Data["host_summary_contract"].(map[string]any)
		if kind := hsc["kind"]; kind != "contract_aggregate" {
			t.Errorf("host_summary_contract.kind = %v, want contract_aggregate", kind)
		}
	}
}
