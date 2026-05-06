package analysis

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"financeqa/internal/openitems"

	_ "modernc.org/sqlite"
)

func TestAgingAndAlertEnginesAnalyzeJournalOpenItems(t *testing.T) {
	db := openAnalysisTestDB(t)
	execAnalysisSQL(t, db, `
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
)`,
		`INSERT INTO journal VALUES ('优集科技','2025-12','2025-12-20','AR-OLD','112201','应收账款-客户B','历史应收',800,0,'客户B')`,
		`INSERT INTO journal VALUES ('优集科技','2026-02','2026-02-20','AR-NEW','112201','应收账款-客户A','本月新增应收',900,0,'客户A')`,
		`INSERT INTO journal VALUES ('优集科技','2026-02','2026-02-25','AR-CLEAR','112201','应收账款-客户B','回款冲应收',0,500,'客户B')`,
		`INSERT INTO journal VALUES ('优集科技','2026-01','2026-01-15','AP-OLD','220201','应付账款-供应商A','历史应付',0,1000,'供应商A')`,
		`INSERT INTO journal VALUES ('优集科技','2026-02','2026-02-10','AP-PAY','220201','应付账款-供应商A','付款冲应付',600,0,'供应商A')`,
	)

	aging := &AgingEngine{db: db}
	summary, err := aging.AnalyzeSummary("优集科技", "2026-02")
	if err != nil {
		t.Fatalf("AnalyzeSummary: %v", err)
	}
	if summary.ReceivableTotal != 1200 || summary.PayableTotal != 400 {
		t.Fatalf("aging totals = %+v, want receivable=1200 payable=400", summary)
	}
	if got := bucketAmount(summary.ReceivableBuckets, "0-30天"); got != 900 {
		t.Fatalf("receivable 0-30 bucket = %.2f, want 900", got)
	}
	if got := bucketAmount(summary.ReceivableBuckets, "61天以上"); got != 300 {
		t.Fatalf("receivable 61+ bucket = %.2f, want 300", got)
	}
	if summary.HealthScore <= 0 || summary.HealthScore > 100 {
		t.Fatalf("health score out of range: %d", summary.HealthScore)
	}

	alerts, err := (&AlertEngine{aging: aging}).Generate("优集科技", "2026-02")
	if err != nil {
		t.Fatalf("Generate alerts: %v", err)
	}
	if len(alerts) == 0 || alerts[0].Type != "overdue_receivable" {
		t.Fatalf("expected overdue receivable alert, got %+v", alerts)
	}
}

func TestTurnoverEngineAnalyzeUsesFinancialTablesAndOpenItems(t *testing.T) {
	db := openAnalysisTestDB(t)
	execAnalysisSQL(t, db, `
CREATE TABLE balance_sheet (
  company TEXT,
  account_code TEXT,
  period TEXT,
  account_name TEXT,
  opening_balance REAL,
  closing_balance REAL
)`,
		`
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
)`,
		`
CREATE TABLE income_statement (
  company TEXT,
  period TEXT,
  item_name TEXT,
  current_amount REAL
)`,
		`INSERT INTO balance_sheet VALUES ('优集科技','1405','2026-01','存货',0,400000)`,
		`INSERT INTO balance_sheet VALUES ('优集科技','1405','2026-02','存货',100000,450000)`,
		`INSERT INTO income_statement VALUES ('优集科技','2026-02','营业收入',3200000)`,
		`INSERT INTO income_statement VALUES ('优集科技','2026-02','营业成本',2100000)`,
		`INSERT INTO journal VALUES ('优集科技','2026-01','2026-01-10','AR-OPEN','112201','应收账款-客户A','历史应收',520000,0,'客户A')`,
		`INSERT INTO journal VALUES ('优集科技','2026-02','2026-02-20','AR-NEW','112201','应收账款-客户B','本月新增应收',30000,0,'客户B')`,
		`INSERT INTO journal VALUES ('优集科技','2026-01','2026-01-05','AP-OPEN','220201','应付账款-供应商A','历史应付',0,320000,'供应商A')`,
		`INSERT INTO journal VALUES ('优集科技','2026-02','2026-02-18','AP-NEW','220201','应付账款-供应商B','本月新增应付',0,10000,'供应商B')`,
	)

	summary, err := (&TurnoverEngine{db: db}).Analyze("优集科技", "2026-02")
	if err != nil {
		t.Fatalf("Analyze turnover: %v", err)
	}
	if summary.ReceivableTurnoverDays < 60 || summary.ReceivableTurnoverDays > 62 {
		t.Fatalf("receivable turnover days = %.2f", summary.ReceivableTurnoverDays)
	}
	if summary.PayableTurnoverDays < 56 || summary.PayableTurnoverDays > 57 {
		t.Fatalf("payable turnover days = %.2f", summary.PayableTurnoverDays)
	}
	if summary.InventoryTurnoverDays < 73 || summary.InventoryTurnoverDays > 74.5 {
		t.Fatalf("inventory turnover days = %.2f", summary.InventoryTurnoverDays)
	}
}

