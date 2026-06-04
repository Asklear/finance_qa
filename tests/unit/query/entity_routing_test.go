package query_test

import (
	"database/sql"
	"strings"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

const testCompany = "南京优集数据科技有限公司"

func seedEntityRoutingSQL(t *testing.T, dbPath, label string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s failed: %v", label, err)
		}
	}
}

func TestEntityRoutingCoreQueryScenarios(t *testing.T) {
	runParallelQueryScenarios(t, []queryScenario{
		{
			Name:     "monthly_kpi_not_misrouted_as_metric_entity",
			Question: "2026年1月收入/成本多少",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if strings.Contains(res.Message, "[收入]") || strings.Contains(res.Message, "[成本]") {
					t.Fatalf("monthly KPI question was misrouted as entity query: %s", res.Message)
				}
			},
		},
		{
			Name:     "real_entity_prefers_contract_dimension",
			Question: "飞未2月收入多少",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if !strings.Contains(res.Message, "先看现金口径") || !strings.Contains(res.Message, "再看财务口径") {
					t.Fatalf("expected contract-dimension dual view response, got: %s", res.Message)
				}
			},
		},
		{
			Name:     "march_entity_revenue_prefers_contract_dimension",
			Question: "飞未云科2026年3月收入多少？",
			DBPath:   buildEntityRoutingTestDB,
			Seed: func(t *testing.T, dbPath string) {
				seedEntityRoutingSQL(t, dbPath, "insert march seed data",
					`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
					 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-25', '记-0099', '6001', '主营业务收入', '贷', 900, '3月飞未收入确认', '飞未云科', 0, 900)`,
					`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
					 VALUES ('南京优集数据科技有限公司', '2026-03-26', '飞未云科', '3月回款', 0, 900)`,
				)
			},
			Assert: func(t *testing.T, res query.Result) {
				if strings.Contains(res.Message, "月度经营分析") {
					t.Fatalf("entity revenue question should not route to monthly summary: %s", res.Message)
				}
				if strings.Contains(res.Message, "语义模糊") {
					t.Fatalf("entity revenue question should not fall back to ambiguity: %s", res.Message)
				}
				if !strings.Contains(res.Message, "合同台账结算") {
					t.Fatalf("expected contract-dimension response, got: %s", res.Message)
				}
				spec, ok := res.Data["query_spec"].(map[string]any)
				if !ok {
					t.Fatalf("query_spec missing: %+v", res.Data)
				}
				if got := spec["entity"]; got != "飞未云科（深圳）技术有限公司" {
					t.Fatalf("query_spec.entity = %v, want 飞未云科（深圳）技术有限公司", got)
				}
			},
		},
		{
			Name:     "readiness_keeps_resolved_entity",
			Question: "南京林悦智能科技有限公司3月数据出来了吗？",
			DBPath:   buildEntityRoutingTestDB,
			Seed: func(t *testing.T, dbPath string) {
				seedEntityRoutingSQL(t, dbPath, "insert readiness seed data",
					`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
					 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-31', '记-0100', '6401', '主营业务成本', '林悦3月成本确认', '借', 500, '南京林悦智能科技有限公司', 500, 0)`,
					`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
					 VALUES ('南京优集数据科技有限公司', '2026-03-20', '南京林悦智能科技有限公司', '合同款', 500, 0)`,
				)
			},
			Assert: func(t *testing.T, res query.Result) {
				if strings.Contains(res.Message, "语义模糊") {
					t.Fatalf("readiness question should not fall back to ambiguity: %s", res.Message)
				}
				if !strings.Contains(res.Message, "南京林悦智能科技有限公司") || !strings.Contains(res.Message, "2026-03") {
					t.Fatalf("expected resolved entity and period in readiness answer, got: %s", res.Message)
				}
			},
		},
		{
			Name:     "year_cumulative_uses_year_range",
			Question: "飞未云科2026年累计销售额多少",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
				accountView, ok := res.Data["account_view"].(map[string]any)
				if !ok {
					t.Fatalf("missing account_view in response: %+v", res.Data)
				}
				if got := accountView["settlement_amount"]; got != float64(2900) {
					t.Fatalf("settlement_amount = %v, want 2900", got)
				}
				if !strings.Contains(res.Message, "2026-01~2026-03") {
					t.Fatalf("expected period range in message, got: %s", res.Message)
				}
			},
		},
		{
			Name:     "profit_uses_cash_first_dual_answer",
			Question: "2026年2月账上利润是多少？",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if !strings.Contains(res.Message, "现金口径") || !strings.Contains(res.Message, "经营口径") {
					t.Fatalf("profit answer should expose cash and operating views, got: %s", res.Message)
				}
				if strings.Index(res.Message, "现金口径") > strings.Index(res.Message, "经营口径") {
					t.Fatalf("profit answer should present cash view before operating view, got: %s", res.Message)
				}
				monthly, ok := res.Data["monthly"].(map[string]any)
				if !ok {
					t.Fatalf("missing monthly payload: %+v", res.Data)
				}
				if monthly["profit"] != float64(500) {
					t.Fatalf("profit = %v, want 500", monthly["profit"])
				}
				if got, ok := res.Data["money_value"].(float64); !ok || got != -106596 {
					t.Fatalf("money_value = %v, want -106596", res.Data["money_value"])
				}
				if got, ok := res.Data["account_value"].(float64); !ok || got != 500 {
					t.Fatalf("account_value = %v, want 500", res.Data["account_value"])
				}
				if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
					t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
				}
			},
		},
		{
			Name:     "revenue_uses_cash_first_dual_answer_without_contract_rows",
			Question: "2026年2月收入是多少？",
			DBPath:   buildEntityRoutingTestDB,
			Seed: func(t *testing.T, dbPath string) {
				seedEntityRoutingSQL(t, dbPath, "delete contract seed data", `DELETE FROM fin_fund_income`, `DELETE FROM fin_contracts`)
			},
			Assert: func(t *testing.T, res query.Result) {
				if !strings.Contains(res.Message, "现金口径") || !strings.Contains(res.Message, "经营口径") {
					t.Fatalf("revenue answer should expose cash and operating views, got: %s", res.Message)
				}
				if strings.Index(res.Message, "现金口径") > strings.Index(res.Message, "经营口径") {
					t.Fatalf("revenue answer should present cash view before operating view, got: %s", res.Message)
				}
				if got, ok := res.Data["money_value"].(float64); !ok || got != 904 {
					t.Fatalf("money_value = %v, want 904", res.Data["money_value"])
				}
				if got, ok := res.Data["account_value"].(float64); !ok || got != 800 {
					t.Fatalf("account_value = %v, want 800", res.Data["account_value"])
				}
			},
		},
		{
			Name:     "injects_intent_trace_query_spec_and_process_envelope",
			Question: "2026年2月收入是多少？",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
				intentTrace, ok := res.Data["intent_trace"].(map[string]any)
				if !ok {
					t.Fatalf("intent_trace missing: %+v", res.Data)
				}
				for _, key := range []string{"router_version", "matched", "scores", "final_intent", "confidence"} {
					if _, exists := intentTrace[key]; !exists {
						t.Fatalf("intent_trace missing key=%s", key)
					}
				}

				querySpec, ok := res.Data["query_spec"].(map[string]any)
				if !ok {
					t.Fatalf("query_spec missing: %+v", res.Data)
				}
				if got := querySpec["query_family"]; got == nil || got.(query.QueryFamily) != query.QueryFamilyCoreMetric {
					t.Fatalf("query_spec.query_family = %v, want core_metric", got)
				}
				if got := querySpec["perspective_policy"]; got == nil || got.(query.PerspectivePolicy) != query.PerspectiveCashThenAccrual {
					t.Fatalf("query_spec.perspective_policy = %v, want cash_then_accrual", got)
				}
				if got := querySpec["period_from"]; got != "2026-02" {
					t.Fatalf("query_spec.period_from = %v, want 2026-02", got)
				}
				if got := querySpec["period_to"]; got != "2026-02" {
					t.Fatalf("query_spec.period_to = %v, want 2026-02", got)
				}

				trace, ok := res.Data["trace"].(map[string]any)
				if !ok {
					t.Fatalf("trace missing: %+v", res.Data)
				}
				process, ok := res.Data["process"].(map[string]any)
				if !ok {
					t.Fatalf("process missing: %+v", res.Data)
				}
				if trace["answer_method"] != "sql" || process["answer_method"] != "sql" {
					t.Fatalf("trace/process answer_method not injected correctly: trace=%v process=%v", trace["answer_method"], process["answer_method"])
				}
				if _, ok := res.Data["executed_sql"].([]string); !ok {
					t.Fatalf("data.executed_sql missing or wrong type: %#v", res.Data["executed_sql"])
				}
				if _, ok := res.Data["calculation_logs"].([]string); !ok {
					t.Fatalf("data.calculation_logs missing or wrong type: %#v", res.Data["calculation_logs"])
				}
			},
		},
		{
			Name:     "multi_metric_uses_cash_first_dual_answer",
			Question: "2026年2月账上收入、成本、利润分别是多少？",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if !strings.Contains(res.Message, "现金口径") || !strings.Contains(res.Message, "经营口径") {
					t.Fatalf("multi metric answer should expose cash and operating views, got: %s", res.Message)
				}
				if strings.Index(res.Message, "现金口径") > strings.Index(res.Message, "经营口径") {
					t.Fatalf("multi metric answer should present cash view before operating view, got: %s", res.Message)
				}
				assertViewAliases(t, res)
				metrics, ok := res.Data["metrics"].(map[string]any)
				if !ok {
					t.Fatalf("missing metrics payload: %+v", res.Data)
				}
				if metrics["收入"] != float64(800) || metrics["成本"] != float64(300) || metrics["利润"] != float64(500) {
					t.Fatalf("unexpected metrics payload: %+v", metrics)
				}
			},
		},
		{
			Name:     "core_metric_exposes_source_backed_fact_sets",
			Question: "2026年2月收入是多少？",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
				factSets, ok := res.Data["fact_sets"].([]query.FactSet)
				if !ok || len(factSets) == 0 {
					t.Fatalf("fact_sets missing or empty: %#v", res.Data["fact_sets"])
				}
				var coreFactSet *query.FactSet
				for i := range factSets {
					if factSets[i].Source == "core_metrics" {
						coreFactSet = &factSets[i]
						break
					}
				}
				if coreFactSet == nil {
					t.Fatalf("core_metrics fact set missing: %#v", factSets)
				}
				foundCash := false
				foundAccrual := false
				for _, fact := range coreFactSet.Facts {
					switch fact.MetricKey {
					case "cash_receipts":
						foundCash = true
					case "accrual_revenue":
						foundAccrual = true
					}
				}
				if !foundCash || !foundAccrual {
					t.Fatalf("core metric facts missing cash/accrual revenue: %+v", coreFactSet.Facts)
				}
				if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
					t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
				}
			},
		},
		{
			Name:     "ambiguous_pre_shou_kuan_routes_to_arap_without_fallback",
			Question: "汇智在2026年2月这笔是供应商付款还是预收款？",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
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
			},
		},
		{
			Name:     "hr_breakdown_not_routed_to_counterparty_entity",
			Question: "2026年3月人力成本多少？工资、社保、公积金分别是多少？",
			DBPath:   buildEntityRoutingTestDB,
			Assert: func(t *testing.T, res query.Result) {
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
			},
		},
	})
}

