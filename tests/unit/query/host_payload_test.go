package query_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestHostPayloadBalanceDetailShouldRespectPeriodRange(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "host-payload.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, transaction_time TEXT, transaction_type TEXT, debit_amount REAL, credit_amount REAL, balance REAL, summary TEXT, counterparty_name TEXT, counterparty_account TEXT)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	seed := []string{
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('南京优集数据科技有限公司','2026-03','1122','应收账款',1,2)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司','2026-03','一、营业收入',100,100)`,
		`INSERT INTO balance_detail(company, year, period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit) VALUES ('南京优集数据科技有限公司',2026,'2026-02','1122','应收账款',10,0,20,5,25,0)`,
		`INSERT INTO balance_detail(company, year, period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit) VALUES ('南京优集数据科技有限公司',2026,'2026-03','1122','应收账款',25,0,30,10,45,0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES ('南京优集数据科技有限公司','2026-03','2026-03-20','记-1','1122','应收账款','测试','借',100,100,0,'测试客户')`,
		`INSERT INTO bank_statement(company, transaction_date, transaction_time, transaction_type, debit_amount, credit_amount, balance, summary, counterparty_name, counterparty_account) VALUES ('南京优集数据科技有限公司','2026-03-20','10:00:00','转账',0,100,1000,'测试回款','测试客户','xx')`,
	}
	for _, stmt := range seed {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.HostLLMPayload("2026-03", "2026-03", "测试问题")
	if !res.Success {
		t.Fatalf("HostLLMPayload failed: %+v", res)
	}

	payload, ok := res.Data["llm_payload"].(map[string]any)
	if !ok {
		t.Fatalf("missing llm_payload: %+v", res.Data)
	}
	financialTables, ok := payload["financial_tables"].(map[string]any)
	if !ok {
		t.Fatalf("missing financial_tables: %+v", payload)
	}
	bdRaw, ok := financialTables["balance_detail"].([]map[string]any)
	if !ok {
		t.Fatalf("balance_detail type mismatch: %T", financialTables["balance_detail"])
	}
	if len(bdRaw) != 1 {
		t.Fatalf("balance_detail should contain exactly one period row, got=%d rows=%v", len(bdRaw), bdRaw)
	}
	if period, _ := bdRaw[0]["period"].(string); period != "2026-03" {
		t.Fatalf("unexpected balance_detail period=%v", bdRaw[0]["period"])
	}
}

func TestHostPayloadReturnsFailureWhenExtractionIsIncomplete(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "host-payload-incomplete.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		// 故意缺少 counterparty_account，模拟 schema drift。
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, transaction_time TEXT, transaction_type TEXT, debit_amount REAL, credit_amount REAL, balance REAL, summary TEXT, counterparty_name TEXT)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.HostLLMPayload("2026-03", "2026-03", "2026年3月收入是多少？")
	if res.Success {
		t.Fatalf("HostLLMPayload should fail on incomplete extraction, got success with data=%+v", res.Data)
	}
	errs, ok := res.Data["extraction_errors"].([]string)
	if !ok || len(errs) == 0 {
		t.Fatalf("extraction_errors missing: %+v", res.Data)
	}
	if !strings.Contains(strings.Join(errs, "\n"), "bank_statement") {
		t.Fatalf("extraction_errors should mention bank_statement, got %v", errs)
	}
}

