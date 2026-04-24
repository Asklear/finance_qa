package query

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestGetLatestPeriodAnchorUsesLatestNonFutureAvailableMonth(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "future-anchor.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE journal (company TEXT, voucher_date TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT)`,
		`CREATE TABLE balance_detail (company TEXT, period TEXT)`,
		`CREATE TABLE fin_fund_income (year_month TEXT)`,
		`CREATE TABLE fin_cost_settlements (year_month TEXT)`,
		`INSERT INTO journal(company, voucher_date) VALUES ('测试公司', '2026-03-31')`,
		`INSERT INTO fin_cost_settlements(year_month) VALUES ('2099-12')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine := &Engine{
		db:                db,
		Company:           "测试公司",
		latestAnchorCache: map[string]time.Time{},
	}

	got := engine.getLatestPeriodAnchor()
	want := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("latest anchor = %s, want latest non-future month %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestGetLatestPeriodAnchorFallsBackToCurrentMonthWhenOnlyFutureMonthsExist(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "future-only-anchor.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE journal (company TEXT, voucher_date TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT)`,
		`CREATE TABLE balance_detail (company TEXT, period TEXT)`,
		`CREATE TABLE fin_fund_income (year_month TEXT)`,
		`CREATE TABLE fin_cost_settlements (year_month TEXT)`,
		`INSERT INTO fin_fund_income(year_month) VALUES ('2099-10')`,
		`INSERT INTO fin_cost_settlements(year_month) VALUES ('2099-12')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine := &Engine{
		db:                db,
		Company:           "测试公司",
		latestAnchorCache: map[string]time.Time{},
	}

	got := engine.getLatestPeriodAnchor()
	now := time.Now().UTC()
	want := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("latest anchor = %s, want current month fallback %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}
