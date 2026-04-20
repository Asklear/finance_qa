package query_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

const testCompany = "南京优集数据科技有限公司"

func TestQueryMonthlyKPIShouldNotBeMisroutedAsMetricEntity(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年1月收入/成本多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if strings.Contains(res.Message, "[收入]") || strings.Contains(res.Message, "[成本]") {
		t.Fatalf("monthly KPI question was misrouted as entity query: %s", res.Message)
	}
	if entity, ok := res.Data["entity"].(string); ok && (entity == "收入" || entity == "成本") {
		t.Fatalf("unexpected metric entity in response: %q", entity)
	}
}

func TestQueryRealEntityQuestionStillUsesCounterpartyPath(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未2月收入多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "[飞未]") {
		t.Fatalf("expected counterparty style response, got: %s", res.Message)
	}
}

func TestEntityYearCumulativeUsesYearRangeInsteadOfSingleMonth(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未云科2026年累计销售额多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	amount, ok := res.Data["amount"].(float64)
	if !ok {
		t.Fatalf("missing amount in response: %+v", res.Data)
	}
	// test db has 2026-01 revenue=1200 and 2026-02 revenue=800 for 飞未云科
	if amount != 2000 {
		t.Fatalf("want cumulative amount=2000, got=%v message=%s", amount, res.Message)
	}
	if !strings.Contains(res.Message, "2026-01~2026-03") {
		t.Fatalf("expected period range in message, got: %s", res.Message)
	}
}

func TestProfitQuestionUsesSingleAccrualAnswer(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月利润是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if strings.Contains(res.Message, "银行卡上看") || strings.Contains(res.Message, "账上看") {
		t.Fatalf("profit answer should use single accrual wording, got: %s", res.Message)
	}
	monthly, ok := res.Data["monthly"].(map[string]any)
	if !ok {
		t.Fatalf("missing monthly payload: %+v", res.Data)
	}
	if monthly["profit"] != float64(500) {
		t.Fatalf("profit = %v, want 500", monthly["profit"])
	}
}

func TestAmbiguousPreShouKuanRoutesToARAPWithoutFallback(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("汇智在2026年2月这笔是供应商付款还是预收款？")
	if !res.Success {
		t.Fatalf("want success, got=%s", res.Message)
	}
	if res.AnswerMethod != "sql" {
		t.Fatalf("want sql, got=%s", res.AnswerMethod)
	}
	if strings.Contains(res.Message, "语义模糊") {
		t.Fatalf("fallback leaked: %s", res.Message)
	}
	if !strings.Contains(res.Message, "供应商") {
		t.Fatalf("expect supplier wording, got: %s", res.Message)
	}
	tr, ok := res.Data["intent_trace"].(map[string]any)
	if !ok {
		t.Fatalf("intent_trace missing")
	}
	for _, k := range []string{"router_version", "matched", "scores", "final_intent", "confidence"} {
		if _, ok := tr[k]; !ok {
			t.Fatalf("intent_trace missing key=%s", k)
		}
	}
}

func TestHRBreakdownQuestionShouldNotRouteToCounterpartyEntity(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月人力成本多少？工资、社保、公积金分别是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if strings.Contains(res.Message, "[公积金]") {
		t.Fatalf("should not route to counterparty fallback: %s", res.Message)
	}
	hr, ok := res.Data["hr_breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("missing hr_breakdown: %+v", res.Data)
	}
	acc, ok := hr["accounting"].(map[string]any)
	if !ok {
		t.Fatalf("missing accounting breakdown: %+v", hr)
	}
	if acc["工资"] != float64(30000) || acc["社保"] != float64(3000) || acc["公积金"] != float64(600) {
		t.Fatalf("unexpected accounting breakdown: %+v", acc)
	}
	cash, ok := hr["cash"].(map[string]any)
	if !ok {
		t.Fatalf("missing cash breakdown: %+v", hr)
	}
	if cash["工资"] != float64(15000) || cash["社保"] != float64(2500) || cash["公积金"] != float64(600) {
		t.Fatalf("unexpected cash breakdown: %+v", cash)
	}
}

