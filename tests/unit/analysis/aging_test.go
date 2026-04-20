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
	if got := bucketAmount(summary.ReceivableBuckets, "0-30天"); got != 900 {
		t.Fatalf("receivable 0-30 = %.2f, want 900", got)
	}
	if got := bucketAmount(summary.ReceivableBuckets, "61天以上"); got != 300 {
		t.Fatalf("receivable 61+ = %.2f, want 300", got)
	}
	if got := bucketAmount(summary.PayableBuckets, "31-60天"); got != 400 {
		t.Fatalf("payable 31-60 = %.2f, want 400", got)
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

	schema := `
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
`

	cmd := exec.Command("sqlite3", dbPath)
	cmd.Stdin = strings.NewReader(schema)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 failed: %v\n%s", err, string(out))
	}

	db, err := sqlpkg.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2025-12','2025-12-20','AR-OLD','112201','应收账款-客户B','历史应收',800,0,'客户B')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-20','AR-NEW','112201','应收账款-客户A','本月新增应收',900,0,'客户A')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-25','AR-CLEAR','112201','应收账款-客户B','回款冲应收',0,500,'客户B')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-01','2026-01-15','AP-OLD','220201','应付账款-供应商A','历史应付',0,1000,'供应商A')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-10','AP-PAY','220201','应付账款-供应商A','付款冲应付',600,0,'供应商A')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed journal failed: %v", err)
		}
	}
	return dbPath
}

func bucketAmount(buckets []analysis.AgingBucket, label string) float64 {
	for _, bucket := range buckets {
		if bucket.Label == label {
			return bucket.Amount
		}
	}
	return 0
}
