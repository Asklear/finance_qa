package query

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMonthlySummaryYTDFallbackUsesRequestedYear(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-ytd.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2027-02','1002','货币资金',100,100)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2027年2月经营状况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "2027年1月以来（YTD）累计") {
		t.Fatalf("message should use requested year, got: %s", res.Message)
	}
	if strings.Contains(res.Message, "2026年1月以来（YTD）累计") {
		t.Fatalf("message should not use hardcoded year, got: %s", res.Message)
	}
}

func TestFallbackHintUsesGenericPlaceholders(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-hint.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-03','1002','货币资金',100,150)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("帮我随便看一下")
	if res.Success {
		t.Fatalf("expected fallback result, got success with message: %s", res.Message)
	}
	hint, _ := res.Data["hint"].(string)
	if hint == "" {
		t.Fatalf("expected non-empty hint")
	}
	if strings.Contains(hint, "2026") || strings.Contains(hint, "飞未云科") {
		t.Fatalf("hint should be generic instead of hardcoded example, got: %s", hint)
	}
}

func TestMonthEndDayAcceptsFullDateInput(t *testing.T) {
	if got := monthEndDay("2027-02-15"); got != "2027-02-15" {
		t.Fatalf("monthEndDay(full-date) = %q, want %q", got, "2027-02-15")
	}
}

func TestStripTemporalNoiseRemovesAnyYearMonthDayTokens(t *testing.T) {
	got := stripTemporalNoise("2027年金程3月26日")
	if got != "金程" {
		t.Fatalf("stripTemporalNoise() = %q, want %q", got, "金程")
	}
}