func TestEngineUsesConfigurableMetricKeywords(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "metric_keywords": {
      "profit": ["净赚"]
    }
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月净赚是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if strings.Contains(res.Message, "银行卡上看") || strings.Contains(res.Message, "账上看") {
		t.Fatalf("custom profit metric keyword should still use single accrual answer, got: %s", res.Message)
	}
	monthly, ok := res.Data["monthly"].(map[string]any)
	if !ok {
		t.Fatalf("missing monthly payload: %+v", res.Data)
	}
	if monthly["profit"] != float64(500) {
		t.Fatalf("profit = %v, want 500", monthly["profit"])
	}
}

func TestEngineUsesConfigurableHRBreakdownKeywords(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "hr_breakdown_keywords": ["细拆"]
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月人力成本细拆")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if _, ok := res.Data["hr_breakdown"].(map[string]any); !ok {
		t.Fatalf("custom hr breakdown keyword should route to breakdown payload, got: %+v", res.Data)
	}
}

func TestEngineUsesConfigurableCounterpartyClassificationKeywords(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "counterparty_classification_question_keywords": ["归类判断"]
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-LINYUE-CFG-1', '640101', '主营业务成本', '借', 1440000, '预提成本', '', 1440000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-LINYUE-CFG-1', '220201', '应付账款', '贷', 1440000, '预提成本_南京林悦智能科技有限公司_2026.02.28', '', 0, 1440000)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert counterparty config data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("南京林悦智能科技有限公司在2026年2月这笔归类判断")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "供应商") {
		t.Fatalf("custom counterparty classification keyword should use role judgement, got: %s", res.Message)
	}
}

func TestEngineUsesConfigurableProfitSingleViewBlockKeywords(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "metric_keywords": {
      "profit": ["净赚"]
    },
    "profit_single_view_block_keywords": ["不一致"]
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月净赚为什么不一致？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "银行卡") || !strings.Contains(res.Message, "账") {
		t.Fatalf("custom profit single-view block keyword should force dual perspective, got: %s", res.Message)
	}
}

func TestCounterpartyCostShouldIncludeVoucherSiblingRows(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-LINYUE-1', '640101', '信息服务费', '借', 1440000, '预提成本', '', 1440000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-LINYUE-1', '220201', '单位', '贷', 1440000, '预提成本_南京林悦智能科技有限公司_2026.02.28', '', 0, 1440000)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert voucher sibling data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("南京林悦智能科技有限公司2月成本多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "供应商相关") {
		t.Fatalf("expected supplier wording, got: %s", res.Message)
	}
	if got := res.Data["amount"]; got != float64(1440000) {
		t.Fatalf("amount = %v, want 1440000", got)
	}
}

func TestInternalPartyUsesConfigurableOrgSuffixesAndContextKeywords(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "hr_breakdown_keywords": ["细拆"]
  },
  "internal_party": {
    "org_suffixes": ["中心"],
    "account_context_keywords": ["内部代发"]
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-18', 'V-INTERNAL-CFG-1', '224101', '其他应付款', '借', 8000, '内部代发薪酬', '华东中心', 8000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-18', 'V-INTERNAL-CFG-1', '100201', '招商银行', '贷', 8000, '支付华东中心代发薪酬', '华东中心', 0, 8000)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert configurable internal party data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月人力成本细拆")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	hr, ok := res.Data["hr_breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("missing hr_breakdown: %+v", res.Data)
	}
	cash, ok := hr["cash"].(map[string]any)
	if !ok {
		t.Fatalf("missing cash breakdown: %+v", hr)
	}
	if cash["分公司内部转账"] != float64(8000) {
		t.Fatalf("configurable internal party detection failed, got: %+v", cash)
	}
}

