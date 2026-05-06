package query

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestPrepareQueryExecutionContextReconcilesResolvedEntityIntoQuerySpec(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-query-context.sqlite")
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
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('测试公司','2026-03','2026-03-25','记-0001','6001','主营业务收入','飞未云科3月收入确认','贷',1000,0,1000,'飞未云科(深圳)技术有限公司')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('测试公司','2026-03','2026-03-31','记-0002','6401','主营业务成本','林悦3月成本确认','借',800,800,0,'南京林悦智能科技有限公司')`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('测试公司','2026-03-26','飞未云科(深圳)技术有限公司','3月回款',0,1100)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C-FW-001','飞未云科（深圳）技术有限公司','飞未项目-京东价格数据')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-001','2026-03','contract_fund_income','26年Q1收入明细',900,1100,'是',900)`,
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

	counterpartyCtx := engine.prepareQueryExecutionContext("飞未云科2026年3月收入多少？")
	if counterpartyCtx.entity != "飞未云科（深圳）技术有限公司" {
		t.Fatalf("ctx.entity = %q, want %q", counterpartyCtx.entity, "飞未云科（深圳）技术有限公司")
	}
	if counterpartyCtx.spec.Entity != counterpartyCtx.entity {
		t.Fatalf("spec.entity = %q, want synced with ctx.entity=%q", counterpartyCtx.spec.Entity, counterpartyCtx.entity)
	}
	if counterpartyCtx.spec.QueryFamily != QueryFamilyContractDimension {
		t.Fatalf("spec.query_family = %s, want %s", counterpartyCtx.spec.QueryFamily, QueryFamilyContractDimension)
	}
	if !counterpartyCtx.hasRealEntity {
		t.Fatalf("expected hasRealEntity=true")
	}

	readinessCtx := engine.prepareQueryExecutionContext("南京林悦智能科技有限公司3月数据出来了吗？")
	if readinessCtx.entity != "南京林悦智能科技有限公司" {
		t.Fatalf("readiness ctx.entity = %q, want %q", readinessCtx.entity, "南京林悦智能科技有限公司")
	}
	if readinessCtx.spec.Entity != readinessCtx.entity {
		t.Fatalf("readiness spec.entity = %q, want synced with ctx.entity=%q", readinessCtx.spec.Entity, readinessCtx.entity)
	}
	if readinessCtx.spec.QueryFamily != QueryFamilyReadiness {
		t.Fatalf("readiness query family = %s, want %s", readinessCtx.spec.QueryFamily, QueryFamilyReadiness)
	}
	if !readinessCtx.spec.ReadinessCheckRequired {
		t.Fatalf("expected readiness flag to stay true")
	}
}

func TestShouldResolveEntityDeeplySkipsEntitylessCompanyLevelRoutes(t *testing.T) {
	cases := []struct {
		name string
		spec QuerySpec
		want bool
	}{
		{
			name: "large transaction company level",
			spec: QuerySpec{Intent: IntentLargeTransactionQuery, QueryFamily: QueryFamilyGeneral},
			want: false,
		},
		{
			name: "counterparty question",
			spec: QuerySpec{Intent: IntentFallback, QueryFamily: QueryFamilyCounterparty},
			want: true,
		},
		{
			name: "readiness question",
			spec: QuerySpec{Intent: IntentFallback, QueryFamily: QueryFamilyReadiness},
			want: true,
		},
		{
			name: "contract detail uses document matcher instead of finance entity resolver",
			spec: QuerySpec{Intent: IntentFallback, QueryFamily: QueryFamilyContractDetail, Entity: "商指针产品服务协议"},
			want: false,
		},
		{
			name: "raw entity already present",
			spec: QuerySpec{Intent: IntentMonthlySummary, QueryFamily: QueryFamilyCoreMetric, Entity: "飞未"},
			want: true,
		},
		{
			name: "synthetic large transaction fragment",
			spec: QuerySpec{Intent: IntentLargeTransactionQuery, QueryFamily: QueryFamilyGeneral, Entity: "单笔最大流入来自谁"},
			want: false,
		},
		{
			name: "synthetic tax fragment",
			spec: QuerySpec{Intent: IntentTaxQuery, QueryFamily: QueryFamilyGeneral, Entity: "销项税额是"},
			want: false,
		},
	}

	for _, tc := range cases {
		if got := shouldResolveEntityDeeply(tc.spec); got != tc.want {
			t.Fatalf("%s: shouldResolveEntityDeeply() = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestBuildQuerySpecSkipsSyntheticQuestionFragmentEntities(t *testing.T) {
	anchor := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		question string
		wantFrom string
		wantTo   string
	}{
		{
			question: "2026年3月单笔最大流入来自谁？金额多少？",
			wantFrom: "2026-03",
			wantTo:   "2026-03",
		},
		{
			question: "2026年3月销项税额是多少？2026年3月进项税额是多少？",
			wantFrom: "2026-03",
			wantTo:   "2026-03",
		},
		{
			question: "2026年Q1收入是多少？",
			wantFrom: "2026-01",
			wantTo:   "2026-03",
		},
	}

	for _, tc := range cases {
		spec := BuildQuerySpec(tc.question, anchor)
		if spec.Entity != "" {
			t.Fatalf("BuildQuerySpec(%q).Entity = %q, want empty", tc.question, spec.Entity)
		}
		if spec.PeriodFrom != tc.wantFrom || spec.PeriodTo != tc.wantTo {
			t.Fatalf("BuildQuerySpec(%q) period=%s~%s, want %s~%s", tc.question, spec.PeriodFrom, spec.PeriodTo, tc.wantFrom, tc.wantTo)
		}
	}
}

func TestLooksLikeTemporalMetricEntityAcceptsUppercaseQuarterToken(t *testing.T) {
	if !looksLikeTemporalMetricEntity("Q1") {
		t.Fatalf("looksLikeTemporalMetricEntity(Q1) = false, want true")
	}
}

func TestBuildQuerySpecRoutesQ1AggregateToCoreMetricWithContractPreference(t *testing.T) {
	anchor := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
	spec := BuildQuerySpec("2026年Q1收入是多少？", anchor)

	if spec.Entity != "" {
		t.Fatalf("BuildQuerySpec(Q1 revenue).Entity = %q, want empty", spec.Entity)
	}
	if spec.QueryFamily != QueryFamilyCoreMetric {
		t.Fatalf("BuildQuerySpec(Q1 revenue).QueryFamily = %s, want %s", spec.QueryFamily, QueryFamilyCoreMetric)
	}
	if !spec.PreferContractAggregate {
		t.Fatalf("BuildQuerySpec(Q1 revenue).PreferContractAggregate = false, want true")
	}
}
