package analysis_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/analysis"
)

func TestAgingEngineAnalyzeSummary(t *testing.T) {
	dbPath := setupAgingTestDB(t)
	eng := analysis.NewAgingEngine(dbPath)

	summary, err := eng.AnalyzeSummary("模拟财务科技有限公司", "2026-02")
	if err != nil {
		t.Fatalf("AnalyzeSummary failed: %v", err)
	}

	if summary.ReceivableTotal != 1200 {
		t.Fatalf("receivable total = %.2f, want 1200", summary.ReceivableTotal)
	}
	if summary.PayableTotal != 400 {
		t.Fatalf("payable total = %.2f, want 400", summary.PayableTotal)
	}
	if summary.HealthScore <= 0 || summary.HealthScore > 100 {
		t.Fatalf("health score out of range: %d", summary.HealthScore)
	}
	if len(summary.ReceivableBuckets) == 0 {
		t.Fatal("receivable buckets should not be empty")
	}
}

func setupAgingTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "aging_test.db")

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
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 failed: %v\n%s", err, string(out))
	}
	return dbPath
}
