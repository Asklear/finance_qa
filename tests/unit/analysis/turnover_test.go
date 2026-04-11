package analysis_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/analysis"
)

func TestTurnoverEngineAnalyze(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "turnover.db")
	sql := `
CREATE TABLE balance_sheet (
  company TEXT,
  period TEXT,
  account_name TEXT,
  opening_balance REAL,
  closing_balance REAL
);
CREATE TABLE income_statement (
  company TEXT,
  period TEXT,
  item_name TEXT,
  current_amount REAL
);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','2026-02','应收账款',520000,550000);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','2026-02','应付账款',320000,330000);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','2026-02','存货',420000,450000);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业收入',3200000);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业成本',2100000);
`
	cmd := exec.Command("sqlite3", dbPath)
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 failed: %v\n%s", err, string(out))
	}

	engine := analysis.NewTurnoverEngine(dbPath)
	defer func() { _ = engine.Close() }()

	summary, err := engine.Analyze("模拟财务科技有限公司", "2026-02")
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if summary.ReceivableTurnoverDays < 60 || summary.ReceivableTurnoverDays > 62 {
		t.Fatalf("unexpected receivable turnover days: %.2f", summary.ReceivableTurnoverDays)
	}
	if summary.PayableTurnoverDays < 56 || summary.PayableTurnoverDays > 57 {
		t.Fatalf("unexpected payable turnover days: %.2f", summary.PayableTurnoverDays)
	}
	if summary.InventoryTurnoverDays < 75 || summary.InventoryTurnoverDays > 76 {
		t.Fatalf("unexpected inventory turnover days: %.2f", summary.InventoryTurnoverDays)
	}
}
