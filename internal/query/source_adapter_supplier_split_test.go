package query

import "testing"

func TestBuildSupplierPaymentTracePayloadKeepsSummaryDetails(t *testing.T) {
	summary := supplierPaymentSummary{
		Period: "2026-03",
		Total:  1500,
		Suppliers: []map[string]any{
			{"name": "供应商A有限公司", "out_amount": 1000.0},
		},
		Excluded: []map[string]any{
			{"name": "暂收款", "exclude_reason": "non_counterparty_flow"},
		},
		ExecutedSQL: []string{"SELECT supplier payments"},
		Logs:        []string{"[供应商付款] period=2026-03"},
	}

	payload := buildSupplierPaymentTracePayload(summary)
	if got := payload["period"]; got != "2026-03" {
		t.Fatalf("period = %v, want 2026-03", got)
	}
	suppliers, ok := payload["suppliers"].([]map[string]any)
	if !ok || len(suppliers) != 1 {
		t.Fatalf("suppliers payload = %#v", payload["suppliers"])
	}
	excluded, ok := payload["excluded"].([]map[string]any)
	if !ok || len(excluded) != 1 {
		t.Fatalf("excluded payload = %#v", payload["excluded"])
	}
	sqls, ok := payload["executed_sql"].([]string)
	if !ok || len(sqls) != 1 {
		t.Fatalf("executed_sql payload = %#v", payload["executed_sql"])
	}
	logs, ok := payload["logs"].([]string)
	if !ok || len(logs) != 1 {
		t.Fatalf("logs payload = %#v", payload["logs"])
	}
}
