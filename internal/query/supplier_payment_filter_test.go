package query

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestShouldIncludeSupplierPaymentCounterpartyExcludesConfiguredPseudoNames(t *testing.T) {
	engine := &Engine{Company: "南京优集数据科技有限公司"}

	cases := []struct {
		name string
		role CounterpartyRole
	}{
		{name: "暂收款", role: CounterpartyMixed},
		{name: "网上电子汇划收入", role: CounterpartyMixed},
		{name: "对公中间业务收入-网上其他收入", role: CounterpartySupplier},
	}

	for _, tc := range cases {
		include, reason := engine.shouldIncludeSupplierPaymentCounterparty(tc.name, CounterpartyClassification{Role: tc.role})
		if include {
			t.Fatalf("%s should be excluded from supplier payments, got include=true", tc.name)
		}
		if reason != "non_counterparty_flow" {
			t.Fatalf("%s should use non_counterparty_flow reason, got %s", tc.name, reason)
		}
	}
}

func TestShouldIncludeSupplierPaymentCounterpartyDoesNotFallbackOnOrgNameOnly(t *testing.T) {
	engine := &Engine{Company: "南京优集数据科技有限公司"}

	include, reason := engine.shouldIncludeSupplierPaymentCounterparty(
		"某外部机构有限公司",
		CounterpartyClassification{Role: CounterpartyUnknown},
	)
	if include {
		t.Fatal("organization-name-only counterparty should not be included without supplier evidence")
	}
	if reason != "unknown_organization_without_evidence" {
		t.Fatalf("reason = %s, want unknown_organization_without_evidence", reason)
	}
}

func TestCollectSupplierPaymentSummaryBackfillsSupplierEvidenceFromJournalSummary(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "supplier-summary-like-evidence.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE bank_statement (
			company TEXT,
			transaction_date TEXT,
			counterparty_name TEXT,
			summary TEXT,
			debit_amount REAL,
			credit_amount REAL
		)`,
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
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-31', '云南泽塔数据科技有限公司', '转账', 489172.75, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', '记-0054', '112301', '单位', '收到云南泽塔数据科技有限公司发票_云南泽塔数据科技有限公司_2026.03.31', '贷', 489172.75, 0, 489172.75, NULL)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', '记-0054', '22210101', '进项税额', '收到云南泽塔数据科技有限公司发票', '借', 27689.02, 27689.02, 0, NULL)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', '记-0054', '640102', '技术服务费', '收到云南泽塔数据科技有限公司发票', '借', 461483.73, 461483.73, 0, NULL)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	summary, err := engine.collectSupplierPaymentSummary("2026-03", "2026-03")
	if err != nil {
		t.Fatalf("collectSupplierPaymentSummary error = %v", err)
	}

	if len(summary.Suppliers) != 1 {
		t.Fatalf("supplier rows = %d, want 1 (%+v)", len(summary.Suppliers), summary.Suppliers)
	}
	if got := summary.Suppliers[0]["name"]; got != "云南泽塔数据科技有限公司" {
		t.Fatalf("supplier name = %v, want 云南泽塔数据科技有限公司", got)
	}
	if got := summary.Suppliers[0]["role"]; got != "supplier" {
		t.Fatalf("supplier role = %v, want supplier", got)
	}
	if summary.Total != 489172.75 {
		t.Fatalf("total = %.2f, want 489172.75", summary.Total)
	}
}
