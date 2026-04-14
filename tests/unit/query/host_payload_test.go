package query_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestHostPayloadBalanceDetailShouldRespectPeriodRange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "host-payload.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, transaction_time TEXT, transaction_type TEXT, debit_amount REAL, credit_amount REAL, balance REAL, summary TEXT, counterparty_name TEXT, counterparty_account TEXT)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	seed := []string{
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('南京优集数据科技有限公司','2026-03','1122','应收账款',1,2)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司','2026-03','一、营业收入',100,100)`,
		`INSERT INTO balance_detail(company, year, period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit) VALUES ('南京优集数据科技有限公司',2026,'2026-02','1122','应收账款',10,0,20,5,25,0)`,
		`INSERT INTO balance_detail(company, year, period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit) VALUES ('南京优集数据科技有限公司',2026,'2026-03','1122','应收账款',25,0,30,10,45,0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES ('南京优集数据科技有限公司','2026-03','2026-03-20','记-1','1122','应收账款','测试','借',100,100,0,'测试客户')`,
		`INSERT INTO bank_statement(company, transaction_date, transaction_time, transaction_type, debit_amount, credit_amount, balance, summary, counterparty_name, counterparty_account) VALUES ('南京优集数据科技有限公司','2026-03-20','10:00:00','转账',0,100,1000,'测试回款','测试客户','xx')`,
	}
	for _, stmt := range seed {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.HostLLMPayload("2026-03", "2026-03", "测试问题")
	if !res.Success {
		t.Fatalf("HostLLMPayload failed: %+v", res)
	}

	payload, ok := res.Data["llm_payload"].(map[string]any)
	if !ok {
		t.Fatalf("missing llm_payload: %+v", res.Data)
	}
	financialTables, ok := payload["financial_tables"].(map[string]any)
	if !ok {
		t.Fatalf("missing financial_tables: %+v", payload)
	}
	bdRaw, ok := financialTables["balance_detail"].([]map[string]any)
	if !ok {
		t.Fatalf("balance_detail type mismatch: %T", financialTables["balance_detail"])
	}
	if len(bdRaw) != 1 {
		t.Fatalf("balance_detail should contain exactly one period row, got=%d rows=%v", len(bdRaw), bdRaw)
	}
	if period, _ := bdRaw[0]["period"].(string); period != "2026-03" {
		t.Fatalf("unexpected balance_detail period=%v", bdRaw[0]["period"])
	}
}
