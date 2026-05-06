package query

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBuildExecutionPlanForContractDimensionSkipsSilentGenericFallbackStages(t *testing.T) {
	ctx := queryExecutionContext{
		engine:        &Engine{},
		q:             "飞未云科2026年3月收入多少？",
		hasRealEntity: true,
		entity:        "飞未云科（深圳）技术有限公司",
		cfg:           getRuleConfig(),
		spec: QuerySpec{
			QueryFamily:            QueryFamilyContractDimension,
			NeedsContractDimension: true,
			Entity:                 "飞未云科（深圳）技术有限公司",
		},
	}

	plan := buildExecutionPlan(ctx)
	for _, stage := range plan {
		if stage == executionStageCounterpartyAuditFallback {
			t.Fatalf("contract-dimension plan should not silently inject counterparty audit fallback: %+v", plan)
		}
		if stage == executionStageIntentRoute {
			t.Fatalf("contract-dimension plan should not continue into generic intent route: %+v", plan)
		}
	}
}

func TestContractDimensionFailureStopsAtStrictContractSource(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contract-explicit-fallback.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, received_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, account_code TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-FW-001', '飞未云科（深圳）技术有限公司', '飞未项目')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-20','记-0001','600101','技术服务费','确认飞未云科收入','贷',500,0,500,'飞未云科（深圳）技术有限公司')`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary) VALUES
		 ('测试公司','2026-03-21',500,0,'飞未云科（深圳）技术有限公司','3月回款')`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES
		 ('测试公司','2026-03','营业收入',500,500),
		 ('测试公司','2026-03','利润总额',500,500),
		 ('测试公司','2026-03','净利润',500,500)`,
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

	res := engine.Query("飞未云科（深圳）技术有限公司2026年3月收入多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "已回退到财务账/流水口径") {
		t.Fatalf("message should disclose financial/cash fallback, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "账上确认收入 500.00") {
		t.Fatalf("message should expose fallback revenue amount, got: %s", res.Message)
	}
	if got, _ := res.Data["contract_fallback_reason"].(string); got == "" {
		t.Fatalf("contract_fallback_reason missing: %+v", res.Data)
	}
	sourceNote, _ := res.Data["source_note"].(string)
	if !strings.Contains(sourceNote, "《序时帐》") || !strings.Contains(sourceNote, "《银行流水》") {
		t.Fatalf("source_note should disclose fallback source, got %q", sourceNote)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != QueryFamilyContractDimension {
		t.Fatalf("query_family = %v, want %v", got, QueryFamilyContractDimension)
	}
}

func TestContractDimensionARAPFailureFallsBackToFinancialWhenContractMissing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contract-arap-source-fallback.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, received_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL, account_code TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-OTHER-001', '其他客户有限公司', '其他项目')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-08','记-AP-001','220201','应付账款','收到律师服务发票','贷',12000,0,12000,'北京市中闻（南京）律师事务所')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-08','记-AP-001','660201','管理费用','收到律师服务发票','借',12000,12000,0,'北京市中闻（南京）律师事务所')`,
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

	res := engine.Query("北京市中闻（南京）律师事务所2026年3月应付账款多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "已回退到财务账/流水口径") {
		t.Fatalf("message should disclose financial/cash fallback, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "期末余额 12000.00") {
		t.Fatalf("message should expose financial fallback amount, got: %s", res.Message)
	}
	sourceNote, _ := res.Data["source_note"].(string)
	if !strings.Contains(sourceNote, "《序时帐》") {
		t.Fatalf("source_note should disclose fallback journal source, got %q", sourceNote)
	}
	primary, ok := res.Data["primary_source_tables"].([]string)
	if !ok || !containsBaseTableForContractFallbackTest(primary, "fin_journal") {
		t.Fatalf("primary_source_tables = %#v, want journal fallback table", res.Data["primary_source_tables"])
	}
}

func TestContractStrictMissingSurfacesContinuityCandidatesForLLMInference(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contract-continuity-candidates.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, received_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL, account_code TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-OLD-001', '辽宁金程信息科技有限公司', '行业商品数据采购合同-A01')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-NEW-001', '四川其妙科技有限公司', '行业商品数据采购合同-A01')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES ('C-OLD-001', '2025-12', 1000, 1000, 0)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES ('C-NEW-001', '2026-01', 1200, 900, 900)`,
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

	res := engine.Query("金程2026年回款情况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got, _ := res.Data["contract_answer_status"].(string); got != "missing" {
		t.Fatalf("contract_answer_status = %q, want missing", got)
	}
	candidates, ok := res.Data["contract_continuity_candidates"].([]map[string]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("contract_continuity_candidates = %#v, want one candidate", res.Data["contract_continuity_candidates"])
	}
	candidate := candidates[0]
	if got := candidate["candidate_entity"]; got != "四川其妙科技有限公司" {
		t.Fatalf("candidate_entity = %v", got)
	}
	if got := candidate["contract_content"]; got != "行业商品数据采购合同-A01" {
		t.Fatalf("contract_content = %v", got)
	}
	if got := candidate["candidate_received_amount"]; got != float64(900) {
		t.Fatalf("candidate_received_amount = %v, want 900", got)
	}
	if got := candidate["basis"]; got != "same_contract_content_across_periods" {
		t.Fatalf("basis = %v", got)
	}
}

func TestExplicitFinancialARAPBypassesContractStrictSource(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "explicit-financial-arap.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, received_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL, account_code TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-08','记-AP-001','220201','应付账款','收到律师服务发票','贷',12000,0,12000,'北京市中闻（南京）律师事务所')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-08','记-AP-001','660201','管理费用','收到律师服务发票','借',12000,12000,0,'北京市中闻（南京）律师事务所')`,
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

	res := engine.Query("北京市中闻（南京）律师事务所2026年3月账上应付账款多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "期末余额 12000.00") {
		t.Fatalf("explicit financial AR/AP should return journal rollforward amount, got: %s", res.Message)
	}
	sourceNote, _ := res.Data["source_note"].(string)
	if !strings.Contains(sourceNote, "《序时帐》") {
		t.Fatalf("source_note should disclose financial journal source, got %q", sourceNote)
	}
	if strings.Contains(sourceNote, "优集资金收入计算表") || strings.Contains(sourceNote, "优集成本计算表") {
		t.Fatalf("explicit financial AR/AP should not use contract workbook source, got %q", sourceNote)
	}
}

func containsStringForContractFallbackTest(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsBaseTableForContractFallbackTest(items []string, want string) bool {
	for _, item := range items {
		if baseSourceTableName(item) == want {
			return true
		}
	}
	return false
}

func containsAnyForContractFallbackTest(s string, items []string) bool {
	for _, item := range items {
		if strings.Contains(s, item) {
			return true
		}
	}
	return false
}
