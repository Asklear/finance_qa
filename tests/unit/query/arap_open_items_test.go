package query_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestARAPUsesCrossMonthFIFOSettlement(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "arap-open-items.db")
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
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-RT-0228', '112201', '应收账款-任拓', '2月确认收入', '借', 19275.00, 19275.00, 0, '任拓')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-26', 'V-RT-0326', '112201', '应收账款-任拓', '3月确认收入', '借', 18190.20, 18190.20, 0, '任拓')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', 'V-RT-0331', '112201', '应收账款-任拓', '3月回款冲应收', '贷', 19275.00, 0, 19275.00, '任拓')`,
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

	res := engine.Query("2026年3月账上应收账款情况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	if got := res.Data["total"]; got != float64(18190.20) {
		t.Fatalf("closing total = %v, want 18190.20", got)
	}
	if got := res.Data["opening_balance"]; got != float64(19275) {
		t.Fatalf("opening balance = %v, want 19275", got)
	}
	if got := res.Data["current_increase"]; got != float64(18190.20) {
		t.Fatalf("current increase = %v, want 18190.20", got)
	}
	if got := res.Data["current_decrease"]; got != float64(19275) {
		t.Fatalf("current decrease = %v, want 19275", got)
	}
	if got := res.Data["historical_settlement"]; got != float64(19275) {
		t.Fatalf("historical settlement = %v, want 19275", got)
	}
	if got := res.Data["current_period_settlement"]; got != float64(0) {
		t.Fatalf("current period settlement = %v, want 0", got)
	}

	details, ok := res.Data["details"].([]map[string]any)
	if !ok || len(details) != 1 {
		t.Fatalf("details = %#v, want one counterparty detail", res.Data["details"])
	}
	detail := details[0]
	if detail["counterparty"] != "任拓" {
		t.Fatalf("counterparty = %v, want 任拓", detail["counterparty"])
	}
	if detail["closing_balance"] != float64(18190.20) {
		t.Fatalf("detail closing balance = %v, want 18190.20", detail["closing_balance"])
	}
	if detail["historical_settlement"] != float64(19275) {
		t.Fatalf("detail historical settlement = %v, want 19275", detail["historical_settlement"])
	}

	if !strings.Contains(res.Message, "期初 19275.00") || !strings.Contains(res.Message, "冲销历史应收 19275.00") {
		t.Fatalf("message should explain opening and historical settlement, got: %s", res.Message)
	}
}

func TestARAPNormalizesSummaryDerivedCounterpartyBeforeSettlement(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "arap-summary-normalize.db")
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
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-25', 'V-JC-0325', '112201', '单位', '为辽宁金程信息科技有限公司服务_辽宁金程信息科技有限公司_2026.03.25', '借', 1771649.01, 1771649.01, 0, '')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-06', 'V-JC-0306', '112201', '单位', '辽宁金程信息科技有限公司转账_辽宁金程信息科技有限公司_2026.03.06', '贷', 2130771.59, 0, 2130771.59, '')`,
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

	res := engine.Query("2026年3月账上应收账款情况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	if got := res.Data["historical_settlement"]; got != float64(0) {
		t.Fatalf("historical settlement = %v, want 0", got)
	}
	if got := res.Data["current_period_settlement"]; got != float64(0) {
		t.Fatalf("current period settlement = %v, want 0", got)
	}
	if got := res.Data["total"]; got != float64(1771649.01) {
		t.Fatalf("closing total = %v, want 1771649.01", got)
	}

	details, ok := res.Data["details"].([]map[string]any)
	if !ok || len(details) != 1 {
		t.Fatalf("details = %#v, want one normalized counterparty detail", res.Data["details"])
	}
	if details[0]["counterparty"] != "辽宁金程信息科技有限公司" {
		t.Fatalf("counterparty = %v, want 辽宁金程信息科技有限公司", details[0]["counterparty"])
	}
}

func TestARAPSummaryDerivedSettlementStaysProbableUntilDirectEvidenceExists(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "arap-probable-summary-match.db")
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
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-JC-OPEN', '112201', '单位', '为辽宁金程信息科技有限公司服务_辽宁金程信息科技有限公司_2026.02.28', '借', 1000, 1000, 0, '')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-06', 'V-JC-SETTLE', '112201', '单位', '辽宁金程信息科技有限公司转账_辽宁金程信息科技有限公司_2026.03.06', '贷', 1000, 0, 1000, '')`,
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

	res := engine.Query("2026年3月账上应收账款情况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	if got := res.Data["historical_settlement"]; got != float64(0) {
		t.Fatalf("confirmed historical settlement = %v, want 0", got)
	}
	confidence, ok := res.Data["settlement_confidence"].(map[string]any)
	if !ok {
		t.Fatalf("missing settlement_confidence: %+v", res.Data)
	}
	if got := confidence["probable_historical_settlement"]; got != float64(1000) {
		t.Fatalf("probable_historical_settlement = %v, want 1000", got)
	}
	if !strings.Contains(res.Message, "高概率冲销历史应收 1000.00") {
		t.Fatalf("message should downgrade to probable wording, got: %s", res.Message)
	}
}

