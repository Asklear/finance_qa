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

func TestContractDimensionFailureFallsBackExplicitlyInsteadOfSilentlyHijacking(t *testing.T) {
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
	if !strings.Contains(res.Message, "合同台账当前不能直接回答") {
		t.Fatalf("message should disclose explicit contract fallback, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "已回退到财务账/流水口径") {
		t.Fatalf("message should disclose fallback destination, got: %s", res.Message)
	}
	if got, _ := res.Data["contract_fallback_reason"].(string); got == "" {
		t.Fatalf("contract_fallback_reason missing: %+v", res.Data)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != QueryFamilyContractDimension {
		t.Fatalf("query_family = %v, want %v", got, QueryFamilyContractDimension)
	}
}
