package query

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOverallExpenseBreakdownQuestionReturnsAllPerspectives(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "expense-breakdown-all-perspectives.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL, source_report_type TEXT, source_sheet_name TEXT)`,
		`CREATE TABLE fin_cost_settlement_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, customer_name TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL, source_report_type TEXT, source_sheet_name TEXT, merge_range TEXT)`,
		`CREATE TABLE fin_cost_settlement_group_members (id INTEGER PRIMARY KEY AUTOINCREMENT, group_id INTEGER, contract_id TEXT, source_row_number INTEGER)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
		 ('C-001','南京供应商科技有限公司','技术服务项目A'),
		 ('C-002','上海数据服务有限公司','数据采购项目B'),
		 ('C-003','上海合并供应商科技有限公司','合并项目A')`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, settlement_amount, paid_amount, invoice_amount, source_report_type, source_sheet_name) VALUES
		 ('C-001','2026-03',700,400,700,'contract_revenue_cost','成本-月度结算'),
		 ('C-002','2026-03',300,300,300,'contract_revenue_cost','成本-月度结算')`,
		`INSERT INTO fin_cost_settlement_groups(id, customer_name, year_month, settlement_amount, paid_amount, invoice_amount, source_report_type, source_sheet_name, merge_range)
		 VALUES (1, '上海合并供应商科技有限公司', '2026-03', 200, 180, 200, 'contract_revenue_cost', '成本-月度结算', 'U3:U4')`,
		`INSERT INTO fin_cost_settlement_group_members(group_id, contract_id, source_row_number) VALUES (1, 'C-003', 3)`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary) VALUES
		 ('测试公司','2026-03-01',0,700,'南京供应商科技有限公司','技术服务付款'),
		 ('测试公司','2026-03-03',0,200,'张三','工资发放'),
		 ('测试公司','2026-03-05',0,50,'国家税务总局南京市税务局','缴纳增值税'),
		 ('测试公司','2026-03-06',0,10,'银行','手续费'),
		 ('测试公司','2026-03-08',1000,0,'客户A','回款')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-01','记-001','640101','主营业务成本','技术服务成本','借',1000,1000,0,'南京供应商科技有限公司'),
		 ('测试公司','2026-03','2026-03-03','记-002','660201','管理费用-工资','工资计提','借',200,200,0,'张三'),
		 ('测试公司','2026-03','2026-03-06','记-003','660301','财务费用-手续费','手续费','借',10,10,0,'银行')`,
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

	res := engine.Query("2026年3月整体支出，按三种口径拆分一下")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	for _, want := range []string{"合同/项目口径", "现金流水口径", "账务科目口径"} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("message should include %s, got: %s", want, res.Message)
		}
	}
	if strings.Contains(res.Message, "C-001") || strings.Contains(res.Message, "contract_id") {
		t.Fatalf("message should not expose contract ids, got: %s", res.Message)
	}

	views, ok := res.Data["breakdown_views"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown_views missing or wrong type: %T %+v", res.Data["breakdown_views"], res.Data)
	}
	assertBreakdownViewTotal(t, views, "contract_project", 1200)
	assertBreakdownViewTotal(t, views, "cash_category", 960)
	assertBreakdownViewTotal(t, views, "account_category", 1210)

	cashRows := rowsFromBreakdownView(t, views, "cash_category")
	assertExpenseCategoryAmount(t, cashRows, "供应商付款", 700)
	assertExpenseCategoryAmount(t, cashRows, "人力薪酬", 200)
	assertExpenseCategoryAmount(t, cashRows, "税费", 50)
	assertExpenseCategoryAmount(t, cashRows, "银行费用", 10)

	if !strings.Contains(res.Message, "来源：") || !strings.Contains(res.Message, "《合同成本结算表》") || !strings.Contains(res.Message, "《银行流水》") || !strings.Contains(res.Message, "《序时帐》") {
		t.Fatalf("message should include all source tables, got: %s", res.Message)
	}
}

func TestOverallExpenseBreakdownQuestionDefaultsToContractProjectPerspective(t *testing.T) {
	engine := newExpenseBreakdownPerspectiveTestEngine(t, true)
	defer engine.Close()

	res := engine.Query("2026年3月整体支出按大类拆分")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	views, ok := res.Data["breakdown_views"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown_views missing or wrong type: %T %+v", res.Data["breakdown_views"], res.Data)
	}
	if _, ok := views["contract_project"]; !ok {
		t.Fatalf("contract_project view missing: %+v", views)
	}
	if _, ok := views["cash_category"]; ok {
		t.Fatalf("cash_category should not be returned by default: %+v", views)
	}
	if _, ok := views["account_category"]; ok {
		t.Fatalf("account_category should not be returned by default: %+v", views)
	}
	if !strings.Contains(res.Message, "合同/项目口径") {
		t.Fatalf("message should prefer contract/project, got: %s", res.Message)
	}
	if strings.Contains(res.Message, "现金流水口径") || strings.Contains(res.Message, "账务科目口径") {
		t.Fatalf("message should not include fallback perspectives by default, got: %s", res.Message)
	}
}

func TestExpenseBreakdownExplicitCashPerspective(t *testing.T) {
	engine := newExpenseBreakdownPerspectiveTestEngine(t, true)
	defer engine.Close()

	res := engine.Query("2026年3月整体支出按现金流水大类拆分")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	views, ok := res.Data["breakdown_views"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown_views missing or wrong type: %T %+v", res.Data["breakdown_views"], res.Data)
	}
	assertBreakdownViewTotal(t, views, "cash_category", 700)
	if _, ok := views["contract_project"]; ok {
		t.Fatalf("contract_project should not be returned for explicit cash query: %+v", views)
	}
	if !strings.Contains(res.Message, "现金流水口径") {
		t.Fatalf("message should use cash perspective, got: %s", res.Message)
	}
}

