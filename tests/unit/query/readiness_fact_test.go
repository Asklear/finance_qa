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

func TestReadinessSourceAdapterReturnsReadinessFacts(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildReadinessFactDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	adapter := query.NewReadinessSourceAdapter(engine)
	spec := query.BuildQuerySpec("飞未3月数据出来了吗？", readinessAnchor())

	factSet, err := adapter.Fetch(context.Background(), spec)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if factSet.Source != "data_readiness" {
		t.Fatalf("source = %s, want data_readiness", factSet.Source)
	}
	assertFactValue(t, factSet, "readiness_has_data", 1)
	assertFactValue(t, factSet, "readiness_row_count", 2)
	assertFactValue(t, factSet, "readiness_journal_rows", 1)
	assertFactValue(t, factSet, "readiness_bank_rows", 1)
}

func TestReadinessQueryExposesSourceBackedFactSets(t *testing.T) {
	runParallelHeavyQueryTest(t)

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

	factSets, ok := res.Data["fact_sets"].([]query.FactSet)
	if !ok || len(factSets) == 0 {
		t.Fatalf("fact_sets missing or empty: %#v", res.Data["fact_sets"])
	}
	if factSets[0].Source != "data_readiness" {
		t.Fatalf("fact set source = %s, want data_readiness", factSets[0].Source)
	}
	assertFactValue(t, factSets[0], "readiness_row_count", 2)
	if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
		t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
	}
}

func TestReadinessCountsJournalCounterpartyMatches(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "readiness-counterparty.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, counterparty_name TEXT, summary TEXT, debit_amount REAL, credit_amount REAL)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-18', 'V-RDY-CP-1', '220201', '应付账款', '收到发票', '贷', 2000, 0, 2000, '测试供应商有限公司')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert readiness counterparty seed data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("测试供应商有限公司3月数据出来了吗？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	factSets, ok := res.Data["fact_sets"].([]query.FactSet)
	if !ok || len(factSets) == 0 {
		t.Fatalf("fact_sets missing or empty: %#v", res.Data["fact_sets"])
	}
	assertFactValue(t, factSets[0], "readiness_has_data", 1)
	assertFactValue(t, factSets[0], "readiness_row_count", 1)
	assertFactValue(t, factSets[0], "readiness_journal_rows", 1)
	assertFactValue(t, factSets[0], "readiness_bank_rows", 0)
}

func TestReadinessCountsContractLedgerMatches(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "readiness-contract-ledger.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, counterparty_name TEXT, summary TEXT, debit_amount REAL, credit_amount REAL)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, received_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`CREATE TABLE fin_fund_income_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, customer_name TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, received_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`CREATE TABLE fin_fund_income_group_members (id INTEGER PRIMARY KEY AUTOINCREMENT, group_id INTEGER, contract_id TEXT, source_row_number INTEGER)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, quantity TEXT, settlement_amount REAL, is_invoiced TEXT, account_code TEXT)`,
		`CREATE TABLE fin_cost_settlement_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, customer_name TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, quantity TEXT, settlement_amount REAL, is_invoiced TEXT, invoice_amount REAL, paid_amount REAL, account_code TEXT)`,
		`CREATE TABLE fin_cost_settlement_group_members (id INTEGER PRIMARY KEY AUTOINCREMENT, group_id INTEGER, contract_id TEXT, source_row_number INTEGER)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C-FW-RDY-1', '飞未云科（深圳）技术有限公司', '飞未项目-京东价格数据')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-RDY-1', '2026-03', 'contract_fund_income', '26年Q1收入明细', 900, 900, '是', 900)`,
		`INSERT INTO fin_fund_income_groups(id, customer_name, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES (1, '飞未云科（深圳）技术有限公司', '2026-03', 'contract_fund_income', '26年Q1收入明细', 100, 100, '是', 100)`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number) VALUES (1, 'C-FW-RDY-1', 8)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, is_invoiced, account_code)
		 VALUES ('C-FW-RDY-1', '2026-03', 'contract_revenue_cost', '26年Q1成本明细', '1项', 300, '是', '640101')`,
		`INSERT INTO fin_cost_settlement_groups(id, customer_name, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code)
		 VALUES (1, '飞未云科（深圳）技术有限公司', '2026-03', 'contract_revenue_cost', '26年Q1成本明细', '1项', 80, '是', 80, 70, '640101')`,
		`INSERT INTO fin_cost_settlement_group_members(group_id, contract_id, source_row_number) VALUES (1, 'C-FW-RDY-1', 9)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert readiness contract seed data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未云科（深圳）技术有限公司3月数据出来了吗？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	factSets, ok := res.Data["fact_sets"].([]query.FactSet)
	if !ok || len(factSets) == 0 {
		t.Fatalf("fact_sets missing or empty: %#v", res.Data["fact_sets"])
	}
	assertFactValue(t, factSets[0], "readiness_has_data", 1)
	assertFactValue(t, factSets[0], "readiness_row_count", 4)

	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	if !containsString(sourceTables, "fin_contracts") || !containsString(sourceTables, "fin_fund_income") || !containsString(sourceTables, "fin_cost_settlements") {
		t.Fatalf("source_tables = %v, want contract ledger tables", sourceTables)
	}
}

func buildReadinessFactDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "readiness-facts.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, counterparty_name TEXT, summary TEXT, debit_amount REAL, credit_amount REAL)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-05', 'V-RDY-1', '600101', '技术服务费', '为飞未云科提供服务', '贷', 1000, 0, 1000, '飞未云科')`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-08', '飞未云科(深圳)技术有限公司', '结算款', 0, 800)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert readiness seed data failed: %v", err)
		}
	}

	return dbPath
}

func readinessAnchor() time.Time {
	return time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
