package query_test

import (
	"database/sql"
	"strings"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestCompanyAggregateMetricPrimarySourceTablesPreferMetricTableOverContractDimension(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		_ = db.Close()
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()
	if _, err := db.Exec(`
UPDATE fin_fund_income SET updated_at = '2026-05-05 08:30:00';
UPDATE fin_fund_income_groups SET updated_at = '2026-05-05 09:00:00';
UPDATE fin_contracts SET updated_at = '2026-05-04 18:00:00';
`); err != nil {
		_ = db.Close()
		t.Fatalf("seed source updated_at: %v", err)
	}
	_ = db.Close()

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
	if !containsString(sourceTables, "tenant_uhub.fin_fund_income_groups") || !containsString(sourceTables, "tenant_uhub.fin_fund_income_group_members") {
		t.Fatalf("source_tables should include merged fund income group tables, got %#v", sourceTables)
	}
	if containsString(sourceTables, "tenant_uhub.fin_cost_settlement_groups") || containsString(sourceTables, "tenant_uhub.fin_cost_settlement_group_members") {
		t.Fatalf("source_tables should not include cost group tables for revenue-only aggregate, got %#v", sourceTables)
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
	sourceUpdateNote, _ := res.Data["source_update_note"].(string)
	if sourceUpdateNote == "" || !strings.Contains(sourceUpdateNote, "来源更新时间：") || !strings.Contains(sourceUpdateNote, "2026-05-05 09:00:00") {
		t.Fatalf("source_update_note should expose latest source update time, got %q", sourceUpdateNote)
	}
	if !strings.Contains(res.Message, sourceUpdateNote) {
		t.Fatalf("message should append source update note %q, got %q", sourceUpdateNote, res.Message)
	}
}

func TestCompanyAggregateMultiMetricSourceNoteIncludesRevenueAndCostWorkbooks(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月收入、成本、利润分别是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	primary, ok := res.Data["primary_source_tables"].([]string)
	if !ok {
		t.Fatalf("primary_source_tables missing: %#v", res.Data["primary_source_tables"])
	}
	if !containsString(primary, "tenant_uhub.fin_fund_income") || !containsString(primary, "tenant_uhub.fin_cost_settlements") {
		t.Fatalf("primary_source_tables should include revenue and cost tables, got %#v", primary)
	}
	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing: %#v", res.Data["source_tables"])
	}
	for _, tableName := range []string{
		"tenant_uhub.fin_fund_income_groups",
		"tenant_uhub.fin_fund_income_group_members",
		"tenant_uhub.fin_cost_settlement_groups",
		"tenant_uhub.fin_cost_settlement_group_members",
	} {
		if !containsString(sourceTables, tableName) {
			t.Fatalf("source_tables should include %s, got %#v", tableName, sourceTables)
		}
	}

	sourceNote, _ := res.Data["source_note"].(string)
	if !containsAll(sourceNote, "优集资金收入计算表-副本.xlsx", "优集成本计算表-4.23-池.xlsx") {
		t.Fatalf("source_note should include both revenue and cost workbook lineage, got %q", sourceNote)
	}
	if !strings.Contains(sourceNote, "补充参考：《合同信息表》") {
		t.Fatalf("source_note should keep contract table as supporting source, got %q", sourceNote)
	}
}

func TestSourceNoteDoesNotExposeMergedGroupHelperTablesToBossReply(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
INSERT OR REPLACE INTO meta_table_comments(table_name, comment) VALUES
('fin_cost_settlement_groups', '合同成本结算合并金额组，记录 Excel 合并单元格代表的供应商级成本事实，不拆分到单个合同。'),
('fin_cost_settlement_group_members', '合同成本结算合并金额组成员表，关联合并金额组与其覆盖的真实合同。')
`); err != nil {
		_ = db.Close()
		t.Fatalf("seed helper table comments: %v", err)
	}
	_ = db.Close()

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("南京林悦智能科技有限公司的应收/应付是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing: %#v", res.Data["source_tables"])
	}
	if len(sourceTables) == 0 {
		t.Fatalf("source_tables should not be empty")
	}

	sourceNote, _ := res.Data["source_note"].(string)
	if strings.Contains(sourceNote, "合并金额组") || strings.Contains(res.Message, "合并金额组") {
		t.Fatalf("boss-facing source should not expose helper table descriptions, source_note=%q message=%q", sourceNote, res.Message)
	}
	if !strings.Contains(sourceNote, "优集成本计算表") {
		t.Fatalf("source_note should still show business workbook source, got %q", sourceNote)
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
