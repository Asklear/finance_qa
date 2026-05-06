package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBootstrapNormalizesLegacyTableCommentsIntoStructuredSourceMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "source-metadata.sqlite")
	if err := Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	legacyComments := []string{
		`INSERT OR REPLACE INTO meta_table_comments(table_name, comment) VALUES ('fin_contracts', '合同维表：整合优集收入支出月度计算表和优集资金收入计算表的所有合同')`,
		`INSERT OR REPLACE INTO meta_table_comments(table_name, comment) VALUES ('fin_fund_income', '资金到账明细表：来自优集资金收入计算表-副本.xlsx的【25年Q4收入明细】和【26年Q1收入明细】sheet')`,
		`INSERT OR REPLACE INTO meta_table_comments(table_name, comment) VALUES ('fin_cost_settlements', '成本结算明细表：来自优集成本计算表-4.23-池.xlsx的【成本-月度结算】sheet')`,
		`INSERT OR REPLACE INTO meta_table_comments(table_name, comment) VALUES ('fin_journal', '序时账/凭证明细（借贷分录）')`,
	}
	for _, stmt := range legacyComments {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed legacy comment failed: %v", err)
		}
	}

	if err := Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("re-bootstrap db: %v", err)
	}

	meta, err := LoadTableSourceMetadata(ctx, db, dbPath, []string{
		"fin_contracts",
		"fin_fund_income",
		"fin_cost_settlements",
		"fin_journal",
	})
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}

	fund := meta["fin_fund_income"]
	if fund.Display != "《优集资金收入计算表-副本.xlsx》的【25年Q4收入明细】和【26年Q1收入明细】" {
		t.Fatalf("fund income display = %q", fund.Display)
	}
	if len(fund.FileNames) != 1 || fund.FileNames[0] != "优集资金收入计算表-副本.xlsx" {
		t.Fatalf("fund income file names = %#v", fund.FileNames)
	}
	if len(fund.SheetNames) != 2 || fund.SheetNames[0] != "25年Q4收入明细" || fund.SheetNames[1] != "26年Q1收入明细" {
		t.Fatalf("fund income sheet names = %#v", fund.SheetNames)
	}

	cost := meta["fin_cost_settlements"]
	if cost.Display != "《优集成本计算表-4.23-池.xlsx》的【成本-月度结算】" {
		t.Fatalf("cost display = %q", cost.Display)
	}
	if len(cost.FileNames) != 1 || cost.FileNames[0] != "优集成本计算表-4.23-池.xlsx" {
		t.Fatalf("cost file names = %#v", cost.FileNames)
	}

	contracts := meta["fin_contracts"]
	if contracts.Display != "《合同信息表》" {
		t.Fatalf("contract display = %q", contracts.Display)
	}
	if len(contracts.FileNames) != 2 {
		t.Fatalf("contract file names = %#v", contracts.FileNames)
	}
	if !strings.Contains(strings.Join(contracts.FileNames, ","), "优集资金收入计算表-副本.xlsx") {
		t.Fatalf("contract file names should inherit fund workbook, got %#v", contracts.FileNames)
	}
	if !strings.Contains(strings.Join(contracts.FileNames, ","), "优集成本计算表-4.23-池.xlsx") {
		t.Fatalf("contract file names should inherit cost workbook, got %#v", contracts.FileNames)
	}

	journal := meta["fin_journal"]
	if journal.Display != "《序时帐》" {
		t.Fatalf("journal display = %q", journal.Display)
	}
}

