package query_test

import (
	"strings"
	"testing"

	"financeqa/internal/query"
)

func TestCompanyAggregateMetricPrimarySourceTablesPreferMetricTableOverContractDimension(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月收入是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	primary, ok := res.Data["primary_source_tables"].([]string)
	if !ok {
		t.Fatalf("primary_source_tables missing: %#v", res.Data["primary_source_tables"])
	}
	if len(primary) != 1 || primary[0] != "tenant_uhub.fin_fund_income" {
		t.Fatalf("primary_source_tables = %#v, want only tenant_uhub.fin_fund_income", primary)
	}
	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing: %#v", res.Data["source_tables"])
	}
	for _, tableName := range sourceTables {
		if tableName == "tenant_uhub.fin_cost_settlements" {
			t.Fatalf("source_tables should not include tenant_uhub.fin_cost_settlements for revenue-only aggregate, got %#v", sourceTables)
		}
	}

	supporting, ok := res.Data["supporting_source_documents"].([]string)
	if !ok || len(supporting) == 0 {
		t.Fatalf("supporting_source_documents missing: %#v", res.Data["supporting_source_documents"])
	}
	for _, doc := range supporting {
		if doc == "《优集成本计算表-4.23-池.xlsx》" {
			t.Fatalf("supporting_source_documents should not include cost workbook for revenue-only aggregate, got %#v", supporting)
		}
	}

	partitions := extractSourcePartitionsForTest(t, res.Data["source_partitions"])
	if len(partitions) != 1 {
		t.Fatalf("source_partitions = %#v, want exactly one partition", res.Data["source_partitions"])
	}
	if got := partitions[0]["table"]; got != "tenant_uhub.fin_fund_income" {
		t.Fatalf("source partition table = %v, want tenant_uhub.fin_fund_income", got)
	}
	if got := partitions[0]["source_report_type"]; got != "contract_fund_income" {
		t.Fatalf("source partition report type = %v, want contract_fund_income", got)
	}
	if got := partitions[0]["source_sheet_name"]; got != "25年Q4收入明细" {
		t.Fatalf("source partition sheet = %v, want 25年Q4收入明细", got)
	}

	primaryPartitions := extractSourcePartitionsForTest(t, res.Data["primary_source_partitions"])
	if len(primaryPartitions) != 1 {
		t.Fatalf("primary_source_partitions = %#v, want one primary partition", res.Data["primary_source_partitions"])
	}
	if res.Data["supporting_source_partitions"] != nil {
		t.Fatalf("supporting_source_partitions should be nil for revenue-only aggregate, got %#v", res.Data["supporting_source_partitions"])
	}

	sourceSummary, _ := res.Data["source_summary"].(string)
	if sourceSummary == "" {
		t.Fatalf("source_summary missing: %#v", res.Data["source_summary"])
	}
	sourceNote, _ := res.Data["source_note"].(string)
	if sourceSummary != sourceNote {
		t.Fatalf("source_summary = %q, want match source_note %q", sourceSummary, sourceNote)
	}
	if sourceSummary == "" || !containsAll(sourceSummary, "优集资金收入计算表-副本.xlsx", "25年Q4收入明细") {
		t.Fatalf("source_summary should expose concrete workbook lineage, got %q", sourceSummary)
	}
}

func extractSourcePartitionsForTest(t *testing.T, v any) []map[string]any {
	t.Helper()

	if rows, ok := v.([]map[string]any); ok {
		return rows
	}
	rawRows, ok := v.([]any)
	if !ok {
		t.Fatalf("source partitions missing or wrong type: %#v", v)
	}
	out := make([]map[string]any, 0, len(rawRows))
	for _, raw := range rawRows {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("source partition row has wrong type: %#v", raw)
		}
		out = append(out, row)
	}
	return out
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if part == "" {
			continue
		}
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
