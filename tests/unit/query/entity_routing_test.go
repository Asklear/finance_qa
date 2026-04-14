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

func TestEntityYearCumulativeUsesYearRangeInsteadOfSingleMonth(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未云科2026年累计销售额多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	amount, ok := res.Data["amount"].(float64)
	if !ok {
		t.Fatalf("missing amount in response: %+v", res.Data)
	}
	// test db has 2026-01 revenue=1200 and 2026-02 revenue=800 for 飞未云科
	if amount != 2000 {
		t.Fatalf("want cumulative amount=2000, got=%v message=%s", amount, res.Message)
	}
	if !strings.Contains(res.Message, "2026-01~2026-02") {
		t.Fatalf("expected period range in message, got: %s", res.Message)
	}
}

func TestEntityCoreMetricStillUsesMandatoryDualPerspective(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未云科2026年2月收入、成本、利润分别是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	required := []string{"先说结论", "银行卡上看", "账上看", "差异最大的3个原因", "建议动作"}
	for _, section := range required {
		if !strings.Contains(res.Message, section) {
			t.Fatalf("mandatory boss dual-perspective section missing %q, message=%s", section, res.Message)
		}
	}
}

func TestAmbiguousPreShouKuanRoutesToARAPWithoutFallback(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("汇智在2026年2月这笔是供应商付款还是预收款？")
	if !res.Success {
		t.Fatalf("want success, got=%s", res.Message)
	}
	if res.AnswerMethod != "sql" {
		t.Fatalf("want sql, got=%s", res.AnswerMethod)
	}
	if strings.Contains(res.Message, "语义模糊") {
		t.Fatalf("fallback leaked: %s", res.Message)
	}
	if !strings.Contains(res.Message, "供应商") {
		t.Fatalf("expect supplier wording, got: %s", res.Message)
	}
	tr, ok := res.Data["intent_trace"].(map[string]any)
	if !ok {
		t.Fatalf("intent_trace missing")
	}
	for _, k := range []string{"router_version", "matched", "scores", "final_intent", "confidence"} {
		if _, ok := tr[k]; !ok {
			t.Fatalf("intent_trace missing key=%s", k)
		}
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
		`INSERT INTO balance_sheet(company, period, account_name, account_code, opening_balance, closing_balance)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '应付职工薪酬', '2211', 0, 300)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-15', '飞未云科', '2月回款', 0, 904)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-13', '南京汇智互娱教育科技有限公司', '供应商付款', 53750, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-25', '南京汇智互娱教育科技有限公司', '供应商付款', 53750, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-25', '640101', '主营业务成本', '借', 95000, '汇智成本确认', '南京汇智互娱教育科技有限公司', 95000, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-25', '22210101', '进项税额', '借', 12500, '汇智进项税', '南京汇智互娱教育科技有限公司', 12500, 0)`,
	}
	for _, stmt := range inserts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert seed data failed: %v", err)
		}
	}

	return dbPath
}
