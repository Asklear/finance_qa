package query

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestResolveQueryRoutingPromotesContractPriorityToContractDimension(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	route := engine.resolveQueryRouting("飞未云科2026年累计销售额多少？")
	if route.entity != "飞未云科（深圳）技术有限公司" {
		t.Fatalf("entity = %q, want %q", route.entity, "飞未云科（深圳）技术有限公司")
	}
	if route.spec.QueryFamily != QueryFamilyContractDimension {
		t.Fatalf("query_family = %s, want %s", route.spec.QueryFamily, QueryFamilyContractDimension)
	}
	if route.spec.PeriodFrom != "2026-01" || route.spec.PeriodTo != "2026-03" {
		t.Fatalf("period = %s~%s, want 2026-01~2026-03", route.spec.PeriodFrom, route.spec.PeriodTo)
	}
	if !route.hasRealEntity {
		t.Fatalf("expected hasRealEntity=true")
	}
}

func TestResolveQueryRoutingTreatsExplicitBankCashAsCompanyCash(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	route := engine.resolveQueryRouting("2026年3月银行卡实际到账多少？")
	if route.spec.SourceConstraint != BossSourceBankStatement {
		t.Fatalf("source_constraint = %q, want %q", route.spec.SourceConstraint, BossSourceBankStatement)
	}
	if route.spec.QueryFamily == QueryFamilyCounterparty {
		t.Fatalf("query_family = %s, want non-counterparty company cash route", route.spec.QueryFamily)
	}
	if route.entity != "" || route.hasRealEntity {
		t.Fatalf("entity = %q hasRealEntity=%t, want no business entity", route.entity, route.hasRealEntity)
	}
}

func TestExplicitBankCashReceiptQueryAnswersFromBankStatement(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月银行卡实际到账多少？")
	if !res.Success {
		t.Fatalf("query success = false, message=%s", res.Message)
	}
	if !strings.Contains(res.Message, "实际到账 1200.00 元") {
		t.Fatalf("message = %q, want actual receipt amount", res.Message)
	}
	if !strings.Contains(res.Message, "来源：《银行流水》") {
		t.Fatalf("message = %q, want bank source disclosure", res.Message)
	}
}

func TestResolveQueryRoutingDoesNotTreatOverallExpenseAsEntity(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	route := engine.resolveQueryRouting("2026年3月整体支出按大类拆一下")
	if route.entity != "" || route.hasRealEntity {
		t.Fatalf("entity = %q hasRealEntity=%t, want no business entity", route.entity, route.hasRealEntity)
	}
	if route.spec.QueryFamily == QueryFamilyCounterparty {
		t.Fatalf("query_family = %s, want non-counterparty route", route.spec.QueryFamily)
	}

	contractViewRoute := engine.resolveQueryRouting("2026年3月整体支出按合同拆一下")
	if contractViewRoute.spec.NeedsContractDimension || contractViewRoute.spec.QueryFamily == QueryFamilyContractDimension {
		t.Fatalf("overall expense contract-view breakdown should not require a specific contract entity, spec=%+v", contractViewRoute.spec)
	}
	if contractViewRoute.spec.BossRewrite.Scope != BossScopeCompany || contractViewRoute.spec.BossRewrite.Granularity != BossGranularityBreakdown {
		t.Fatalf("boss rewrite = %+v, want company breakdown", contractViewRoute.spec.BossRewrite)
	}
}

func TestResolveQueryRoutingKeepsReadinessFamilyAndResolvedEntity(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	route := engine.resolveQueryRouting("南京林悦智能科技有限公司3月数据出来了吗？")
	if route.entity != "南京林悦智能科技有限公司" {
		t.Fatalf("entity = %q, want %q", route.entity, "南京林悦智能科技有限公司")
	}
	if route.spec.QueryFamily != QueryFamilyReadiness {
		t.Fatalf("query_family = %s, want %s", route.spec.QueryFamily, QueryFamilyReadiness)
	}
	if !route.spec.ReadinessCheckRequired {
		t.Fatalf("expected readiness flag to stay true")
	}
}

func TestResolveQueryRoutingKeepsClassificationQuestionOffContractPriorityPath(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	route := engine.resolveQueryRouting("飞未云科这个主体目前更像客户、供应商还是混合往来？")
	if route.entity != "飞未云科（深圳）技术有限公司" {
		t.Fatalf("entity = %q, want %q", route.entity, "飞未云科（深圳）技术有限公司")
	}
	if route.spec.QueryFamily == QueryFamilyContractDimension {
		t.Fatalf("query_family = %s, want non-contract classification route", route.spec.QueryFamily)
	}
}

func TestResolveQueryRoutingUsesContractAnchorForRelativeContractQuestions(t *testing.T) {
	dbPath := buildQueryContextContractAnchorDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	route := engine.resolveQueryRouting("飞未云科本月销售额多少？")
	if route.spec.QueryFamily != QueryFamilyContractDimension {
		t.Fatalf("query_family = %s, want %s", route.spec.QueryFamily, QueryFamilyContractDimension)
	}
	if route.spec.PeriodFrom != "2026-03" || route.spec.PeriodTo != "2026-03" {
		t.Fatalf("period = %s~%s, want 2026-03~2026-03", route.spec.PeriodFrom, route.spec.PeriodTo)
	}
	if got := route.anchor.Format("2006-01"); got != "2026-03" {
		t.Fatalf("anchor = %s, want 2026-03", got)
	}
}

func buildQueryContextResolutionDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "query-context-resolution.sqlite")
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
			account_code TEXT,
			account_name TEXT,
			opening_balance REAL,
			closing_balance REAL
		)`,
		`CREATE TABLE fin_contracts (
			contract_id TEXT PRIMARY KEY,
			customer_name TEXT,
			contract_content TEXT
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
			invoice_amount REAL
		)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-03','1002','货币资金',100,150)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C-FW-001','飞未云科（深圳）技术有限公司','飞未项目-京东价格数据')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-001','2026-03','contract_fund_income','26年Q1收入明细',900,900,'是',900)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('测试公司','2026-03','2026-03-31','V-READY-1','6401','主营业务成本','林悦3月成本确认','借',500,500,0,'南京林悦智能科技有限公司')`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('测试公司','2026-03-20','南京林悦智能科技有限公司','合同款',500,0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('测试公司','2026-03-21','招商银行','收款',0,1200)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	return dbPath
}

func buildQueryContextContractAnchorDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "query-context-contract-anchor.sqlite")
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
		`CREATE TABLE bank_statement (
			company TEXT,
			transaction_date TEXT,
			counterparty_name TEXT,
			summary TEXT,
			debit_amount REAL,
			credit_amount REAL
		)`,
		`CREATE TABLE fin_contracts (
			contract_id TEXT PRIMARY KEY,
			customer_name TEXT,
			contract_content TEXT
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
			invoice_amount REAL
		)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('测试公司','2026-04','2026-04-30','J-NEW-1','6001','主营业务收入','4月账务更新','贷',100,0,100,'其他客户')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C-FW-ANCHOR-1','飞未云科（深圳）技术有限公司','飞未项目-京东价格数据')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-ANCHOR-1','2026-03','contract_fund_income','26年Q1收入明细',900,900,'是',900)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	return dbPath
}