func TestBootstrapUpgradesStructuredSourceMetadataWithMissingDescription(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "source-metadata-upgrade.sqlite")
	if err := Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	legacyStructured := `financeqa_source: {"version":"v1","display":"《利润表》","logical_label":"income_statement","report_types":["income_statement"],"updated_at":"2026-04-23T06:38:46Z"}`
	if _, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO meta_table_comments(table_name, comment) VALUES ('fin_income_statement', ?)`, legacyStructured); err != nil {
		t.Fatalf("seed legacy structured comment failed: %v", err)
	}

	if err := Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("re-bootstrap db: %v", err)
	}

	var raw string
	if err := db.QueryRowContext(ctx, `SELECT comment FROM meta_table_comments WHERE table_name = 'fin_income_statement'`).Scan(&raw); err != nil {
		t.Fatalf("load upgraded comment: %v", err)
	}
	if !strings.Contains(raw, `"description":"利润表导入结果，按公司、会计期间和项目存储本期发生额与累计发生额。`) {
		t.Fatalf("upgraded comment should include description, got %q", raw)
	}
}

func TestBootstrapRemovesFeishuTokenOnlyWorkbookNamesFromSourceMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "source-metadata-token-cleanup.sqlite")
	if err := Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	dirtyFund := `financeqa_source: {"version":"v1","display":"《优集资金收入计算表-2026.xlsx》；《Iel5bFZWSoGF7hxjyPpcn5Elnqd.xlsx》","logical_label":"fin_fund_income","file_names":["优集资金收入计算表-2026.xlsx","Iel5bFZWSoGF7hxjyPpcn5Elnqd.xlsx"],"sheet_names":["26年Q1收入明细"],"report_types":["contract_fund_income"],"updated_at":"2026-05-05T11:38:30Z"}`
	dirtyCost := `financeqa_source: {"version":"v1","display":"《优集成本计算表-4.23-池.xlsx》；《Iel5bFZWSoGF7hxjyPpcn5Elnqd.xlsx》","logical_label":"fin_cost_settlements","file_names":["优集成本计算表-4.23-池.xlsx","Iel5bFZWSoGF7hxjyPpcn5Elnqd.xlsx"],"sheet_names":["成本-月度结算"],"report_types":["contract_revenue_cost"],"updated_at":"2026-05-05T11:38:30Z"}`
	for tableName, comment := range map[string]string{
		"fin_fund_income":      dirtyFund,
		"fin_cost_settlements": dirtyCost,
	} {
		if _, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO meta_table_comments(table_name, comment) VALUES (?, ?)`, tableName, comment); err != nil {
			t.Fatalf("seed dirty comment for %s: %v", tableName, err)
		}
	}

	if err := Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("re-bootstrap db: %v", err)
	}

	meta, err := LoadTableSourceMetadata(ctx, db, dbPath, []string{"fin_fund_income", "fin_cost_settlements", "fin_contracts"})
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	for tableName, got := range meta {
		if strings.Contains(got.Display, "Iel5bFZWSoGF7hxjyPpcn5Elnqd") {
			t.Fatalf("%s display should hide Feishu token-only workbook, got %q", tableName, got.Display)
		}
		if strings.Contains(strings.Join(got.FileNames, ","), "Iel5bFZWSoGF7hxjyPpcn5Elnqd") {
			t.Fatalf("%s file_names should remove Feishu token-only workbook, got %#v", tableName, got.FileNames)
		}
	}
	if meta["fin_fund_income"].Display != "《优集资金收入计算表-2026.xlsx》的【26年Q1收入明细】" {
		t.Fatalf("fund display = %q", meta["fin_fund_income"].Display)
	}
	if meta["fin_cost_settlements"].Display != "《优集成本计算表-4.23-池.xlsx》的【成本-月度结算】" {
		t.Fatalf("cost display = %q", meta["fin_cost_settlements"].Display)
	}
}

func TestFormatPostgresTableCommentSQLQuotesPayloadSafely(t *testing.T) {
	t.Parallel()

	sql := formatPostgresTableCommentSQL("tenant_uhub", "fin_fund_income", `financeqa_source: {"display":"《优集资金收入计算表-副本.xlsx》's"}`)
	if !strings.Contains(sql, `COMMENT ON TABLE "tenant_uhub"."fin_fund_income" IS 'financeqa_source: {"display":"《优集资金收入计算表-副本.xlsx》''s"}'`) {
		t.Fatalf("unexpected comment sql: %s", sql)
	}
}