func TestRangeRevenueQuestionsAggregateAcrossPeriods(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	for _, stmt := range []string{`DELETE FROM fin_fund_income`, `DELETE FROM fin_contracts`} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("delete contract seed data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	cases := []struct {
		name           string
		question       string
		wantPeriod     string
		wantRequested  string
		wantCoverageTo string
		wantTruncated  bool
		wantRevenue    float64
	}{
		{name: "quarter", question: "2026年第一季度营收", wantPeriod: "2026-01~2026-02", wantRequested: "2026-01~2026-03", wantCoverageTo: "2026-02", wantTruncated: true, wantRevenue: 2000},
		{name: "half year", question: "2026年上半年营收", wantPeriod: "2026-01~2026-02", wantRequested: "2026-01~2026-06", wantCoverageTo: "2026-02", wantTruncated: true, wantRevenue: 2000},
		{name: "full year", question: "2026年全年营收", wantPeriod: "2026-01~2026-02", wantRequested: "2026-01~2026-12", wantCoverageTo: "2026-02", wantTruncated: true, wantRevenue: 2000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := engine.Query(tc.question)
			if !res.Success {
				t.Fatalf("query failed: %+v", res)
			}
			if got, ok := res.Data["account_value"].(float64); !ok || got != tc.wantRevenue {
				t.Fatalf("account_value = %v, want %v, message=%s", res.Data["account_value"], tc.wantRevenue, res.Message)
			}
			if got, ok := res.Data["period"].(string); !ok || got != tc.wantPeriod {
				t.Fatalf("period = %v, want %s", res.Data["period"], tc.wantPeriod)
			}
			if got, ok := res.Data["requested_period"].(string); !ok || got != tc.wantRequested {
				t.Fatalf("requested_period = %v, want %s", res.Data["requested_period"], tc.wantRequested)
			}
			coverage, ok := res.Data["coverage"].(map[string]any)
			if !ok {
				t.Fatalf("missing coverage metadata: %+v", res.Data)
			}
			if got := coverage["actual_to"]; got != tc.wantCoverageTo {
				t.Fatalf("coverage.actual_to = %v, want %s", got, tc.wantCoverageTo)
			}
			if got, ok := coverage["truncated"].(bool); !ok || got != tc.wantTruncated {
				t.Fatalf("coverage.truncated = %v, want %t", coverage["truncated"], tc.wantTruncated)
			}
			if !strings.Contains(res.Message, tc.wantRequested) || !strings.Contains(res.Message, tc.wantPeriod) {
				t.Fatalf("expected requested and actual periods in message, got: %s", res.Message)
			}
		})
	}
}