func TestExpenseBreakdownExplicitAccountPerspective(t *testing.T) {
	engine := newExpenseBreakdownPerspectiveTestEngine(t, true)
	defer engine.Close()

	res := engine.Query("2026年3月整体支出按序时账科目拆分")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	views, ok := res.Data["breakdown_views"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown_views missing or wrong type: %T %+v", res.Data["breakdown_views"], res.Data)
	}
	assertBreakdownViewTotal(t, views, "account_category", 1000)
	if _, ok := views["contract_project"]; ok {
		t.Fatalf("contract_project should not be returned for explicit account query: %+v", views)
	}
	if !strings.Contains(res.Message, "账务科目口径") {
		t.Fatalf("message should use account perspective, got: %s", res.Message)
	}
}

func TestExpenseBreakdownDefaultsFallbackToAccountWhenContractRowsMissing(t *testing.T) {
	engine := newExpenseBreakdownPerspectiveTestEngine(t, false)
	defer engine.Close()

	res := engine.Query("2026年3月整体支出按大类拆分")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	views, ok := res.Data["breakdown_views"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown_views missing or wrong type: %T %+v", res.Data["breakdown_views"], res.Data)
	}
	assertBreakdownViewTotal(t, views, "account_category", 1000)
	if _, ok := views["contract_project"]; ok {
		t.Fatalf("empty contract_project should not be returned after fallback: %+v", views)
	}
	if !strings.Contains(res.Message, "已回退到账务科目口径") {
		t.Fatalf("message should explain fallback, got: %s", res.Message)
	}
}

func newExpenseBreakdownPerspectiveTestEngine(t *testing.T, includeContractRows bool) *Engine {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "expense-breakdown-perspective.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL, source_report_type TEXT, source_sheet_name TEXT)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-001','南京供应商科技有限公司','技术服务项目A')`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary) VALUES ('测试公司','2026-03-01',0,700,'南京供应商科技有限公司','技术服务付款')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES ('测试公司','2026-03','2026-03-01','记-001','640101','主营业务成本','技术服务成本','借',1000,1000,0,'南京供应商科技有限公司')`,
	}
	if includeContractRows {
		stmts = append(stmts, `INSERT INTO fin_cost_settlements(contract_id, year_month, settlement_amount, paid_amount, invoice_amount, source_report_type, source_sheet_name) VALUES ('C-001','2026-03',700,400,700,'contract_revenue_cost','成本-月度结算')`)
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
	return engine
}

func TestExpenseBreakdownUsesConfiguredCategories(t *testing.T) {
	rulesPath := filepath.Join(t.TempDir(), "rules.json")
	rulesJSON := `{
  "schema_version": 2,
  "router": {
    "expense_breakdown": {
      "cash_categories": [
        {"category": "租赁支出", "keywords": ["租赁付款", "房租"]}
      ],
      "account_categories": [
        {"category": "租赁支出", "keywords": ["租赁费"], "account_code_prefixes": ["660204"]}
      ]
    }
  }
}`
	if err := os.WriteFile(rulesPath, []byte(rulesJSON), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	dbPath := filepath.Join(t.TempDir(), "expense-breakdown-configured-categories.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL, source_report_type TEXT, source_sheet_name TEXT)`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary) VALUES
		 ('测试公司','2026-03-10',0,12000,'南京办公空间有限公司','办公室租赁付款')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-10','记-010','660204','管理费用-租赁费','办公室租赁费','借',12000,12000,0,'南京办公空间有限公司')`,
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

	res := engine.Query("2026年3月整体支出，按三种口径拆分一下")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	views, ok := res.Data["breakdown_views"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown_views missing or wrong type: %T %+v", res.Data["breakdown_views"], res.Data)
	}
	assertExpenseCategoryAmount(t, rowsFromBreakdownView(t, views, "cash_category"), "租赁支出", 12000)
	assertExpenseCategoryAmount(t, rowsFromBreakdownView(t, views, "account_category"), "租赁支出", 12000)
}

func assertBreakdownViewTotal(t *testing.T, views map[string]any, key string, want float64) {
	t.Helper()
	view, ok := views[key].(map[string]any)
	if !ok {
		t.Fatalf("%s view missing or wrong type: %T", key, views[key])
	}
	got, ok := view["total"].(float64)
	if !ok {
		t.Fatalf("%s total has wrong type: %T", key, view["total"])
	}
	if got != want {
		t.Fatalf("%s total = %.2f, want %.2f", key, got, want)
	}
}

func rowsFromBreakdownView(t *testing.T, views map[string]any, key string) []map[string]any {
	t.Helper()
	view, ok := views[key].(map[string]any)
	if !ok {
		t.Fatalf("%s view missing or wrong type: %T", key, views[key])
	}
	rows, ok := view["rows"].([]map[string]any)
	if !ok {
		t.Fatalf("%s rows missing or wrong type: %T", key, view["rows"])
	}
	return rows
}

func assertExpenseCategoryAmount(t *testing.T, rows []map[string]any, category string, want float64) {
	t.Helper()
	for _, row := range rows {
		if row["category"] != category {
			continue
		}
		got, ok := row["amount"].(float64)
		if !ok {
			t.Fatalf("%s amount has wrong type: %T", category, row["amount"])
		}
		if got != want {
			t.Fatalf("%s amount = %.2f, want %.2f", category, got, want)
		}
		return
	}
	t.Fatalf("category %q not found in %+v", category, rows)
}