func TestTaxQuestionsRenderCombinedAndNetMessages(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "tax-query.db")
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
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-26', 'V-TAX-2', '22210101', '进项税额', '进项税', '借', 170640.61, 170640.61, 0, '')`,
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

	combined := engine.Query("2026年3月销项税额是多少？2026年3月进项税额是多少？")
	if !combined.Success {
		t.Fatalf("combined tax query failed: %+v", combined)
	}
	if !strings.Contains(combined.Message, "销项税额 133631.18 元") || !strings.Contains(combined.Message, "进项税额 170640.61 元") {
		t.Fatalf("combined tax message mismatch: %s", combined.Message)
	}

	net := engine.Query("2026年3月净税额（销项-进项）是多少？")
	if !net.Success {
		t.Fatalf("net tax query failed: %+v", net)
	}
	if !strings.Contains(net.Message, "净税额 -37009.43 元") {
		t.Fatalf("net tax message mismatch: %s", net.Message)
	}
}

func TestTaxQuestionsAccumulate222101And222102InputTax(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "tax-input-prefixes.db")
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
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-25', 'V-TAX-1', '22210106', '销项税额', '销项税', '贷', 100, 0, 100, '')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-26', 'V-TAX-2', '22210101', '进项税额', '进项税', '借', 70, 70, 0, '')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-27', 'V-TAX-3', '22210201', '待认证进项税额', '待认证进项', '借', 30, 30, 0, '')`,
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

	res := engine.Query("2026年3月进项税额是多少？")
	if !res.Success {
		t.Fatalf("input tax query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "应计 100.00 元") {
		t.Fatalf("expected 222101 and 222102 both included, got: %s", res.Message)
	}
	if got, ok := res.Data["total_input"].(float64); !ok || got != 100 {
		t.Fatalf("total_input = %v, want 100", res.Data["total_input"])
	}
}