func TestSupplierPaymentQuestionUsesPeriodScopedExternalSuppliers(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-05', '供应商A有限公司', '技术服务费', 1000, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-06', '北京市中闻（南京）律师事务所', '法律服务费', 500, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-07', '南京优集数据科技有限公司深圳分公司', '内部转账', 700, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-08', '梁梦瑶', '报销', 200, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-09', '暂收款', '实时缴税', 300, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03-10', '网上电子汇划收入', '手续费', 10, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-05', 'V-SUP-1', '640101', '主营业务成本', '借', 1000, '供应商A成本确认', '供应商A有限公司', 1000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-03', '2026-03-05', 'V-SUP-1', '220201', '应付账款', '贷', 1000, '供应商A成本确认', '供应商A有限公司', 0, 1000)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert supplier payment seed data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月有多少家供应商发生付款？分别叫什么、各付了多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got, ok := res.Data["period"].(string); !ok || got != "2026-03" {
		t.Fatalf("period = %v, want 2026-03", res.Data["period"])
	}
	switch got := res.Data["count"].(type) {
	case float64:
		if got != 2 {
			t.Fatalf("count = %v, want 2", res.Data["count"])
		}
	case int:
		if got != 2 {
			t.Fatalf("count = %v, want 2", res.Data["count"])
		}
	default:
		t.Fatalf("count has unexpected type %T", res.Data["count"])
	}
	if got, ok := res.Data["total"].(float64); !ok || got != 1500 {
		t.Fatalf("total = %v, want 1500", res.Data["total"])
	}
	suppliers, ok := res.Data["suppliers"].([]map[string]any)
	if !ok {
		t.Fatalf("suppliers payload missing: %+v", res.Data)
	}
	if len(suppliers) != 2 {
		t.Fatalf("supplier rows = %d, want 2", len(suppliers))
	}
	gotNames := map[string]float64{}
	for _, item := range suppliers {
		name, _ := item["name"].(string)
		amount, _ := item["out_amount"].(float64)
		gotNames[name] = amount
	}
	if gotNames["供应商A有限公司"] != 1000 || gotNames["北京市中闻（南京）律师事务所"] != 500 {
		t.Fatalf("unexpected supplier payment rows: %+v", gotNames)
	}
	if strings.Contains(res.Message, "36 个") || strings.Contains(res.Message, "净流出") {
		t.Fatalf("supplier payment answer should not use legacy supplier-count wording: %s", res.Message)
	}
	if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
		t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
	}

	roster := engine.Query("2026年3月供应商有哪些")
	if !roster.Success {
		t.Fatalf("supplier roster query failed: %+v", roster)
	}
	if strings.Contains(roster.Message, "语义模糊") {
		t.Fatalf("supplier roster should not fall back as ambiguous, got %s", roster.Message)
	}
	rosterSuppliers, ok := roster.Data["suppliers"].([]map[string]any)
	if !ok || len(rosterSuppliers) != 2 {
		t.Fatalf("supplier roster payload = %+v, want two suppliers", roster.Data["suppliers"])
	}
}

