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

func TestCustomerContractQuestionUsesContractBookAndCashViews(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("辽宁金程信息科技有限公司2025年合同结算多少？其中10月到账多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "现金口径") || !strings.Contains(res.Message, "财务口径") {
		t.Fatalf("message should mention cash and financial views, got: %s", res.Message)
	}
	if strings.Index(res.Message, "现金口径") > strings.Index(res.Message, "财务口径") {
		t.Fatalf("contract answer should present cash view before financial view, got: %s", res.Message)
	}
	if got := res.Data["role"]; got != "customer_contract" {
		t.Fatalf("role = %v, want customer_contract", got)
	}
	if got := res.Data["sub_period_receipts"]; got != float64(1234) {
		t.Fatalf("sub_period_receipts = %v, want 1234", got)
	}
	if _, ok := res.Data["money_view"]; !ok {
		t.Fatalf("missing money_view alias: %+v", res.Data)
	}
	if _, ok := res.Data["account_view"]; !ok {
		t.Fatalf("missing account_view alias: %+v", res.Data)
	}
	if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
		t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
	}
}

func TestSupplierContractQuestionUsesContractCostAndBankPayments(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("南京林悦智能科技有限公司2025年合同成本多少？实际付款多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "现金口径") || !strings.Contains(res.Message, "财务口径") {
		t.Fatalf("message should mention cash and financial views, got: %s", res.Message)
	}
	if strings.Index(res.Message, "现金口径") > strings.Index(res.Message, "财务口径") {
		t.Fatalf("contract answer should present cash view before financial view, got: %s", res.Message)
	}
	if got := res.Data["role"]; got != "supplier_contract" {
		t.Fatalf("role = %v, want supplier_contract", got)
	}
	if got := res.Data["cash_paid_amount"]; got != float64(666) {
		t.Fatalf("cash_paid_amount = %v, want 666", got)
	}
	if _, ok := res.Data["money_view"]; !ok {
		t.Fatalf("missing money_view alias: %+v", res.Data)
	}
	if _, ok := res.Data["account_view"]; !ok {
		t.Fatalf("missing account_view alias: %+v", res.Data)
	}
}

func TestMixedContractQuestionUsesCashFirstDualAnswer(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("南京众信数通智能科技有限公司2025年合同收入结算、合同成本、到账、付款分别是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "现金口径") || !strings.Contains(res.Message, "财务口径") {
		t.Fatalf("message should mention cash and financial views, got: %s", res.Message)
	}
	if strings.Index(res.Message, "现金口径") > strings.Index(res.Message, "财务口径") {
		t.Fatalf("contract answer should present cash view before financial view, got: %s", res.Message)
	}
	if got := res.Data["role"]; got != "mixed_contract" {
		t.Fatalf("role = %v, want mixed_contract", got)
	}
	if _, ok := res.Data["money_view"]; !ok {
		t.Fatalf("missing money_view alias: %+v", res.Data)
	}
	if _, ok := res.Data["account_view"]; !ok {
		t.Fatalf("missing account_view alias: %+v", res.Data)
	}
}