func TestPayableOpenItemsTreatNegativeCreditAsReduction(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "ap-negative-credit.db")
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
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-AP-1', '220201', '单位', '预提成本_北京欧特欧国际咨询有限公司_2026.02.28', '贷', 1600000, 0, 1600000, '')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', 'V-AP-2', '220201', '单位', '冲回预提成本_北京欧特欧国际咨询有限公司_2026.03.31', '贷', -1600000, 0, -1600000, '')`,
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

	res := engine.Query("2026年3月账上应付账款情况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["historical_settlement"]; got != float64(1600000) {
		t.Fatalf("historical settlement = %v, want 1600000", got)
	}
	if got := res.Data["total"]; got != float64(0) {
		t.Fatalf("closing total = %v, want 0", got)
	}
}

func TestGeneralARAPUsesBalanceSheetAsOfficialTotal(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "arap-official-priority.db")
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
		`CREATE TABLE balance_sheet (
			company TEXT,
			period TEXT,
			account_name TEXT,
			account_code TEXT,
			opening_balance REAL,
			closing_balance REAL
		)`,
		`CREATE TABLE balance_detail (
			company TEXT,
			year INTEGER,
			period TEXT,
			account_code TEXT,
			account_name TEXT,
			opening_debit REAL,
			opening_credit REAL,
			current_debit REAL,
			current_credit REAL,
			closing_debit REAL,
			closing_credit REAL
		)`,
		`INSERT INTO balance_sheet(company, period, account_name, account_code, opening_balance, closing_balance)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '应付账款', '2202', 1000, 1200)`,
		`INSERT INTO balance_detail(company, year, period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit)
		 VALUES ('南京优集数据科技有限公司', 2026, '2026-03', '2202', '应付账款', 0, 1000, 300, 500, 0, 1200)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-20', 'V-AP-BOOK-1', '220201', '单位', '收到供应商A发票_供应商A_2026.03.20', '贷', 500, 0, 500, '')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-25', 'V-AP-BOOK-2', '220201', '单位', '转账供应商A_供应商A_2026.03.25', '借', 300, 300, 0, '')`,
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

	res := engine.Query("2026年3月账上应付账款情况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["total"]; got != float64(1200) {
		t.Fatalf("official total = %v, want 1200", got)
	}
	if source, _ := res.Data["source"].(string); source != "balance_sheet" {
		t.Fatalf("source = %q, want balance_sheet", source)
	}
	openItem, ok := res.Data["open_item_analysis"].(map[string]any)
	if !ok {
		t.Fatalf("missing open_item_analysis: %+v", res.Data)
	}
	if openItem["total"] != float64(200) {
		t.Fatalf("open_item_analysis.total = %v, want 200", openItem["total"])
	}
	if !strings.Contains(res.Message, "期末余额 1200.00 元") {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestARAPSourceAdapterReturnsOfficialAndOpenItemFacts(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildARAPOfficialPriorityDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	adapter := query.NewARAPSourceAdapter(engine)
	spec := query.BuildQuerySpec("2026年3月账上应付账款情况", time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC))

	factSet, err := adapter.Fetch(context.Background(), spec)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	assertARAPFactValue(t, factSet, "official_arap_total", 1200)
	assertARAPFactValue(t, factSet, "official_arap_opening_balance", 1000)
	assertARAPFactValue(t, factSet, "openitem_closing_total", 200)
	assertARAPFactValue(t, factSet, "openitem_historical_settlement", 0)
}

func TestARAPQueryExposesSourceBackedFactSets(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildARAPOfficialPriorityDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月账上应付账款情况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	factSets, ok := res.Data["fact_sets"].([]query.FactSet)
	if !ok || len(factSets) == 0 {
		t.Fatalf("fact_sets missing or empty: %#v", res.Data["fact_sets"])
	}
	if factSets[0].Source != "arap" {
		t.Fatalf("fact set source = %s, want arap", factSets[0].Source)
	}
}

func buildARAPOfficialPriorityDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "arap-official-priority.db")
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
		`CREATE TABLE balance_sheet (
			company TEXT,
			period TEXT,
			account_name TEXT,
			account_code TEXT,
			opening_balance REAL,
			closing_balance REAL
		)`,
		`CREATE TABLE balance_detail (
			company TEXT,
			year INTEGER,
			period TEXT,
			account_code TEXT,
			account_name TEXT,
			opening_debit REAL,
			opening_credit REAL,
			current_debit REAL,
			current_credit REAL,
			closing_debit REAL,
			closing_credit REAL
		)`,
		`INSERT INTO balance_sheet(company, period, account_name, account_code, opening_balance, closing_balance)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '应付账款', '2202', 1000, 1200)`,
		`INSERT INTO balance_detail(company, year, period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit)
		 VALUES ('南京优集数据科技有限公司', 2026, '2026-03', '2202', '应付账款', 0, 1000, 300, 500, 0, 1200)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-20', 'V-AP-BOOK-1', '220201', '单位', '收到供应商A发票_供应商A_2026.03.20', '贷', 500, 0, 500, '')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-25', 'V-AP-BOOK-2', '220201', '单位', '转账供应商A_供应商A_2026.03.25', '借', 300, 300, 0, '')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}
	return dbPath
}

func assertARAPFactValue(t *testing.T, factSet query.FactSet, metricKey string, want float64) {
	t.Helper()
	for _, fact := range factSet.Facts {
		if fact.MetricKey == metricKey {
			if fact.Value != want {
				t.Fatalf("%s value = %v, want %v", metricKey, fact.Value, want)
			}
			return
		}
	}
	t.Fatalf("metricKey %s not found in facts: %+v", metricKey, factSet.Facts)
}