func TestEngineUsesConfigurableMetricKeywords(t *testing.T) {
	skipHeavyQueryTest(t)

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
	skipHeavyQueryTest(t)

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
	skipHeavyQueryTest(t)

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

func TestCounterpartyClassificationRetroWritesActualRangeIntoQuerySpec(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("飞未云科这个主体目前更像客户、供应商还是混合往来？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "2026-01~2026-03") {
		t.Fatalf("expected retro range in message, got: %s", res.Message)
	}

	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["period_from"]; got != "2026-01" {
		t.Fatalf("period_from = %v, want 2026-01", got)
	}
	if got := spec["period_to"]; got != "2026-03" {
		t.Fatalf("period_to = %v, want 2026-03", got)
	}
}

func TestEngineUsesConfigurableProfitSingleViewBlockKeywords(t *testing.T) {
	skipHeavyQueryTest(t)

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

	res := engine.Query("2026年2月账上净赚为什么不一致？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "现金口径") || !strings.Contains(res.Message, "经营口径") {
		t.Fatalf("custom profit single-view block keyword should force dual perspective, got: %s", res.Message)
	}
}

func TestCounterpartyCostShouldIncludeVoucherSiblingRows(t *testing.T) {
	runParallelHeavyQueryTest(t)

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

	res := engine.Query("南京林悦智能科技有限公司2月账上成本多少")
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

func TestSupplierCostQuestionFallsBackToPaymentButKeepsSupplierCostWording(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
VALUES ('南京优集数据科技有限公司', '2026-03-15', '南京林悦智能科技有限公司', '供应商付款', 1915915.19, 0)
`); err != nil {
		t.Fatalf("insert supplier payment bank row failed: %v", err)
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("南京林悦智能科技有限公司3月银行流水付款口径成本多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["role"]; got != "supplier" {
		t.Fatalf("role = %v, want supplier", got)
	}
	if got := res.Data["amount"]; got != float64(1915915.19) {
		t.Fatalf("amount = %v, want 1915915.19", got)
	}
	if !strings.Contains(res.Message, "供应商") || !strings.Contains(res.Message, "成本") || !strings.Contains(res.Message, "付款口径") {
		t.Fatalf("expected supplier cost fallback wording, got: %s", res.Message)
	}
}

func TestInternalPartyUsesConfigurableOrgSuffixesAndContextKeywords(t *testing.T) {
	skipHeavyQueryTest(t)

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
	runParallelHeavyQueryTest(t)

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

	res := engine.Query("账上看林悦在2026年2月这笔是成本还是收入？请给判断依据。")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "供应商/成本侧") {
		t.Fatalf("expected supplier classification, got: %s", res.Message)
	}
}

func TestHRBreakdownListsBranchTransferSeparatelyWhenVoucherHasPayrollLiability(t *testing.T) {
	runParallelHeavyQueryTest(t)

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
	runParallelHeavyQueryTest(t)

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
	runParallelHeavyQueryTest(t)

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

	res := engine.Query("辽宁金程信息科技有限公司账上应收/应付是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "按开放项推断") {
		t.Fatalf("message should disclose inferred open-item basis, got: %s", res.Message)
	}
}

func TestEntityARAPQuestionUsesCounterpartyOpenItems(t *testing.T) {
	runParallelHeavyQueryTest(t)

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

	res := engine.Query("南京林悦智能科技有限公司账上应收/应付是多少？")
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
	if _, exists := res.Data["contract_fallback_target"]; exists {
		t.Fatalf("explicit financial AR/AP should not mark a contract fallback target: %+v", res.Data)
	}
}

func TestCounterpartyReceiptsQuestionIncludesCurrentMonthBreakdown(t *testing.T) {
	runParallelHeavyQueryTest(t)

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

	res := engine.Query("金程今年银行流水回款多少？其中3月到账多少？")
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

func TestLargeTransactionQuestionUsesDedicatedBankFlowQuery(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月最大的单笔流入对手方是谁，金额多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["counterparty"]; got != "飞未云科" {
		t.Fatalf("counterparty = %v, want 飞未云科", got)
	}
	if got := res.Data["amount"]; got != float64(904) {
		t.Fatalf("amount = %v, want 904", got)
	}
	if !strings.Contains(res.Message, "最大流入对手方") {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestLargeOutflowQuestionUsesDedicatedBankFlowQuery(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-20', '供应商A有限公司', '大额付款', 200000, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02-21', '供应商B有限公司', '普通付款', 700, 0)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert outflow seed data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月单笔最大流出来自谁？金额多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["counterparty"]; got != "供应商A有限公司" {
		t.Fatalf("counterparty = %v, want 供应商A有限公司", got)
	}
	if got := res.Data["amount"]; got != float64(200000) {
		t.Fatalf("amount = %v, want 200000", got)
	}
	if !strings.Contains(res.Message, "最大流出对手方") {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestLargeTransactionRosterQuestionReturnsInboundAndOutboundLists(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildEntityRoutingTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-16', '辽宁鼎元信息科技有限公司', '结算款', 0, 2102065.91)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-05', '云栖智数（深圳）技术有限公司', '结算款', 0, 1450200)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-05', '云栖智数（深圳）技术有限公司', '结算款', 0, 1250400)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-06', '北京瀚研国际咨询有限公司', '服务费', 2450000, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01-19', '南京澜阅智能科技有限公司', '服务费', 1490815, 0)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert large transaction seed data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年Q1有哪几笔大额的进账和支出?分别是跟谁的?")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	inbound, ok := res.Data["inbound_transactions"].([]map[string]any)
	if !ok || len(inbound) < 3 {
		t.Fatalf("inbound_transactions = %#v, want at least 3 rows", res.Data["inbound_transactions"])
	}
	outbound, ok := res.Data["outbound_transactions"].([]map[string]any)
	if !ok || len(outbound) < 2 {
		t.Fatalf("outbound_transactions = %#v, want at least 2 rows", res.Data["outbound_transactions"])
	}
	for _, want := range []string{"辽宁鼎元", "云栖智数", "瀚研", "澜阅", "2450000.00", "2102065.91"} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("message = %q, want include %q", res.Message, want)
		}
	}
}

func buildEntityRoutingTestDB(t *testing.T) string {
	t.Helper()

	return cloneSQLiteFixture(t, "entity-routing", func(dbPath string) {
		buildEntityRoutingTestDBAt(t, dbPath)
	})
}

func buildEntityRoutingTestDBAt(t *testing.T, dbPath string) {
	t.Helper()

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
			current_amount REAL,
			cumulative_amount REAL
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
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C-FW-001', '飞未云科（深圳）技术有限公司', '飞未云科项目-京东价格数据')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C-FW-002', '飞未云科（深圳）技术有限公司', '飞未云科项目-京东销量数据')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-001', '2026-01', 'contract_fund_income', '26年Q1收入明细', 1200, 1200, '是', 1200)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-002', '2026-02', 'contract_fund_income', '26年Q1收入明细', 800, 800, '是', 800)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-002', '2026-03', 'contract_fund_income', '26年Q1收入明细', 900, 900, '是', 900)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01', '一、营业收入', 1200, 1200)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-01', '五、净利润', 800, 800)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '一、营业收入', 800, 2000)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '五、净利润', 500, 1300)`,
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
}