func TestProfitCashBridgeWithMinimalDatabase(t *testing.T) {
	db := openAnalysisTestDB(t)
	execAnalysisSQL(t, db, `
CREATE TABLE income_statement (
  company TEXT,
  period TEXT,
  item_name TEXT,
  current_amount REAL
)`,
		`
CREATE TABLE balance_detail (
  company TEXT,
  period TEXT,
  account_code TEXT,
  account_name TEXT,
  opening_debit REAL,
  opening_credit REAL,
  closing_debit REAL,
  closing_credit REAL
)`,
		`
CREATE TABLE bank_statement (
  company TEXT,
  transaction_date TEXT,
  debit_amount REAL,
  credit_amount REAL
)`,
		`
CREATE TABLE journal (
  company TEXT,
  period TEXT,
  voucher_date TEXT,
  voucher_no TEXT,
  account_code TEXT,
  account_name TEXT,
  summary TEXT,
  debit_amount REAL,
  credit_amount REAL
)`,
		`INSERT INTO income_statement VALUES ('优集科技','2026-03','净利润',100)`,
		`INSERT INTO balance_detail VALUES ('优集科技','2026-02','1122','应收账款',0,0,80,0)`,
		`INSERT INTO balance_detail VALUES ('优集科技','2026-03','1122','应收账款',0,0,130,0)`,
		`INSERT INTO bank_statement VALUES ('优集科技','2026-03-10',50,200)`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-10','V1','100201','银行存款','客户回款',200,0)`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-10','V1','600101','主营业务收入','客户回款',0,200)`,
	)

	bridge, err := AnalyzeProfitCashBridgeWithDB(context.Background(), db, "优集科技", "2026-03")
	if err != nil {
		t.Fatalf("AnalyzeProfitCashBridgeWithDB: %v", err)
	}
	if bridge.NetProfit != 100 || bridge.ARIncrease != 50 || bridge.EstimatedOperatingCash != 50 {
		t.Fatalf("bridge profit/AR estimate = %+v", bridge)
	}
	if bridge.BankNetCash != 150 || bridge.OperatingCashNet != 200 || bridge.OperatingCashGap != 150 {
		t.Fatalf("bridge cash = %+v", bridge)
	}
}

func TestAnalysisHelpers(t *testing.T) {
	end, err := periodEndDate("2024-02")
	if err != nil {
		t.Fatalf("periodEndDate: %v", err)
	}
	if got := end.Format("2006-01-02"); got != "2024-02-29" {
		t.Fatalf("periodEndDate = %q", got)
	}
	if prev, err := previousPeriod("2026-01"); err != nil || prev != "2025-12" {
		t.Fatalf("previousPeriod = %q, %v", prev, err)
	}
	if got := lastDayOfMonth("2024-02"); got != "2024-02-29" {
		t.Fatalf("lastDayOfMonth = %q", got)
	}
	if got := periodEndDateString("2024-03-01"); got != "2024-02-29" {
		t.Fatalf("periodEndDateString = %q", got)
	}
	if got := round2(12.345); got != 12.35 {
		t.Fatalf("round2 = %.2f", got)
	}
	if got := safeTurnoverDays(100, 400); got != 91.25 {
		t.Fatalf("safeTurnoverDays = %.2f", got)
	}
	if got := safeTurnoverDays(100, 0); got != 0 {
		t.Fatalf("safeTurnoverDays zero amount = %.2f", got)
	}

	buckets := bucketsFromOpenItems([]openitems.OpenItem{
		{AgeDays: 10, Amount: 1.234},
		{AgeDays: 45, Amount: 2},
		{AgeDays: 90, Amount: 3},
	})
	if bucketAmount(buckets, "0-30天") != 1.23 || bucketAmount(buckets, "31-60天") != 2 || bucketAmount(buckets, "61天以上") != 3 {
		t.Fatalf("buckets = %+v", buckets)
	}

	if !isOperatingCashRoot("1122") || !isNonOperatingCashRoot("1601") {
		t.Fatal("cash root classification failed")
	}
	if !shouldTreatMixedPayrollCashAsOperating(&voucherCashState{
		operating:    true,
		nonOperating: true,
		bankOut:      100,
		roots: map[string]float64{
			"2211": 100,
			"1221": 10,
			"2221": 5,
		},
	}) {
		t.Fatal("expected payroll withholding voucher to remain operating cash")
	}
	if shouldTreatMixedPayrollCashAsOperating(&voucherCashState{
		operating:    true,
		nonOperating: true,
		bankOut:      100,
		roots:        map[string]float64{"1601": 100},
	}) {
		t.Fatal("fixed asset mixed voucher should not be treated as operating cash")
	}
}

func openAnalysisTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "analysis.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func execAnalysisSQL(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec analysis SQL failed: %v\n%s", err, stmt)
		}
	}
}

func bucketAmount(buckets []AgingBucket, label string) float64 {
	for _, bucket := range buckets {
		if bucket.Label == label {
			return bucket.Amount
		}
	}
	return 0
}
