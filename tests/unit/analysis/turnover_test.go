package analysis_test

import (
	sqlpkg "database/sql"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/analysis"

	_ "modernc.org/sqlite"
)

func TestTurnoverEngineAnalyze(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "turnover.db")
	schema := `
CREATE TABLE balance_sheet (
  company TEXT,
  account_code TEXT,
  period TEXT,
  account_name TEXT,
  opening_balance REAL,
  closing_balance REAL
);
CREATE TABLE journal (
  company TEXT,
  period TEXT,
  voucher_date TEXT,
  voucher_no TEXT,
  account_code TEXT,
  account_name TEXT,
  summary TEXT,
  debit_amount REAL,
  credit_amount REAL,
  counterparty TEXT
);
CREATE TABLE income_statement (
  company TEXT,
  period TEXT,
  item_name TEXT,
  current_amount REAL
);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','1405','2026-01','存货',0,400000);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','1405','2026-02','存货',100000,450000);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业收入',3200000);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业成本',2100000);
`
	cmd := exec.Command("sqlite3", dbPath)
	cmd.Stdin = strings.NewReader(schema)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 failed: %v\n%s", err, string(out))
	}

	db, err := sqlpkg.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	stmts := []string{
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-01','2026-01-10','AR-OPEN','112201','应收账款-客户A','历史应收',520000,0,'客户A')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-20','AR-NEW','112201','应收账款-客户B','本月新增应收',30000,0,'客户B')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-01','2026-01-05','AP-OPEN','220201','应付账款-供应商A','历史应付',0,320000,'供应商A')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-18','AP-NEW','220201','应付账款-供应商B','本月新增应付',0,10000,'供应商B')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed journal failed: %v", err)
		}
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
	if summary.InventoryTurnoverDays < 73 || summary.InventoryTurnoverDays > 74.5 {
		t.Fatalf("unexpected inventory turnover days: %.2f", summary.InventoryTurnoverDays)
	}
}
