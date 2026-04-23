package query

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSummarizeCumulativeValidationAccumulatorUsesOneYuanTolerance(t *testing.T) {
	summary := summarizeCumulativeValidationAccumulator(cumulativeValidationAccumulator{
		CurrentSum:   100.40,
		PreviousCumu: sql.NullFloat64{Float64: 50.00, Valid: true},
		LatestCumu:   sql.NullFloat64{Float64: 151.00, Valid: true},
		PreviousAt:   "2026-01",
		LatestAt:     "2026-03",
	})

	if summary.CurrentSum != 100.40 {
		t.Fatalf("CurrentSum = %.2f, want 100.40", summary.CurrentSum)
	}
	if summary.CumulativeDelta != 101.00 {
		t.Fatalf("CumulativeDelta = %.2f, want 101.00", summary.CumulativeDelta)
	}
	if summary.Diff != -0.60 {
		t.Fatalf("Diff = %.2f, want -0.60", summary.Diff)
	}
	if !summary.Passed {
		t.Fatalf("Passed = false, want true when abs(diff) <= 1.00")
	}
}

func TestValidateIncomeStatementRangeTotalsUsesConfiguredIncomeStatementPatterns(t *testing.T) {
	rulesPath := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(rulesPath, []byte(`{
  "schema_version": 2,
  "accounting": {
    "income_statement_items": {
      "revenue": ["自定义营收"],
      "cost": ["自定义成本"],
      "profit_total": ["自定义利润总额"]
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	dbPath := filepath.Join(t.TempDir(), "range-validation.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, period TEXT, opening_period TEXT)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司', '2026-03', '2026-01')`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-01', '自定义营收', 100, 100)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-02', '自定义营收', 200, 300)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-03', '自定义营收', 300, 600)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-01', '自定义成本', 40, 40)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-02', '自定义成本', 70, 110)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-03', '自定义成本', 90, 200)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-01', '自定义利润总额', 60, 60)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-02', '自定义利润总额', 130, 190)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司', '2026-03', '自定义利润总额', 210, 400)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	validation, _, logs, err := engine.validateIncomeStatementRangeTotals("2026-02", "2026-03")
	if err != nil {
		t.Fatalf("validateIncomeStatementRangeTotals: %v", err)
	}
	if validation == nil {
		t.Fatalf("expected validation result, got nil logs=%v", logs)
	}

	items, ok := validation["items"].(map[string]any)
	if !ok {
		t.Fatalf("validation items missing: %+v", validation)
	}
	revenue, ok := items["revenue"].(map[string]any)
	if !ok {
		t.Fatalf("revenue validation missing: %+v", items)
	}
	if got := revenue["current_sum"]; got != float64(500) {
		t.Fatalf("revenue current_sum = %v, want 500", got)
	}
	if got := revenue["cumulative_delta"]; got != float64(500) {
		t.Fatalf("revenue cumulative_delta = %v, want 500", got)
	}
}