func TestContractProfitQuestionWithoutContractKeywordStillUsesContractDimension(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("南京众信数通智能科技有限公司2025年利润多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
		t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != query.QueryFamilyContractDimension {
		t.Fatalf("query_family = %v, want %v", got, query.QueryFamilyContractDimension)
	}
	if !strings.Contains(res.Message, "合同利润 180.00 元") {
		t.Fatalf("message should contain contract book profit, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "净回款 192.00 元") {
		t.Fatalf("message should contain cash net receipts, got: %s", res.Message)
	}
}

func TestContractContentQuestionUsesContractDimension(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("行业商品数据采购合同A01内容是什么？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != query.QueryFamilyContractDimension {
		t.Fatalf("query_family = %v, want %v", got, query.QueryFamilyContractDimension)
	}
	if !strings.Contains(res.Message, "行业商品数据采购合同-A01") {
		t.Fatalf("message should contain contract content, got: %s", res.Message)
	}
}

func TestContractRevenueQuestionWithoutContractKeywordStillUsesContractDimension(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("辽宁金程信息科技有限公司2025年营收多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != query.QueryFamilyContractDimension {
		t.Fatalf("query_family = %v, want %v", got, query.QueryFamilyContractDimension)
	}
	if !strings.Contains(res.Message, "合同台账结算 3000.00 元") {
		t.Fatalf("message should contain contract settlement revenue, got: %s", res.Message)
	}
	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	if len(sourceTables) == 0 || sourceTables[0] != "tenant_uhub.fin_contracts" {
		t.Fatalf("source_tables should start with tenant_uhub.fin_contracts, got %#v", sourceTables)
	}
}

func TestCompanyAggregateMetricPrefersContractAggregateFirst(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月收入、成本、利润分别是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "老板口径先看合同/项目汇总") {
		t.Fatalf("message should prefer contract aggregate, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "营收 1300.00 元") || !strings.Contains(res.Message, "合同成本 1008.00 元") || !strings.Contains(res.Message, "利润 292.00 元") {
		t.Fatalf("message should use contract aggregate numbers, got: %s", res.Message)
	}
	if got, _ := res.Data["source_priority"].(string); got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first", res.Data["source_priority"])
	}
	if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
		t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != query.QueryFamilyCoreMetric {
		t.Fatalf("query_family = %v, want %v", got, query.QueryFamilyCoreMetric)
	}
	if got := spec["prefer_contract_aggregate"]; got != true {
		t.Fatalf("prefer_contract_aggregate = %v, want true", got)
	}
	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	if len(sourceTables) != 3 {
		t.Fatalf("source_tables count = %d, want 3", len(sourceTables))
	}
}

func TestCompanyAggregateMetricIncludesSourceNoteFromTableComment(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月收入是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "来源：") {
		t.Fatalf("message should include source note, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "《优集资金收入计算表-副本.xlsx》") {
		t.Fatalf("message should include workbook source, got: %s", res.Message)
	}
	sourceNote, _ := res.Data["source_note"].(string)
	if !strings.Contains(sourceNote, "25年Q4收入明细") || !strings.Contains(sourceNote, "26年Q1收入明细") {
		t.Fatalf("source_note should expose contract sheet lineage, got: %v", res.Data["source_note"])
	}
}

func TestCompanyAggregateMetricFallsBackWhenContractSummaryMissingCoverage(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`DELETE FROM fin_fund_income`,
		`DELETE FROM fin_cost_settlements`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司', '2025-10', '营业收入', 900, 900)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司', '2025-10', '营业成本', 600, 600)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司', '2025-10', '利润总额', 300, 300)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司', '2025-10', '净利润', 300, 300)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("prepare fallback data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月收入是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "已回退到现金+经营/财务口径") {
		t.Fatalf("message should explain contract fallback, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "先说现金口径") {
		t.Fatalf("message should fall back to dual perspective core metric, got: %s", res.Message)
	}
	if got := res.Data["contract_fallback_reason"]; got == nil {
		t.Fatalf("contract_fallback_reason missing: %+v", res.Data)
	}
}

func TestProjectMetricQuestionUsesContractDimensionRouting(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("辽宁金程信息科技有限公司项目2025年收入多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != query.QueryFamilyContractDimension {
		t.Fatalf("query_family = %v, want %v", got, query.QueryFamilyContractDimension)
	}
	if !strings.Contains(res.Message, "合同台账结算 3000.00 元") {
		t.Fatalf("message should use contract dimension result, got: %s", res.Message)
	}
}

func TestContractSourceAdapterReturnsCustomerContractFacts(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	adapter := query.NewContractSourceAdapter(engine)
	spec := query.BuildQuerySpec("辽宁金程信息科技有限公司2025年合同结算多少？其中10月到账多少？", contractAnchor())

	factSet, err := adapter.Fetch(context.Background(), spec)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if factSet.Source != "contracts" {
		t.Fatalf("source = %s, want contracts", factSet.Source)
	}
	assertFactValue(t, factSet, "contract_match_count", 1)
	assertFactValue(t, factSet, "contract_book_settlement", 3000)
	assertFactValue(t, factSet, "contract_book_invoice", 3000)
	assertFactValue(t, factSet, "contract_cash_received", 2734)
	assertFactValue(t, factSet, "contract_cash_received_subperiod", 1234)
}

