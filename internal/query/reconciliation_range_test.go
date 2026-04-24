package query

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestQueryReconciliationAggregatesBookSummaryAcrossRequestedRange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reconciliation-range.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES
		 ('测试公司','2026-01','营业收入',100,100),
		 ('测试公司','2026-01','营业成本',60,60),
		 ('测试公司','2026-01','利润总额',40,40),
		 ('测试公司','2026-01','净利润',40,40),
		 ('测试公司','2026-02','营业收入',200,300),
		 ('测试公司','2026-02','营业成本',120,180),
		 ('测试公司','2026-02','利润总额',80,120),
		 ('测试公司','2026-02','净利润',80,120),
		 ('测试公司','2026-03','营业收入',300,600),
		 ('测试公司','2026-03','营业成本',150,330),
		 ('测试公司','2026-03','利润总额',150,270),
		 ('测试公司','2026-03','净利润',150,270)`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary) VALUES
		 ('测试公司','2026-01-15',100,0,'客户A','1月回款'),
		 ('测试公司','2026-02-18',200,0,'客户A','2月回款'),
		 ('测试公司','2026-03-22',300,0,'客户A','3月回款'),
		 ('测试公司','2026-03-25',0,180,'供应商A','3月付款')`,
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

	res := engine.queryReconciliation("为什么2026年第一季度利润和现金差这么多？", "2026-01", "2026-03")
	if !res.Success {
		t.Fatalf("queryReconciliation failed: %+v", res)
	}

	bookView, ok := res.Data["book_view"].(monthlyBookView)
	if !ok {
		t.Fatalf("book_view type mismatch: %T", res.Data["book_view"])
	}
	if bookView.Revenue != 600 {
		t.Fatalf("book_view.Revenue = %.2f, want 600.00", bookView.Revenue)
	}
	if bookView.TotalCost != 330 {
		t.Fatalf("book_view.TotalCost = %.2f, want 330.00", bookView.TotalCost)
	}
	if bookView.Profit != 270 {
		t.Fatalf("book_view.Profit = %.2f, want 270.00", bookView.Profit)
	}
	if got, _ := res.Data["period"].(string); got != "2026-01~2026-03" {
		t.Fatalf("period = %q, want 2026-01~2026-03", got)
	}
	if got := res.Message; got == "" || strings.HasPrefix(got, "2026-03 我拆成两层给你看") {
		t.Fatalf("message should use range period label, got: %s", got)
	}
}
