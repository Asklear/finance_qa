package query

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestGenericReceivableQuestionUsesContractAggregateBeforeBalanceSheet(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月应收账款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(600) {
		t.Fatalf("total = %v, want contract receivable 600", got)
	}
	if strings.Contains(res.Message, "科目余额表") {
		t.Fatalf("generic receivable should not answer from balance sheet only, got message=%q", res.Message)
	}
	if !strings.Contains(res.Message, "合同") || !strings.Contains(res.Message, "应收") {
		t.Fatalf("message should disclose contract receivable口径, got %q", res.Message)
	}
}

func TestExplicitBalanceSheetReceivableQuestionKeepsOfficialARAP(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月科目余额中的应收账款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source"]; got != "balance_sheet" {
		t.Fatalf("source = %v, want balance_sheet; data=%+v", got, res.Data)
	}
	if got := res.Data["total"]; got != float64(9999) {
		t.Fatalf("total = %v, want official balance 9999", got)
	}
	if !strings.Contains(res.Message, "科目余额表") {
		t.Fatalf("message should disclose balance sheet source, got %q", res.Message)
	}
}

func TestGenericPayableQuestionUsesContractAggregateBeforeBalanceSheet(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月应付账款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(500) {
		t.Fatalf("total = %v, want contract payable 500", got)
	}
	if !strings.Contains(res.Message, "合同") || !strings.Contains(res.Message, "应付") {
		t.Fatalf("message should disclose contract payable口径, got %q", res.Message)
	}
}

func TestInvoicedUnpaidQuestionUsesInvoiceGapWithoutSyntheticEntity(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年已开票未付款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(400) {
		t.Fatalf("total = %v, want invoice gap 400", got)
	}
	if entity := res.Data["entity"]; entity != nil && entity != "" {
		t.Fatalf("synthetic entity should be empty, got %v", entity)
	}
	if !strings.Contains(res.Message, "已开票未回款") {
		t.Fatalf("message should explain customer-side invoice gap, got %q", res.Message)
	}
}

func TestProjectInvoiceOpenRosterQuestionDoesNotExtractSyntheticEntity(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("有哪些项目已开票未回款")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(400) {
		t.Fatalf("total = %v, want invoice open amount 400", got)
	}
	if entity := res.Data["entity"]; entity != nil && entity != "" {
		t.Fatalf("synthetic entity should be empty, got %v", entity)
	}
	if strings.Contains(res.Message, "没有识别到合同/项目主体") || strings.Contains(res.Message, "合同口径当前不能直接回答") {
		t.Fatalf("project roster question should answer company-scope invoice open amount, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "已开票未回款") {
		t.Fatalf("message should explain invoice open amount, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "测试客户") || !strings.Contains(res.Message, "测试客户项目") {
		t.Fatalf("message should list customer and project content, got %q", res.Message)
	}
	if strings.Contains(res.Message, "C-001") {
		t.Fatalf("message should not expose internal contract id, got %q", res.Message)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	items, ok := summary["invoice_open_items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("invoice_open_items = %#v, want one item", summary["invoice_open_items"])
	}
	if got := items[0]["customer_name"]; got != "测试客户" {
		t.Fatalf("invoice_open_items[0].customer_name = %v", got)
	}
	if got := items[0]["contract_content"]; got != "测试客户项目" {
		t.Fatalf("invoice_open_items[0].contract_content = %v", got)
	}
	if got := items[0]["open_amount"]; got != float64(400) {
		t.Fatalf("invoice_open_items[0].open_amount = %v, want 400", got)
	}
}

func TestReceivedInvoiceUnpaidQuestionUsesSupplierInvoiceGap(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月已收票未付款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(300) {
		t.Fatalf("total = %v, want supplier invoice unpaid 300", got)
	}
	if !strings.Contains(res.Message, "已收票未付款") {
		t.Fatalf("message should explain supplier-side invoice gap, got %q", res.Message)
	}
}

func buildContractARAPPriorityDB(t *testing.T) string {
	t.Helper()
	dbPath := t.TempDir() + "/contract-arap-priority.sqlite"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, opening_period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, transaction_time TEXT, transaction_type TEXT, debit_amount REAL, credit_amount REAL, balance REAL, summary TEXT, counterparty_name TEXT, counterparty_account TEXT)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, received_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, paid_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('测试公司','2026-03','1122','应收账款',0,9999)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('测试公司','2026-03','2202','应付账款',0,8888)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-001','测试客户','测试客户项目')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-002','测试供应商','测试供应商项目')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C-001','2026-03','contract_fund_income','26年Q1收入明细',1000,400,'是',800)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-002','2026-03','contract_revenue_cost','成本-月度结算',700,200,'是',500)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v\n%s", err, stmt)
		}
	}
	return dbPath
}
