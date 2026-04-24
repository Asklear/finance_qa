package query_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestHRBreakdownUsesConfigurableAccountMappings(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
 "accounting": {
    "hr_breakdown_account_codes": {
      "wage": ["661001"],
      "social": ["661002"],
      "housing": ["661003"]
    },
    "hr_cash_bank_account_prefixes": ["1022"],
    "hr_payroll_liability_account_prefixes": ["2251"],
    "hr_payroll_liability_name_keywords": ["待发薪酬"],
    "hr_category_keywords": {
      "wage": ["员工薪酬"],
      "social": ["社保代扣"],
      "housing": ["住房金"]
    }
  },
  "internal_party": {
    "org_suffixes": ["事业部"]
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	dbPath := filepath.Join(t.TempDir(), "hr-breakdown-config.db")
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
		`CREATE TABLE balance_sheet (
			company TEXT,
			period TEXT,
			account_name TEXT,
			account_code TEXT,
			opening_balance REAL,
			closing_balance REAL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	seed := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', 'ACC-1', '661001', '员工薪酬', '借', 12000, '计提3月员工薪酬', '', 12000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', 'ACC-2', '661002', '社保代扣', '借', 900, '计提3月社保代扣', '', 900, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', 'ACC-3', '661003', '住房金', '借', 300, '计提3月住房金', '', 300, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-10', 'PAY-1', '225101', '待发薪酬', '借', 7000, '员工薪酬代发', '', 7000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-10', 'PAY-1', '102201', '银行存款', '贷', 7000, '代发', '', 0, 7000)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-11', 'PAY-2', '225102', '待发薪酬', '借', 500, '社保代扣支付', '', 500, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-11', 'PAY-2', '102201', '银行存款', '贷', 500, '代发', '', 0, 500)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-12', 'PAY-3', '225103', '待发薪酬', '借', 200, '住房金支付', '', 200, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-12', 'PAY-3', '102201', '银行存款', '贷', 200, '代发', '', 0, 200)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-13', 'PAY-4', '225101', '待发薪酬', '借', 1000, '深圳事业部划款', '深圳事业部', 1000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-13', 'PAY-4', '102201', '银行存款', '贷', 1000, '事业部划款', '深圳事业部', 0, 1000)`,
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

	res := engine.Query("2026年3月人力成本多少？工资、社保、公积金分别是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	hr, ok := res.Data["hr_breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("missing hr_breakdown: %+v", res.Data)
	}
	acc, ok := hr["accounting"].(map[string]any)
	if !ok {
		t.Fatalf("missing accounting breakdown: %+v", hr)
	}
	if acc["工资"] != float64(12000) || acc["社保"] != float64(900) || acc["公积金"] != float64(300) {
		t.Fatalf("unexpected accounting breakdown: %+v", acc)
	}
	cash, ok := hr["cash"].(map[string]any)
	if !ok {
		t.Fatalf("missing cash breakdown: %+v", hr)
	}
	if cash["工资"] != float64(7000) || cash["社保"] != float64(500) || cash["公积金"] != float64(200) {
		t.Fatalf("unexpected cash breakdown: %+v", cash)
	}
}
