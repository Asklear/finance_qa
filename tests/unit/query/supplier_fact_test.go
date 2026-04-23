package query_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestSupplierPaymentSourceAdapterReturnsFactsAndRoster(t *testing.T) {
	dbPath := buildSupplierFactDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	adapter := query.NewSupplierPaymentSourceAdapter(engine)
	spec := query.BuildQuerySpec("2026年3月有多少家供应商发生付款？分别叫什么、各付了多少？", supplierAnchor())

	factSet, err := adapter.Fetch(context.Background(), spec)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if factSet.Source != "supplier_payments" {
		t.Fatalf("source = %s, want supplier_payments", factSet.Source)
	}
	assertFactValue(t, factSet, "supplier_payment_count", 2)
	assertFactValue(t, factSet, "supplier_payment_total", 1500)

	var rosterFact *query.Fact
	for i := range factSet.Facts {
		if factSet.Facts[i].MetricKey == "supplier_payment_roster" {
			rosterFact = &factSet.Facts[i]
			break
		}
	}
	if rosterFact == nil {
		t.Fatalf("supplier_payment_roster fact missing: %+v", factSet.Facts)
	}
	roster, ok := rosterFact.TracePayload["suppliers"].([]map[string]any)
	if !ok || len(roster) != 2 {
		t.Fatalf("supplier roster = %#v, want 2 rows", rosterFact.TracePayload["suppliers"])
	}
}

func TestSupplierPaymentQueryExposesSourceBackedFactSets(t *testing.T) {
	dbPath := buildSupplierFactDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月有多少家供应商发生付款？分别叫什么、各付了多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	factSets, ok := res.Data["fact_sets"].([]query.FactSet)
	if !ok || len(factSets) == 0 {
		t.Fatalf("fact_sets missing or empty: %#v", res.Data["fact_sets"])
	}
	if factSets[0].Source != "supplier_payments" {
		t.Fatalf("fact set source = %s, want supplier_payments", factSets[0].Source)
	}
	assertFactValue(t, factSets[0], "supplier_payment_count", 2)
	assertFactValue(t, factSets[0], "supplier_payment_total", 1500)
}

func buildSupplierFactDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "supplier-facts.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, counterparty_name TEXT, summary TEXT, debit_amount REAL, credit_amount REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, direction TEXT, amount REAL, summary TEXT, counterparty TEXT, debit_amount REAL, credit_amount REAL)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-05', '供应商A有限公司', '技术服务费', 1000, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-06', '北京市中闻（南京）律师事务所', '法律服务费', 500, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-07', '南京优集数据科技有限公司深圳分公司', '内部转账', 700, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-08', '梁梦瑶', '报销', 200, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-09', '暂收款', '实时缴税', 300, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-05', 'V-SUP-1', '640101', '主营业务成本', '借', 1000, '供应商A成本确认', '供应商A有限公司', 1000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-05', 'V-SUP-1', '220201', '应付账款', '贷', 1000, '供应商A成本确认', '供应商A有限公司', 0, 1000)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert supplier fact seed data failed: %v", err)
		}
	}

	return dbPath
}

func supplierAnchor() time.Time {
	return time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
}