func TestCounterpartyClassificationQuestionPrefersRoleJudgement(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-LINYUE-CLASS-1', '640101', '主营业务成本', '借', 1440000, '预提成本', '', 1440000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-LINYUE-CLASS-1', '220201', '应付账款', '贷', 1440000, '预提成本_南京林悦智能科技有限公司_2026.02.28', '', 0, 1440000)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert classification voucher failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("林悦在2026年2月这笔是成本还是收入？请给判断依据。")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "供应商/成本侧") {
		t.Fatalf("expected supplier classification, got: %s", res.Message)
	}
}

func TestHRBreakdownListsBranchTransferSeparatelyWhenVoucherHasPayrollLiability(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-18', 'V-BRANCH-WAGE-1', '221101', '应付职工薪酬-工资', '借', 8000, '支付分公司代发薪酬', '上海分公司', 8000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-18', 'V-BRANCH-WAGE-1', '100201', '招商银行', '贷', 8000, '上海分公司转账', '上海分公司', 0, 8000)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert branch wage voucher failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
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
	cash, ok := hr["cash"].(map[string]any)
	if !ok {
		t.Fatalf("missing cash breakdown: %+v", hr)
	}
	if cash["工资"] != float64(15000) {
		t.Fatalf("cash wage = %v, want 15000", cash["工资"])
	}
	if cash["分公司内部转账"] != float64(8000) {
		t.Fatalf("branch transfer = %v, want 8000", cash["分公司内部转账"])
	}
}

func TestHRBreakdownDetectsInternalTransferWithoutHardcodedBranchName(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-19', 'V-INTERNAL-TRANSFER-1', '122101', '单位', '借', 9000, '转账南京优集杭州分公司_南京优集杭州分公司_2026.03.19', '', 9000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-19', 'V-INTERNAL-TRANSFER-1', '100201', '招商银行', '贷', 9000, '转账南京优集杭州分公司', '', 0, 9000)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert internal transfer voucher failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
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
	cash, ok := hr["cash"].(map[string]any)
	if !ok {
		t.Fatalf("missing cash breakdown: %+v", hr)
	}
	if cash["工资"] != float64(15000) {
		t.Fatalf("cash wage = %v, want 15000", cash["工资"])
	}
	if cash["分公司内部转账"] != float64(9000) {
		t.Fatalf("branch transfer = %v, want 9000", cash["分公司内部转账"])
	}
}

func TestEntityARAPUsesConfidenceAwareWordingForSummaryDerivedMatches(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-28', 'V-JC-OPEN', '112201', '单位', '借', 1000, '为辽宁金程信息科技有限公司服务_辽宁金程信息科技有限公司_2026.02.28', '', 1000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-06', 'V-JC-SETTLE', '112201', '单位', '贷', 1000, '辽宁金程信息科技有限公司转账_辽宁金程信息科技有限公司_2026.03.06', '', 0, 1000)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-25', 'V-JC-NEW', '112201', '单位', '借', 600, '为辽宁金程信息科技有限公司服务_辽宁金程信息科技有限公司_2026.03.25', '', 600, 0)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert summary-derived entity arap data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("辽宁金程信息科技有限公司的应收/应付是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "按开放项推断") {
		t.Fatalf("message should disclose inferred open-item basis, got: %s", res.Message)
	}
}

func TestEntityARAPQuestionUsesCounterpartyOpenItems(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-20', 'V-LINYUE-AP-1', '220201', '应付账款', '贷', 494000, '收到南京林悦智能科技有限公司发票', '', 0, 494000)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-20', 'V-LINYUE-AP-1', '640101', '主营业务成本', '借', 494000, '收到南京林悦智能科技有限公司发票', '', 494000, 0)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert entity ap voucher failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("南京林悦智能科技有限公司的应收/应付是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["receivable_total"]; got != float64(0) {
		t.Fatalf("receivable_total = %v, want 0", got)
	}
	if got := res.Data["payable_total"]; got != float64(494000) {
		t.Fatalf("payable_total = %v, want 494000", got)
	}
	if !strings.Contains(res.Message, "应收 0.00 元") || !strings.Contains(res.Message, "应付 494000.00 元") {
		t.Fatalf("unexpected entity AR/AP message: %s", res.Message)
	}
}

