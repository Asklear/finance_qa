package query

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMonthlySummaryYTDFallbackUsesRequestedYear(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-ytd.sqlite")
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
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2027-02','1002','货币资金',100,100)`,
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

	res := engine.Query("2027年2月经营状况")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "2027年1月以来（YTD）累计") {
		t.Fatalf("message should use requested year, got: %s", res.Message)
	}
	if strings.Contains(res.Message, "2026年1月以来（YTD）累计") {
		t.Fatalf("message should not use hardcoded year, got: %s", res.Message)
	}
}

func TestFallbackHintUsesGenericPlaceholders(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-hint.sqlite")
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
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-03','1002','货币资金',100,150)`,
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

	res := engine.Query("帮我随便看一下")
	if res.Success {
		t.Fatalf("expected fallback result, got success with message: %s", res.Message)
	}
	hint, _ := res.Data["hint"].(string)
	if hint == "" {
		t.Fatalf("expected non-empty hint")
	}
	if strings.Contains(hint, "2026") || strings.Contains(hint, "飞未云科") {
		t.Fatalf("hint should be generic instead of hardcoded example, got: %s", hint)
	}
}

func TestMonthEndDayAcceptsFullDateInput(t *testing.T) {
	if got := monthEndDay("2027-02-15"); got != "2027-02-15" {
		t.Fatalf("monthEndDay(full-date) = %q, want %q", got, "2027-02-15")
	}
}

func TestStripTemporalNoiseRemovesAnyYearMonthDayTokens(t *testing.T) {
	got := stripTemporalNoise("2027年金程3月26日")
	if got != "金程" {
		t.Fatalf("stripTemporalNoise() = %q, want %q", got, "金程")
	}
}

func TestIntervalCoreMetricQuestionRoutesToRangeSummary(t *testing.T) {
	cases := []struct {
		question string
		from     string
		to       string
	}{
		{question: "2026年上半年营收", from: "2026-01", to: "2026-06"},
		{question: "2026年全年营收", from: "2026-01", to: "2026-12"},
		{question: "2026年第一季度收入", from: "2026-01", to: "2026-03"},
		{question: "2026年累计利润", from: "2026-01", to: "2026-04"},
	}

	for _, tc := range cases {
		if !isIntervalCoreMetricQuestion(tc.question, "", false, tc.from, tc.to) {
			t.Fatalf("isIntervalCoreMetricQuestion(%q) = false, want true", tc.question)
		}
	}
	if isIntervalCoreMetricQuestion("飞未云科2026年累计销售额多少", "飞未云科", true, "2026-01", "2026-04") {
		t.Fatalf("counterparty cumulative question should not be treated as company range summary")
	}
}

func TestIntervalCoreMetricQuestionClampsToAvailablePeriods(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hardening-range-clamp.sqlite")
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
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-03','1002','货币资金',100,150)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES
		 ('测试公司','2026-01','营业收入',100,100),
		 ('测试公司','2026-01','净利润',20,20),
		 ('测试公司','2026-02','营业收入',200,300),
		 ('测试公司','2026-02','净利润',50,70),
		 ('测试公司','2026-03','营业收入',300,600),
		 ('测试公司','2026-03','净利润',80,150)`,
		`INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, counterparty_name, summary)
		 VALUES ('测试公司','2026-03-31',600,450,'客户A','3月回款')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty) VALUES
		 ('测试公司','2026-03','2026-03-31','记-0001','600101','技术服务费','确认3月收入','贷',300,0,300,'客户A'),
		 ('测试公司','2026-03','2026-03-31','记-0001','640101','信息服务费','确认3月成本','借',220,220,0,'供应商A')`,
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

	res := engine.Query("2026年上半年营收")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	coverage, _ := res.Data["coverage"].(map[string]any)
	if coverage == nil {
		t.Fatalf("expected coverage metadata, got: %+v", res.Data)
	}
	if got := coverage["actual_to"]; got != "2026-03" {
		t.Fatalf("coverage.actual_to = %v, want 2026-03", got)
	}
	if got := coverage["requested_to"]; got != "2026-06" {
		t.Fatalf("coverage.requested_to = %v, want 2026-06", got)
	}
	if !strings.Contains(res.Message, "当前账务数据仅到 2026-03") {
		t.Fatalf("message should disclose available cutoff, got: %s", res.Message)
	}
	if got := res.Data["period"]; got != "2026-01~2026-03" {
		t.Fatalf("period = %v, want 2026-01~2026-03", got)
	}
}

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
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance)
		 VALUES ('测试公司','2026-03','1002','货币资金',100,150)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('测试公司','2026-03','2026-03-25','记-0001','6001','主营业务收入','飞未云科3月收入确认','贷',1000,0,1000,'飞未云科(深圳)技术有限公司')`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
		 VALUES ('测试公司','2026-03','2026-03-31','记-0002','6401','主营业务成本','林悦3月成本确认','借',800,800,0,'南京林悦智能科技有限公司')`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('测试公司','2026-03-26','飞未云科(深圳)技术有限公司','3月回款',0,1100)`,
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
	if counterpartyCtx.entity != "飞未云科(深圳)技术有限公司" {
		t.Fatalf("ctx.entity = %q, want %q", counterpartyCtx.entity, "飞未云科(深圳)技术有限公司")
	}
	if counterpartyCtx.spec.Entity != counterpartyCtx.entity {
		t.Fatalf("spec.entity = %q, want synced with ctx.entity=%q", counterpartyCtx.spec.Entity, counterpartyCtx.entity)
	}
	if counterpartyCtx.spec.QueryFamily != QueryFamilyCounterparty {
		t.Fatalf("spec.query_family = %s, want %s", counterpartyCtx.spec.QueryFamily, QueryFamilyCounterparty)
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