func TestHostPayloadIncludesContractDetailsAndSourceNote(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := filepath.Join(t.TempDir(), "host-payload-contracts.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, opening_period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, transaction_time TEXT, transaction_type TEXT, debit_amount REAL, credit_amount REAL, balance REAL, summary TEXT, counterparty_name TEXT, counterparty_account TEXT)`,
		`CREATE TABLE fin_contracts (
			contract_id TEXT PRIMARY KEY,
			customer_name TEXT,
			contract_content TEXT,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT
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
			invoice_amount REAL,
			paid_amount REAL,
			account_code TEXT,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT
		)`,
		`CREATE TABLE fin_cost_settlement_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_name TEXT,
			year_month TEXT,
			source_report_type TEXT,
			source_sheet_name TEXT,
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			quantity TEXT,
			settlement_amount REAL,
			is_invoiced TEXT,
			invoice_amount REAL,
			paid_amount REAL,
			account_code TEXT,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT
		)`,
		`CREATE TABLE fin_cost_settlement_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER,
			contract_id TEXT,
			source_row_number INTEGER
		)`,
		`CREATE TABLE fin_fund_income (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT,
			year_month TEXT,
			source_report_type TEXT,
			source_sheet_name TEXT,
			quantity TEXT,
			settlement_amount REAL,
			received_amount REAL,
			is_invoiced TEXT,
			invoice_amount REAL,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT
		)`,
		`CREATE TABLE fin_fund_income_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_name TEXT,
			year_month TEXT,
			source_report_type TEXT,
			source_sheet_name TEXT,
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			quantity TEXT,
			settlement_amount REAL,
			received_amount REAL,
			is_invoiced TEXT,
			invoice_amount REAL,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT
		)`,
		`CREATE TABLE fin_fund_income_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER,
			contract_id TEXT,
			source_row_number INTEGER
		)`,
		`CREATE TABLE meta_table_comments (
			table_name TEXT PRIMARY KEY,
			comment TEXT,
			updated_at TEXT
		)`,
		`CREATE TABLE fin_file_mappings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_type TEXT,
			period TEXT,
			company TEXT,
			storage_key TEXT,
			file_name TEXT,
			description TEXT,
			file_size INTEGER,
			created_at TEXT,
			updated_at TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	seed := []string{
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('南京优集数据科技有限公司','2026-03','1122','应收账款',1,2)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司','2026-03','一、营业收入',100,100)`,
		`INSERT INTO balance_detail(company, year, period, opening_period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit) VALUES ('南京优集数据科技有限公司',2026,'2026-03','2026-01','1122','应收账款',10,0,20,5,25,0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES ('南京优集数据科技有限公司','2026-03','2026-03-20','记-1','1122','应收账款','测试','借',100,100,0,'飞未云科（深圳）技术有限公司')`,
		`INSERT INTO bank_statement(company, transaction_date, transaction_time, transaction_type, debit_amount, credit_amount, balance, summary, counterparty_name, counterparty_account) VALUES ('南京优集数据科技有限公司','2026-03-20','10:00:00','转账',0,100,1000,'测试回款','飞未云科（深圳）技术有限公司','xx')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content, contract_start_date, contract_end_date, settlement_cycle) VALUES ('C-FW-001', '飞未云科（深圳）技术有限公司', '飞未云科项目-京东价格数据', '2026-01-01', '2026-12-31', '月结')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, received_amount, is_invoiced, invoice_amount, contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price) VALUES ('C-FW-001', '2026-03', 'contract_fund_income', '26年Q1收入明细', '1项', 900, 900, '是', 900, '2026-01-01', '2026-12-31', '月结', '900元/项')`,
		`INSERT INTO fin_fund_income_groups(id, customer_name, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, merge_range, quantity, settlement_amount, received_amount, is_invoiced, invoice_amount, contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price) VALUES (1, '飞未云科（深圳）技术有限公司', '2026-03', 'contract_fund_income', '26年Q1收入明细', 8, 9, 'R8:R9', '1项', 100, 100, '是', 100, '2026-01-01', '2026-12-31', '月结', '100元/项')`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number) VALUES (1, 'C-FW-001', 8)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code, contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price) VALUES ('C-FW-001', '2026-03', 'contract_revenue_cost', '成本-月度结算', '1项', 200, '是', 200, 180, '640101', '2026-01-01', '2026-12-31', '月结', '200元/项')`,
		`INSERT INTO fin_cost_settlement_groups(id, customer_name, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, merge_range, quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code, contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price) VALUES (1, '飞未云科（深圳）技术有限公司', '2026-03', 'contract_revenue_cost', '成本-月度结算', 8, 9, 'U8:U9', '1项', 80, '是', 80, 70, '640101', '2026-01-01', '2026-12-31', '月结', '80元/项')`,
		`INSERT INTO fin_cost_settlement_group_members(group_id, contract_id, source_row_number) VALUES (1, 'C-FW-001', 8)`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_fund_income', 'financeqa_source: {"display":"《优集资金收入计算表-副本.xlsx》","file_names":["优集资金收入计算表-副本.xlsx"],"sheet_names":["26年Q1收入明细"]}')`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_cost_settlements', 'financeqa_source: {"display":"《优集成本计算表-4.23-池.xlsx》","file_names":["优集成本计算表-4.23-池.xlsx"],"sheet_names":["成本-月度结算"]}')`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_contracts', 'financeqa_source: {"display":"《合同信息表》","file_names":["优集资金收入计算表-副本.xlsx","优集成本计算表-4.23-池.xlsx"]}')`,
		`INSERT INTO fin_file_mappings(table_type, period, company, storage_key, file_name, updated_at) VALUES
		 ('fund-income', '2026-Q1', '南京优集数据科技有限公司', 'tenant/uhub/finance/2026/优集收入、成本计算表 - 上传.xlsx', '优集收入、成本计算表 - 上传.xlsx', '2026-05-06 09:30:00'),
		 ('cost-settlements', '2026-Q1', '南京优集数据科技有限公司', 'tenant/uhub/finance/2026/优集收入、成本计算表 - 上传.xlsx', '优集收入、成本计算表 - 上传.xlsx', '2026-05-06 09:30:00')`,
	}
	for _, stmt := range seed {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.HostLLMPayload("2026-03", "2026-03", "飞未云科2026年累计销售额多少？")
	if !res.Success {
		t.Fatalf("HostLLMPayload failed: %+v", res)
	}
	sourceNote, _ := res.Data["source_note"].(string)
	if strings.TrimSpace(sourceNote) == "" {
		t.Fatalf("source_note should not be empty: %+v", res.Data)
	}
	if !strings.Contains(sourceNote, "优集收入、成本计算表 - 上传.xlsx") {
		t.Fatalf("source_note should use fin_file_mappings file name, got %q", sourceNote)
	}
	for _, stale := range []string{"优集资金收入计算表-副本.xlsx", "优集成本计算表-4.23-池.xlsx", "合同信息表"} {
		if strings.Contains(sourceNote, stale) {
			t.Fatalf("source_note should not fall back to stale table comments or hardcoded labels, got %q", sourceNote)
		}
	}

	payload, ok := res.Data["llm_payload"].(map[string]any)
	if !ok {
		t.Fatalf("missing llm_payload: %+v", res.Data)
	}
	sourceCatalogText := strings.TrimSpace(anyForHostPayloadTest(payload["source_catalog"]))
	if !strings.Contains(sourceCatalogText, "优集收入、成本计算表 - 上传.xlsx") {
		t.Fatalf("source_catalog should use fin_file_mappings file name, got %s", sourceCatalogText)
	}
	for _, stale := range []string{"优集资金收入计算表-副本.xlsx", "优集成本计算表-4.23-池.xlsx", "合同信息表"} {
		if strings.Contains(sourceCatalogText, stale) {
			t.Fatalf("source_catalog should not expose stale table comments or hardcoded labels, got %s", sourceCatalogText)
		}
	}
	financialTables, ok := payload["financial_tables"].(map[string]any)
	if !ok {
		t.Fatalf("missing financial_tables: %+v", payload)
	}

	contracts, ok := financialTables["fin_contracts"].([]map[string]any)
	if !ok || len(contracts) != 1 {
		t.Fatalf("fin_contracts payload mismatch: %#v", financialTables["fin_contracts"])
	}
	if contracts[0]["contract_start_date"] != "2026-01-01" {
		t.Fatalf("contract_start_date missing from payload: %#v", contracts[0])
	}

	fundRows, ok := financialTables["fin_fund_income"].([]map[string]any)
	if !ok || len(fundRows) != 1 {
		t.Fatalf("fin_fund_income payload mismatch: %#v", financialTables["fin_fund_income"])
	}
	if fundRows[0]["source_sheet_name"] != "26年Q1收入明细" {
		t.Fatalf("source_sheet_name missing from fin_fund_income payload: %#v", fundRows[0])
	}
	fundGroupRows, ok := financialTables["fin_fund_income_groups"].([]map[string]any)
	if !ok || len(fundGroupRows) != 1 {
		t.Fatalf("fin_fund_income_groups payload mismatch: %#v", financialTables["fin_fund_income_groups"])
	}
	if fundGroupRows[0]["merge_range"] != "R8:R9" {
		t.Fatalf("merge_range missing from fin_fund_income_groups payload: %#v", fundGroupRows[0])
	}

	costRows, ok := financialTables["fin_cost_settlements"].([]map[string]any)
	if !ok || len(costRows) != 1 {
		t.Fatalf("fin_cost_settlements payload mismatch: %#v", financialTables["fin_cost_settlements"])
	}
	if costRows[0]["paid_amount"] != float64(180) {
		t.Fatalf("paid_amount missing from fin_cost_settlements payload: %#v", costRows[0])
	}
	costGroupRows, ok := financialTables["fin_cost_settlement_groups"].([]map[string]any)
	if !ok || len(costGroupRows) != 1 {
		t.Fatalf("fin_cost_settlement_groups payload mismatch: %#v", financialTables["fin_cost_settlement_groups"])
	}
	if costGroupRows[0]["paid_amount"] != float64(70) {
		t.Fatalf("paid_amount missing from fin_cost_settlement_groups payload: %#v", costGroupRows[0])
	}

	routeDecision, ok := payload["route_decision"].(map[string]any)
	if !ok {
		t.Fatalf("route_decision should be included for host summarizer: %#v", payload["route_decision"])
	}
	if got := routeDecision["selected_source"]; got != "contract_aggregate" {
		t.Fatalf("route_decision.selected_source = %v, want contract_aggregate", got)
	}
	probeResults, ok := routeDecision["probe_results"].([]map[string]any)
	if !ok || len(probeResults) == 0 {
		t.Fatalf("route_decision.probe_results missing: %#v", routeDecision["probe_results"])
	}
	sourceDocs, ok := probeResults[0]["source_documents"].([]string)
	if !ok || len(sourceDocs) == 0 {
		t.Fatalf("probe source_documents should be included: %#v", probeResults[0])
	}
}

func anyForHostPayloadTest(v any) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(fmt.Sprintf("%#v", v)), "\n", " "), "\t", " "))
}
