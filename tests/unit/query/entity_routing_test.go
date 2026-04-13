package query_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

const testCompany = "南京优集数据科技有限公司"

func TestQueryMonthlyKPIShouldNotBeMisroutedAsMetricEntity(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年1月收入/成本多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if strings.Contains(res.Message, "[收入]") || strings.Contains(res.Message, "[成本]") {
		t.Fatalf("monthly KPI question was misrouted as entity query: %s", res.Message)
	}
	if entity, ok := res.Data["entity"].(string); ok && (entity == "收入" || entity == "成本") {
		t.Fatalf("unexpected metric entity in response: %q", entity)
	}
}

func TestQueryRealEntityQuestionStillUsesCounterpartyPath(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未2月收入多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "[飞未]") {
		t.Fatalf("expected counterparty style response, got: %s", res.Message)
	}
}

func buildEntityRoutingTestDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "entity-routing.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE journal (
			company TEXT,
			voucher_date TEXT,
			account_code TEXT,
			account_name TEXT,
			direction TEXT,
			amount REAL,
			summary TEXT,
			counterparty TEXT,
			debit_amount REAL,
			credit_amount REAL
		)`,
		`CREATE TABLE bank_statement (
			company TEXT,
			transaction_date TEXT,
			counterparty_name TEXT,
			summary TEXT,
			debit_amount REAL,
			credit_amount REAL
		)`,
		`CREATE TABLE balance_sheet (
			company TEXT,
			period TEXT,
			account_name TEXT,
			account_code TEXT,
			opening_balance REAL,
			closing_balance REAL
		)`,
		`CREATE TABLE income_statement (
			company TEXT,
			period TEXT,
			item_name TEXT,
			current_amount REAL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	inserts := []string{
		`INSERT INTO balance_sheet(company, period, account_name, account_code, opening_balance, closing_balance)
		 VALUES ('南京优集数据科技有限公司', '2026-01', '货币资金', '1002', 1000, 2000)`,
		`INSERT INTO balance_sheet(company, period, account_name, account_code, opening_balance, closing_balance)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '货币资金', '1002', 2000, 2600)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-15', '6001', '主营业务收入', '贷', 1200, '1月飞未收入确认', '飞未云科', 0, 1200)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-20', '6401', '主营业务成本', '借', 400, '1月项目成本', '飞未云科', 400, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-10', '6001', '主营业务收入', '贷', 800, '2月飞未收入确认', '飞未云科', 0, 800)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01', '一、营业收入', 1200)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01', '五、净利润', 800)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '一、营业收入', 800)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '五、净利润', 500)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-15', '飞未云科', '2月回款', 0, 904)`,
	}
	for _, stmt := range inserts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert seed data failed: %v", err)
		}
	}

	return dbPath
}
