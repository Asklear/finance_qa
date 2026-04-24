package query_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestTaxQueryAnnotatesSourceSummary(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tax-source-attribution.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE journal (
			company TEXT,
			period TEXT,
			voucher_date TEXT,
			voucher_no TEXT,
			account_code TEXT,
			account_name TEXT,
			summary TEXT,
			direction TEXT,
			amount REAL,
			debit_amount REAL,
			credit_amount REAL,
			counterparty TEXT
		)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-25', 'V-TAX-1', '22210106', '销项税额', '销项税', '贷', 133631.18, 0, 133631.18, '')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月销项税额是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceSummary, _ := res.Data["source_summary"].(string)
	if !containsAll(sourceSummary, "来源：", "《序时帐》") {
		t.Fatalf("source_summary = %q, want include 来源 and 《序时帐》", sourceSummary)
	}
}

func TestLargeBankTransactionAnnotatesSourceSummary(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月最大的单笔流入对手方是谁，金额多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceSummary, _ := res.Data["source_summary"].(string)
	if !containsAll(sourceSummary, "来源：", "《银行流水》") {
		t.Fatalf("source_summary = %q, want include 来源 and 《银行流水》", sourceSummary)
	}
}

func TestPreciseBalanceQueryAnnotatesSourceSummary(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月货币资金余额是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceSummary, _ := res.Data["source_summary"].(string)
	if !containsAll(sourceSummary, "来源：", "《资产负债表》", "《序时帐》") {
		t.Fatalf("source_summary = %q, want include 《资产负债表》 and 《序时帐》", sourceSummary)
	}
}

func TestReadinessQueryAnnotatesSourceSummary(t *testing.T) {
	dbPath := buildReadinessFactDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未3月数据出来了吗？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceSummary, _ := res.Data["source_summary"].(string)
	if !containsAll(sourceSummary, "来源：", "《序时帐》", "《银行流水》") {
		t.Fatalf("source_summary = %q, want include 《序时帐》 and 《银行流水》", sourceSummary)
	}
}

func TestCounterpartyClassificationAnnotatesSourceSummary(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未云科这个主体目前更像客户、供应商还是混合往来？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceSummary, _ := res.Data["source_summary"].(string)
	if !containsAll(sourceSummary, "来源：", "《序时帐》", "《银行流水》") {
		t.Fatalf("source_summary = %q, want judgment evidence from 《序时帐》 and 《银行流水》", sourceSummary)
	}
	if containsAll(sourceSummary, "《合同资金收入表》") || containsAll(sourceSummary, "《合同成本结算表》") {
		t.Fatalf("source_summary should not over-attribute contract ledgers for classification judgment, got %q", sourceSummary)
	}
}

func TestReconciliationQueryAnnotatesSourceSummary(t *testing.T) {
	dbPath := buildReconciliationSourceAttributionDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("为什么2026年3月银行卡上看和账上看的利润不一样？差异最大的3个原因是什么？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceSummary, _ := res.Data["source_summary"].(string)
	if !containsAll(sourceSummary, "来源：", "《银行流水》") {
		t.Fatalf("source_summary = %q, want include 来源 and 《银行流水》", sourceSummary)
	}
	if !containsAll(sourceSummary, "《利润表》") && !containsAll(sourceSummary, "《序时帐》") {
		t.Fatalf("source_summary = %q, want include 《利润表》 or 《序时帐》", sourceSummary)
	}
}

func buildReconciliationSourceAttributionDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "reconciliation-source-attribution.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (
			company TEXT,
			period TEXT,
			item_name TEXT,
			current_amount REAL,
			cumulative_amount REAL
		)`,
		`CREATE TABLE journal (
			company TEXT,
			period TEXT,
			voucher_date TEXT,
			voucher_no TEXT,
			account_code TEXT,
			account_name TEXT,
			summary TEXT,
			direction TEXT,
			amount REAL,
			debit_amount REAL,
			credit_amount REAL,
			counterparty TEXT
		)`,
		`CREATE TABLE bank_statement (
			company TEXT,
			transaction_date TEXT,
			debit_amount REAL,
			credit_amount REAL,
			counterparty_name TEXT,
			summary TEXT
		)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES
		 ('南京优集数据科技有限公司', '2026-03', '一、营业收入', 1000, 3000),
		 ('南京优集数据科技有限公司', '2026-03', '五、净利润', 200, 600)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('南京优集数据科技有限公司', '2026-03', '2026-03-10', '记-0001', '600101', '技术服务费', '确认客户A收入', '贷', 1000, 0, 1000, '客户A'),
		 ('南京优集数据科技有限公司', '2026-03', '2026-03-12', '记-0002', '640101', '营业成本', '确认供应商A成本', '借', 700, 700, 0, '供应商A')`,
		`INSERT INTO bank_statement(company, transaction_date, debit_amount, credit_amount, counterparty_name, summary) VALUES
		 ('南京优集数据科技有限公司', '2026-03-08', 0, 650, '客户A', '3月回款'),
		 ('南京优集数据科技有限公司', '2026-03-20', 900, 0, '供应商A', '3月付款')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	return dbPath
}
