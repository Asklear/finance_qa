package query

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestResolveEntityByScoredCandidatesRejectsQuestionModifier(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	if got := engine.resolveEntityByScoredCandidates("当前的应收账款汇总"); got != "" {
		t.Fatalf("resolveEntityByScoredCandidates() = %q, want empty", got)
	}
}

func TestResolveEntityByScoredCandidatesMatchesKnownAlias(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	if got := engine.resolveEntityByScoredCandidates("飞未云科这个主体目前更像客户、供应商还是混合往来？"); got != "飞未云科（深圳）技术有限公司" {
		t.Fatalf("resolveEntityByScoredCandidates() = %q, want 飞未云科（深圳）技术有限公司", got)
	}
}

func TestResolveEntityByScoredCandidatesUsesSummaryDerivedCompanyNames(t *testing.T) {
	dbPath := buildEntityRoutingTestDBForScoredResolver(t)
	engine, err := NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	if got := engine.resolveEntityByScoredCandidates("金程今年银行流水回款多少？其中3月到账多少？"); got != "辽宁金程信息科技有限公司" {
		t.Fatalf("resolveEntityByScoredCandidates() = %q, want 辽宁金程信息科技有限公司", got)
	}
}

func buildEntityRoutingTestDBForScoredResolver(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "entity-resolver-scored.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, counterparty_name TEXT, summary TEXT, debit_amount REAL, credit_amount REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, direction TEXT, amount REAL, summary TEXT, counterparty TEXT, debit_amount REAL, credit_amount REAL)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-25', 'V-JC-NEW', '112201', '单位', '借', 600, '为辽宁金程信息科技有限公司服务_辽宁金程信息科技有限公司_2026.03.25', '', 600, 0)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}
	return dbPath
}
