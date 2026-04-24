package query

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCollectCounterpartyEvidenceKeepsSiblingRowsAcrossMultipleVoucherContexts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-counterparty-evidence.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
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
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-03','1002','货币资金',100,150)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-02','2026-02-28','V-1','640101','主营业务成本','预提成本','借',1000,1000,0,''),
		 ('测试公司','2026-02','2026-02-28','V-1','220201','应付账款','预提成本_南京林悦智能科技有限公司','贷',1000,0,1000,''),
		 ('测试公司','2026-02','2026-02-27','V-2','640101','主营业务成本','收到南京林悦智能科技有限公司发票','借',2000,2000,0,''),
		 ('测试公司','2026-02','2026-02-27','V-2','220201','应付账款','收到南京林悦智能科技有限公司发票','贷',2000,0,2000,'南京林悦智能科技有限公司')`,
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

	evidence := engine.collectCounterpartyEvidence("南京林悦智能科技有限公司", "2026-02", "2026-02")
	if len(evidence) != 4 {
		t.Fatalf("evidence rows = %d, want 4 (%+v)", len(evidence), evidence)
	}

	totalCost := 0.0
	for _, ev := range evidence {
		if ev.AccountCode == "640101" && (ev.Direction == "借" || ev.DebitAmount > 0) {
			totalCost += ev.DebitAmount
		}
	}
	if totalCost != 3000 {
		t.Fatalf("total debit cost = %.2f, want 3000", totalCost)
	}
}

func TestResolveCounterpartyCandidatesExpandsShortAliasToCanonicalNames(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-counterparty-alias.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
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
		`CREATE TABLE fin_contracts (customer_name TEXT, contract_content TEXT)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-03','1002','货币资金',100,150)`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary)
		 VALUES ('测试公司','2026-03-06',1000,0,'飞未云科(深圳)技术有限公司','结算款')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('测试公司','2026-03','2026-03-31','V-FW-1','600101','技术服务费','确认收入','贷',1000,0,1000,'飞未云科(深圳)技术有限公司')`,
		`INSERT INTO fin_contracts(customer_name) VALUES ('飞未云科(深圳)技术有限公司')`,
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

	candidates := engine.resolveCounterpartyCandidates("飞未云科")
	if len(candidates) == 0 {
		t.Fatalf("expected candidates for alias 飞未云科")
	}
	if got := candidates[0]; got != "飞未云科(深圳)技术有限公司" {
		t.Fatalf("best candidate = %q, want %q", got, "飞未云科(深圳)技术有限公司")
	}
	if got := engine.extractNamedEntity("飞未云科这个主体目前更像客户、供应商还是混合往来？"); got != "飞未云科(深圳)技术有限公司" {
		t.Fatalf("extractNamedEntity() = %q, want %q", got, "飞未云科(深圳)技术有限公司")
	}
}

func TestExtractNamedEntityPrefersAliasCandidatesBeforeDBFallback(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-short-alias.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
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
		`CREATE TABLE fin_contracts (customer_name TEXT, contract_content TEXT)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-02','1002','货币资金',0,0)`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary)
		 VALUES ('测试公司','2026-02-13',0,53750,'南京汇智互娱教育科技有限公司','供应商付款')`,
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

	if got := engine.extractNamedEntity("汇智在2026年2月这笔是供应商付款还是预收款？"); got != "南京汇智互娱教育科技有限公司" {
		t.Fatalf("extractNamedEntity() = %q, want %q", got, "南京汇智互娱教育科技有限公司")
	}
}

func TestExactCounterpartyEvidenceMapGroupsEvidenceByCounterparty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-supplier-evidence-map.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
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
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-03','1002','货币资金',0,0)`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary) VALUES
		 ('测试公司','2026-03-05',0,1000,'供应商A有限公司','技术服务费'),
		 ('测试公司','2026-03-06',0,500,'北京市中闻（南京）律师事务所','法律服务费')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-05','V-SUP-1','640101','主营业务成本','供应商A成本确认','借',1000,1000,0,'供应商A有限公司'),
		 ('测试公司','2026-03','2026-03-05','V-SUP-1','220201','应付账款','供应商A成本确认','贷',1000,0,1000,'供应商A有限公司')`,
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

	grouped := engine.collectExactCounterpartyEvidenceMap([]string{"供应商A有限公司", "北京市中闻（南京）律师事务所"}, "2026-03-01", "2026-03-31")
	if len(grouped["供应商A有限公司"]) != 3 {
		t.Fatalf("supplier evidence count = %d, want 3", len(grouped["供应商A有限公司"]))
	}
	if len(grouped["北京市中闻（南京）律师事务所"]) != 1 {
		t.Fatalf("law firm evidence count = %d, want 1", len(grouped["北京市中闻（南京）律师事务所"]))
	}
}
