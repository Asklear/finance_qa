package analysis_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/analysis"
)

func TestAlertEngineGenerate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "alert.db")
	sql := `
CREATE TABLE journal (
  company TEXT,
  period TEXT,
  voucher_date TEXT,
  account_code TEXT,
  debit_amount REAL,
  credit_amount REAL,
  counterparty TEXT
);
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-20','112201',900,0,'客户A');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-01-01','112201',300,0,'客户B');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-10','220201',0,400,'供应商A');
`
	cmd := exec.Command("sqlite3", dbPath)
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 failed: %v\n%s", err, string(out))
	}

	engine := analysis.NewAlertEngine(dbPath)
	defer func() { _ = engine.Close() }()

	alerts, err := engine.Generate("模拟财务科技有限公司", "2026-02")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(alerts) == 0 {
		t.Fatal("expected at least one alert")
	}
	if alerts[0].Type != "overdue_receivable" {
		t.Fatalf("first alert type = %q", alerts[0].Type)
	}
}