func TestCounterpartyReceiptsQuestionIncludesCurrentMonthBreakdown(t *testing.T) {
	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-15', '辽宁金程信息科技有限公司', '1月回款', 0, 5000)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-06', '辽宁金程信息科技有限公司', '3月回款', 0, 2100)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert receipt data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("金程今年回款多少？其中3月到账多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "今年") || !strings.Contains(res.Message, "其中3月") {
		t.Fatalf("message should disclose cumulative and month breakdown, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "7100.00") {
		t.Fatalf("message should include cumulative receipt amount, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "2100.00") {
		t.Fatalf("message should include month receipt amount, got: %s", res.Message)
	}
}

func buildEntityRoutingTestDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "entity-routing.db")
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
			account_name TEXT,
			account_code TEXT,
			opening_balance REAL,
			closing_balance REAL
		)`,
		`CREATE TABLE income_statement (
			company TEXT,
			period TEXT,
			item_name TEXT,
			current_amount REAL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	inserts := []string{
		`INSERT INTO balance_sheet(company, period, account_name, account_code, opening_balance, closing_balance)
		 VALUES ('南京优集数据科技有限公司', '2026-01', '货币资金', '1002', 1000, 2000)`,
		`INSERT INTO balance_sheet(company, period, account_name, account_code, opening_balance, closing_balance)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '货币资金', '1002', 2000, 2600)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-15', '6001', '主营业务收入', '贷', 1200, '1月飞未收入确认', '飞未云科', 0, 1200)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-20', '6401', '主营业务成本', '借', 400, '1月项目成本', '飞未云科', 400, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-10', '6001', '主营业务收入', '贷', 800, '2月飞未收入确认', '飞未云科', 0, 800)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01', '一、营业收入', 1200)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01', '五、净利润', 800)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '一、营业收入', 800)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '五、净利润', 500)`,
		`INSERT INTO balance_sheet(company, period, account_name, account_code, opening_balance, closing_balance)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '应付职工薪酬', '2211', 0, 300)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-15', '飞未云科', '2月回款', 0, 904)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-31', '66020101', '工资', '借', 10000, '计提3月工资', '', 10000, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-31', '66022301', '工资', '借', 20000, '计提3月工资', '', 20000, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-31', '66020102', '社保', '借', 1000, '3月社保扣款', '', 1000, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-31', '66022302', '社保', '借', 2000, '3月社保扣款', '', 2000, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-31', '66020103', '公积金', '借', 200, '3月公积金', '', 200, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-31', '66022303', '公积金', '借', 400, '3月公积金', '', 400, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-10', '100201', '招商银行', '贷', 15000, '发放2月工资', '', 0, 15000)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-25', '100201', '招商银行', '贷', 2500, '3月社保扣款', '', 0, 2500)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-26', '100201', '招商银行', '贷', 600, '3月公积金', '', 0, 600)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-13', '南京汇智互娱教育科技有限公司', '供应商付款', 53750, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-25', '南京汇智互娱教育科技有限公司', '供应商付款', 53750, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-25', '640101', '主营业务成本', '借', 95000, '汇智成本确认', '南京汇智互娱教育科技有限公司', 95000, 0)`,
		`INSERT INTO journal(company, voucher_date, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-25', '22210101', '进项税额', '借', 12500, '汇智进项税', '南京汇智互娱教育科技有限公司', 12500, 0)`,
	}
	for _, stmt := range inserts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert seed data failed: %v", err)
		}
	}

	return dbPath
}