func TestContractQueryExposesSourceBackedFactSets(t *testing.T) {
	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("辽宁金程信息科技有限公司2025年合同结算多少？其中10月到账多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	factSets, ok := res.Data["fact_sets"].([]query.FactSet)
	if !ok || len(factSets) == 0 {
		t.Fatalf("fact_sets missing or empty: %#v", res.Data["fact_sets"])
	}
	if factSets[0].Source != "contracts" {
		t.Fatalf("fact set source = %s, want contracts", factSets[0].Source)
	}
	assertFactValue(t, factSets[0], "contract_book_settlement", 3000)
	assertFactValue(t, factSets[0], "contract_cash_received_subperiod", 1234)
}

func buildContractQueryTestDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "contract-query.db")
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
			current_amount REAL,
			cumulative_amount REAL
		)`,
		`CREATE TABLE fin_contracts (
			contract_id TEXT PRIMARY KEY,
			customer_name TEXT,
			contract_content TEXT,
			created_at TEXT
		)`,
		`CREATE TABLE fin_cost_settlements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT,
			year_month TEXT,
			source_report_type TEXT,
			source_sheet_name TEXT,
			quantity TEXT,
			settlement_amount REAL,
			is_invoiced TEXT,
			account_code TEXT,
			created_at TEXT
		)`,
		`CREATE TABLE fin_fund_income (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT,
			year_month TEXT,
			source_report_type TEXT,
			source_sheet_name TEXT,
			settlement_amount REAL,
			received_amount REAL,
			is_invoiced TEXT,
			invoice_amount REAL,
			created_at TEXT
		)`,
		`CREATE TABLE meta_table_comments (
			table_name TEXT PRIMARY KEY,
			comment TEXT,
			updated_at TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	inserts := []string{
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C001', '辽宁金程信息科技有限公司', '行业商品数据采购合同-A01')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C002', '南京林悦智能科技有限公司', '技术服务采购合同-LY01')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C003', '南京众信数通智能科技有限公司', '数据服务合同-ZX01')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C001', '2025-10', 'contract_fund_income', '25年Q4收入明细', 1000, 1234, '是', 1000)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C001', '2025-11', 'contract_fund_income', '25年Q4收入明细', 2000, 1500, '是', 2000)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C003', '2025-10', 'contract_fund_income', '25年Q4收入明细', 300, 280, '是', 300)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, is_invoiced, account_code) VALUES ('C002', '2025-10', 'contract_revenue_cost', '成本-月度结算', '1人月', 888, '是', '640101')`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, is_invoiced, account_code) VALUES ('C003', '2025-10', 'contract_revenue_cost', '成本-月度结算', '1项', 120, '是', '640101')`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount) VALUES ('南京优集数据科技有限公司', '2025-10-18', '南京林悦智能科技有限公司', '合同付款', 666, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount) VALUES ('南京优集数据科技有限公司', '2025-10-22', '南京众信数通智能科技有限公司', '合同付款', 88, 0)`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_contracts', 'financeqa_source: {"display":"《合同信息表》","file_names":["优集资金收入计算表-副本.xlsx","优集成本计算表-4.23-池.xlsx"]}')`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_fund_income', 'financeqa_source: {"display":"《优集资金收入计算表-副本.xlsx》的【25年Q4收入明细】和【26年Q1收入明细】","file_names":["优集资金收入计算表-副本.xlsx"],"sheet_names":["25年Q4收入明细","26年Q1收入明细"]}')`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_cost_settlements', 'financeqa_source: {"display":"《优集成本计算表-4.23-池.xlsx》","file_names":["优集成本计算表-4.23-池.xlsx"]}')`,
	}
	for _, stmt := range inserts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert seed data failed: %v", err)
		}
	}

	return dbPath
}

func contractAnchor() time.Time {
	return time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
}
