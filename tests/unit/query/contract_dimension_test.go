package query_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestContractDimensionQueryScenarios(t *testing.T) {
	runParallelQueryScenarios(t, []queryScenario{
		{
			Name:     "customer_contract_book_and_cash_views",
			Question: "辽宁金程信息科技有限公司2025年合同结算多少？其中10月到账多少？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				assertCashBeforeFinancialView(t, res.Message)
				if got := res.Data["role"]; got != "customer_contract" {
					t.Fatalf("role = %v, want customer_contract", got)
				}
				if got := res.Data["sub_period_receipts"]; got != float64(1234) {
					t.Fatalf("sub_period_receipts = %v, want 1234", got)
				}
				assertViewAliases(t, res)
				if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
					t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
				}
			},
		},
		{
			Name:     "supplier_contract_cost_and_bank_payments",
			Question: "南京林悦智能科技有限公司2025年合同成本多少？实际付款多少？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				assertCashBeforeFinancialView(t, res.Message)
				if got := res.Data["role"]; got != "supplier_contract" {
					t.Fatalf("role = %v, want supplier_contract", got)
				}
				if got := res.Data["cash_paid_amount"]; got != float64(666) {
					t.Fatalf("cash_paid_amount = %v, want 666", got)
				}
				assertViewAliases(t, res)
			},
		},
		{
			Name:     "supplier_contract_uses_merged_cost_settlement_groups",
			Question: "上海合并供应商科技有限公司2026年Q1合同成本多少？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if got := res.Data["role"]; got != "supplier_contract" {
					t.Fatalf("role = %v, want supplier_contract", got)
				}
				bookView, ok := res.Data["book_view"].(map[string]any)
				if !ok {
					t.Fatalf("book_view missing: %+v", res.Data)
				}
				if got := bookView["contract_cost"]; got != float64(600) {
					t.Fatalf("contract_cost = %v, want 600", got)
				}
				contracts, ok := res.Data["contracts"].([]map[string]any)
				if !ok {
					t.Fatalf("contracts missing: %+v", res.Data["contracts"])
				}
				if len(contracts) != 4 {
					t.Fatalf("contract count = %d, want 4: %#v", len(contracts), contracts)
				}
				if !strings.Contains(res.Message, "合同成本 600.00 元") {
					t.Fatalf("message should include merged cost group total, got: %s", res.Message)
				}
			},
		},
		{
			Name:     "mixed_contract_uses_cash_first_dual_answer",
			Question: "南京众信数通智能科技有限公司2025年合同收入结算、合同成本、到账、付款分别是多少？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				assertCashBeforeFinancialView(t, res.Message)
				if got := res.Data["role"]; got != "mixed_contract" {
					t.Fatalf("role = %v, want mixed_contract", got)
				}
				assertViewAliases(t, res)
			},
		},
		{
			Name:     "profit_without_contract_keyword_still_uses_contract_dimension",
			Question: "南京众信数通智能科技有限公司2025年利润多少？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
					t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
				}
				if !strings.Contains(res.Message, "合同利润 180.00 元") {
					t.Fatalf("message should contain contract book profit, got: %s", res.Message)
				}
				if !strings.Contains(res.Message, "净回款 192.00 元") {
					t.Fatalf("message should contain cash net receipts, got: %s", res.Message)
				}
			},
		},
		{
			Name:     "contract_content_question_uses_contract_dimension",
			Question: "行业商品数据采购合同A01内容是什么？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if !strings.Contains(res.Message, "行业商品数据采购合同-A01") {
					t.Fatalf("message should contain contract content, got: %s", res.Message)
				}
			},
		},
		{
			Name:     "revenue_without_contract_keyword_uses_contract_dimension",
			Question: "辽宁金程信息科技有限公司2025年营收多少？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if !strings.Contains(res.Message, "合同台账结算 3000.00 元") {
					t.Fatalf("message should contain contract settlement revenue, got: %s", res.Message)
				}
				sourceTables, ok := res.Data["source_tables"].([]string)
				if !ok {
					t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
				}
				if len(sourceTables) == 0 || sourceTables[0] != "tenant_uhub.fin_contracts" {
					t.Fatalf("source_tables should start with tenant_uhub.fin_contracts, got %#v", sourceTables)
				}
			},
		},
		{
			Name:     "alias_revenue_uses_contract_dimension",
			Question: "飞未云科2026年累计销售额多少？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if !strings.Contains(res.Message, "合同台账结算 3600.00 元") {
					t.Fatalf("message should use contract ledger revenue, got: %s", res.Message)
				}
			},
		},
		{
			Name:     "customer_contract_uses_merged_fund_income_groups",
			Question: "Yipit data 2026年Q1回款和结算金额是多少？",
			DBPath:   buildContractQueryTestDB,
			Assert: func(t *testing.T, res query.Result) {
				if got := res.Data["role"]; got != "customer_contract" {
					t.Fatalf("role = %v, want customer_contract", got)
				}
				bookView, ok := res.Data["book_view"].(map[string]any)
				if !ok {
					t.Fatalf("book_view missing: %+v", res.Data)
				}
				cashView, ok := res.Data["cash_view"].(map[string]any)
				if !ok {
					t.Fatalf("cash_view missing: %+v", res.Data)
				}
				if got := bookView["settlement_amount"]; got != float64(600) {
					t.Fatalf("settlement_amount = %v, want 600", got)
				}
				if got := cashView["received_amount"]; got != float64(570) {
					t.Fatalf("received_amount = %v, want 570", got)
				}
				contracts, ok := res.Data["contracts"].([]map[string]any)
				if !ok {
					t.Fatalf("contracts missing: %+v", res.Data["contracts"])
				}
				if len(contracts) != 4 {
					t.Fatalf("contract count = %d, want 4: %#v", len(contracts), contracts)
				}
				for _, contract := range contracts {
					content, _ := contract["contract_content"].(string)
					if strings.Contains(content, "合并金额组") {
						t.Fatalf("contracts should not expose merged pseudo contract: %#v", contracts)
					}
				}
				if !strings.Contains(res.Message, "实际到账 570.00 元") || !strings.Contains(res.Message, "合同台账结算 600.00 元") {
					t.Fatalf("message should include merged group totals, got: %s", res.Message)
				}
			},
		},
	})
}

func TestCompoundCustomerUnpaidQuestionUsesRevenueSideForAllEntitiesAndPeriods(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	seedCompoundCustomerUnpaidRows(t, dbPath)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("给我统计一下四川其妙和辽宁金程25年Q4以及26Q1 未付款的金额")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != query.QueryFamilyContractDimension {
		t.Fatalf("query_family = %v, want %s", got, query.QueryFamilyContractDimension)
	}
	if got := spec["metric_kind"]; got != query.MetricKindReceipts {
		t.Fatalf("metric_kind = %v, want %s", got, query.MetricKindReceipts)
	}
	route, ok := res.Data["route_decision"].(map[string]any)
	if !ok {
		t.Fatalf("route_decision missing: %+v", res.Data)
	}
	primary, ok := route["primary_tables"].([]string)
	if !ok {
		t.Fatalf("route primary_tables missing: %#v", route["primary_tables"])
	}
	if !containsString(primary, "fin_fund_income") || containsString(primary, "fin_cost_settlements") {
		t.Fatalf("primary_tables = %#v, want revenue-side fund income only", primary)
	}

	items, ok := res.Data["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %#v", res.Data["items"])
	}
	if len(items) != 4 {
		t.Fatalf("items len = %d, want 4: %#v", len(items), items)
	}
	assertCompoundUnpaidItem(t, items, "四川其妙科技有限公司", "2026年Q1", "matched", 1600)
	assertCompoundUnpaidItem(t, items, "辽宁金程信息科技有限公司", "2025年Q4", "matched", 266)
	assertCompoundUnpaidItem(t, items, "四川其妙科技有限公司", "2025年Q4", "missing", 0)
	assertCompoundUnpaidItem(t, items, "辽宁金程信息科技有限公司", "2026年Q1", "missing", 0)

	if !strings.Contains(res.Message, "四川其妙科技有限公司 2026年Q1") ||
		!strings.Contains(res.Message, "辽宁金程信息科技有限公司 2025年Q4") ||
		strings.Contains(res.Message, "成本合同口径") {
		t.Fatalf("message should answer deterministically from revenue-side contract income, got: %s", res.Message)
	}
}

func TestCompoundCustomerContractQuestionSupportsMultipleRevenueMetrics(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	seedCompoundCustomerUnpaidRows(t, dbPath)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("给我统计一下四川其妙和辽宁金程25年Q4以及26Q1 结算金额、收款金额、开票金额、未付款金额")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["query_pipeline"]; got != "compound_source_query" {
		t.Fatalf("query_pipeline = %v, want compound_source_query", got)
	}
	metrics, ok := res.Data["requested_metrics"].([]string)
	if !ok {
		t.Fatalf("requested_metrics missing: %#v", res.Data["requested_metrics"])
	}
	for _, want := range []string{"contract_revenue.settlement", "contract_revenue.received", "contract_revenue.invoice", "contract_revenue.unpaid"} {
		if !containsString(metrics, want) {
			t.Fatalf("requested_metrics missing %s, got %#v", want, metrics)
		}
	}
	items, ok := res.Data["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %#v", res.Data["items"])
	}
	assertCompoundContractMetricItem(t, items, "四川其妙科技有限公司", "2026年Q1", map[string]float64{
		"contract_revenue.settlement": 2500,
		"contract_revenue.received":   900,
		"contract_revenue.invoice":    500,
		"contract_revenue.unpaid":     1600,
	})
	assertCompoundContractMetricItem(t, items, "辽宁金程信息科技有限公司", "2025年Q4", map[string]float64{
		"contract_revenue.settlement": 3000,
		"contract_revenue.received":   2734,
		"contract_revenue.invoice":    3000,
		"contract_revenue.unpaid":     266,
	})
	if !strings.Contains(res.Message, "已开票 500.00 元") || !strings.Contains(res.Message, "未付款 1600.00 元") {
		t.Fatalf("message should include requested invoice and unpaid metrics, got: %s", res.Message)
	}
}

func TestCompoundCustomerContractQuestionDoesNotRequireUnpaidMetric(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	seedCompoundCustomerUnpaidRows(t, dbPath)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("给我统计一下四川其妙和辽宁金程25年Q4以及26Q1 结算金额和收款金额")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["query_pipeline"]; got != "compound_source_query" {
		t.Fatalf("query_pipeline = %v, want compound_source_query", got)
	}
	metrics, ok := res.Data["requested_metrics"].([]string)
	if !ok {
		t.Fatalf("requested_metrics missing: %#v", res.Data["requested_metrics"])
	}
	if !containsString(metrics, "contract_revenue.settlement") || !containsString(metrics, "contract_revenue.received") || containsString(metrics, "contract_revenue.unpaid") {
		t.Fatalf("requested_metrics = %#v, want settlement and received only", metrics)
	}
	if strings.Contains(res.Message, "未付款") {
		t.Fatalf("message should not include unrequested unpaid metric, got: %s", res.Message)
	}
}

func TestCompoundCustomerUnpaidQuestionReturnsProjectDetails(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	seedCompoundCustomerUnpaidRows(t, dbPath)
	seedCompoundCustomerMergedRevenueOwnerRows(t, dbPath)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("帮我整理下四川其妙和辽宁金程 2025Q4和2026Q1 已结算未付款的项目和对应金额，另外拆开具体项目明细")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["query_pipeline"]; got != "compound_source_query" {
		t.Fatalf("query_pipeline = %v, want compound_source_query", got)
	}
	details, ok := res.Data["detail_items"].([]map[string]any)
	if !ok || len(details) == 0 {
		t.Fatalf("detail_items missing or empty: %#v", res.Data["detail_items"])
	}
	assertCompoundDetailItem(t, details, "四川其妙科技有限公司", "2026年Q1", "四川其妙数据服务合同", 3200, 1600, 1600)
	assertCompoundDetailItem(t, details, "辽宁金程信息科技有限公司", "2025年Q4", "行业商品数据采购合同-A01", 3000, 2734, 266)
	assertNoCompoundDetailItem(t, details, "四川其妙科技有限公司", "2025年Q4")
	assertNoCompoundDetailItem(t, details, "辽宁金程信息科技有限公司", "2026年Q1")

	items, ok := res.Data["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %#v", res.Data["items"])
	}
	for _, item := range items {
		if item["entity"] == "四川其妙科技有限公司" && item["period_label"] == "2026年Q1" {
			if nested, ok := item["detail_items"].([]map[string]any); !ok || len(nested) != 1 {
				t.Fatalf("四川其妙 2026Q1 nested detail_items = %#v, want one item", item["detail_items"])
			}
			return
		}
	}
	t.Fatalf("missing 四川其妙 2026Q1 aggregate item in %#v", items)
}

func TestCompoundPlannerSupportsContractCostTableMetrics(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	seedCompoundCustomerUnpaidRows(t, dbPath)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("北京成本供应商和上海合并供应商25年Q4以及26Q1 合同成本、已付款、已收票未付款")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["query_pipeline"]; got != "compound_source_query" {
		t.Fatalf("query_pipeline = %v, want compound_source_query", got)
	}
	items, ok := res.Data["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %#v", res.Data["items"])
	}
	assertCompoundContractMetricItem(t, items, "北京成本供应商科技有限公司", "2025年Q4", map[string]float64{
		"contract_cost.settlement":      1000,
		"contract_cost.paid":            300,
		"contract_cost.invoiced_unpaid": 500,
	})
	assertCompoundContractMetricItem(t, items, "上海合并供应商科技有限公司", "2026年Q1", map[string]float64{
		"contract_cost.settlement":      600,
		"contract_cost.paid":            570,
		"contract_cost.invoiced_unpaid": 30,
	})
	primary, ok := res.Data["primary_source_tables"].([]string)
	if !ok || !containsString(primary, "fin_cost_settlements") || !containsString(primary, "fin_cost_settlement_groups") {
		t.Fatalf("primary_source_tables = %#v, want cost settlement tables", res.Data["primary_source_tables"])
	}
}

func TestCompoundPlannerSupportsBankStatementMetrics(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	seedCompoundCustomerUnpaidRows(t, dbPath)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("北京成本供应商和上海合并供应商25年Q4以及26Q1 银行流水付款金额")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	items, ok := res.Data["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %#v", res.Data["items"])
	}
	assertCompoundContractMetricItem(t, items, "北京成本供应商科技有限公司", "2025年Q4", map[string]float64{
		"bank_statement.paid": 444,
	})
	assertCompoundContractMetricItem(t, items, "上海合并供应商科技有限公司", "2026年Q1", map[string]float64{
		"bank_statement.paid": 555,
	})
	primary, ok := res.Data["primary_source_tables"].([]string)
	if !ok || !containsString(primary, "fin_bank_statement") {
		t.Fatalf("primary_source_tables = %#v, want bank statement", res.Data["primary_source_tables"])
	}
}

func TestCompoundPlannerSupportsJournalMetrics(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	seedCompoundCustomerUnpaidRows(t, dbPath)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("北京成本供应商和上海合并供应商25年Q4以及26Q1 序时账借方金额和贷方金额")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	items, ok := res.Data["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %#v", res.Data["items"])
	}
	assertCompoundContractMetricItem(t, items, "北京成本供应商科技有限公司", "2025年Q4", map[string]float64{
		"journal.debit":  1000,
		"journal.credit": 0,
	})
	assertCompoundContractMetricItem(t, items, "上海合并供应商科技有限公司", "2026年Q1", map[string]float64{
		"journal.debit":  0,
		"journal.credit": 200,
	})
	primary, ok := res.Data["primary_source_tables"].([]string)
	if !ok || !containsString(primary, "fin_journal") {
		t.Fatalf("primary_source_tables = %#v, want journal", res.Data["primary_source_tables"])
	}
}

func TestCompoundPlannerSupportsMixedSourceMetricsInOneQuestion(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	seedCompoundCustomerUnpaidRows(t, dbPath)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("四川其妙和北京成本供应商25年Q4以及26Q1 合同收入结算金额、合同成本、银行流水付款金额、序时账借方金额")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["query_pipeline"]; got != "compound_source_query" {
		t.Fatalf("query_pipeline = %v, want compound_source_query", got)
	}
	metrics, ok := res.Data["requested_metrics"].([]string)
	if !ok {
		t.Fatalf("requested_metrics missing: %#v", res.Data["requested_metrics"])
	}
	for _, want := range []string{
		"contract_revenue.settlement",
		"contract_cost.settlement",
		"bank_statement.paid",
		"journal.debit",
	} {
		if !containsString(metrics, want) {
			t.Fatalf("requested_metrics missing %s, got %#v", want, metrics)
		}
	}
	if containsString(metrics, "bank_statement.received") {
		t.Fatalf("requested_metrics = %#v, should not treat 合同收入 as bank received", metrics)
	}
	primary, ok := res.Data["primary_source_tables"].([]string)
	if !ok {
		t.Fatalf("primary_source_tables missing: %#v", res.Data["primary_source_tables"])
	}
	for _, want := range []string{
		"fin_fund_income",
		"fin_cost_settlements",
		"fin_bank_statement",
		"fin_journal",
	} {
		if !containsString(primary, want) {
			t.Fatalf("primary_source_tables missing %s, got %#v", want, primary)
		}
	}
	items, ok := res.Data["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %#v", res.Data["items"])
	}
	assertCompoundContractMetricItem(t, items, "四川其妙科技有限公司", "2026年Q1", map[string]float64{
		"contract_revenue.settlement": 2500,
		"contract_cost.settlement":    0,
		"bank_statement.paid":         0,
		"journal.debit":               0,
	})
	assertCompoundContractMetricItem(t, items, "北京成本供应商科技有限公司", "2025年Q4", map[string]float64{
		"contract_revenue.settlement": 0,
		"contract_cost.settlement":    1000,
		"bank_statement.paid":         444,
		"journal.debit":               1000,
	})
}

func assertCompoundUnpaidItem(t *testing.T, items []map[string]any, entity, periodLabel, status string, wantUnpaid float64) {
	t.Helper()
	for _, item := range items {
		if item["entity"] == entity && item["period_label"] == periodLabel {
			if got := item["coverage_status"]; got != status {
				t.Fatalf("%s %s coverage_status = %v, want %s", entity, periodLabel, got, status)
			}
			if got := item["unpaid_amount"]; got != wantUnpaid {
				t.Fatalf("%s %s unpaid_amount = %v, want %.2f", entity, periodLabel, got, wantUnpaid)
			}
			return
		}
	}
	t.Fatalf("missing item entity=%s period=%s in %#v", entity, periodLabel, items)
}

func assertCompoundContractMetricItem(t *testing.T, items []map[string]any, entity, periodLabel string, want map[string]float64) {
	t.Helper()
	for _, item := range items {
		if item["entity"] != entity || item["period_label"] != periodLabel {
			continue
		}
		metrics, ok := item["metrics"].(map[string]any)
		if !ok {
			t.Fatalf("%s %s metrics missing: %#v", entity, periodLabel, item["metrics"])
		}
		for key, wantValue := range want {
			if got := metrics[key]; got != wantValue {
				t.Fatalf("%s %s metrics[%s] = %v, want %.2f", entity, periodLabel, key, got, wantValue)
			}
		}
		return
	}
	t.Fatalf("missing item entity=%s period=%s in %#v", entity, periodLabel, items)
}

func assertCompoundDetailItem(t *testing.T, items []map[string]any, entity, periodLabel, content string, wantSettlement, wantReceived, wantUnpaid float64) {
	t.Helper()
	for _, item := range items {
		if item["entity"] != entity || item["period_label"] != periodLabel || item["contract_content"] != content {
			continue
		}
		if got := item["settlement_amount"]; got != wantSettlement {
			t.Fatalf("%s %s %s settlement_amount = %v, want %.2f", entity, periodLabel, content, got, wantSettlement)
		}
		if got := item["received_amount"]; got != wantReceived {
			t.Fatalf("%s %s %s received_amount = %v, want %.2f", entity, periodLabel, content, got, wantReceived)
		}
		if got := item["unpaid_amount"]; got != wantUnpaid {
			t.Fatalf("%s %s %s unpaid_amount = %v, want %.2f", entity, periodLabel, content, got, wantUnpaid)
		}
		return
	}
	t.Fatalf("missing detail item entity=%s period=%s content=%s in %#v", entity, periodLabel, content, items)
}

func assertNoCompoundDetailItem(t *testing.T, items []map[string]any, entity, periodLabel string) {
	t.Helper()
	for _, item := range items {
		if item["entity"] == entity && item["period_label"] == periodLabel {
			t.Fatalf("unexpected detail item for %s %s: %#v", entity, periodLabel, item)
		}
	}
}

func seedCompoundCustomerUnpaidRows(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	inserts := []string{
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C030', '四川其妙科技有限公司', '四川其妙数据服务合同')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C031', '四川尧皆圭瑶科技有限公司', '四川尧皆圭瑶数据服务合同')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C032', '北京成本供应商科技有限公司', '北京成本供应商服务合同')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C030', '2026-01', 'contract_fund_income', '26年Q1收入明细', 1000, 400, '否', 0)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C030', '2026-02', 'contract_fund_income', '26年Q1收入明细', 500, 500, '是', 500)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C030', '2026-03', 'contract_fund_income', '26年Q1收入明细', 1000, 0, '否', 0)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C031', '2026-03', 'contract_fund_income', '26年Q1收入明细', 900, 900, '是', 900)`,
		`INSERT INTO fin_cost_settlement_groups(id, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, customer_name, quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code)
		 VALUES (301, '2025-10', 'contract_revenue_cost', '成本-月度结算', 30, 30, '北京成本供应商科技有限公司', '1项', 1000, '是', 800, 300, '640101')`,
		`INSERT INTO fin_cost_settlement_group_members(group_id, contract_id, source_row_number) VALUES (301, 'C032', 30)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount) VALUES ('南京优集数据科技有限公司', '2025-10-20', '北京成本供应商科技有限公司', '银行付款', 444, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount) VALUES ('南京优集数据科技有限公司', '2026-02-20', '上海合并供应商科技有限公司', '银行付款', 555, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2025-10', '2025-10-20', 'J-COST-1', '640101', '主营业务成本', '借', 1000, '成本确认', '北京成本供应商科技有限公司', 1000, 0)`,
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-02', '2026-02-20', 'J-COST-2', '220201', '应付账款', '贷', 200, '应付确认', '上海合并供应商科技有限公司', 0, 200)`,
	}
	for _, stmt := range inserts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert compound customer seed data failed: %v", err)
		}
	}
}

func seedCompoundCustomerMergedRevenueOwnerRows(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	inserts := []string{
		`INSERT INTO fin_fund_income_groups(id, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, customer_name, quantity, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES (401, '2026-01', 'contract_fund_income', '26年Q1收入明细', 40, 41, '四川其妙科技有限公司', '/', 700, 700, '是', 1200)`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number) VALUES (401, 'C030', 40), (401, 'C031', 41)`,
	}
	for _, stmt := range inserts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert compound merged revenue seed data failed: %v", err)
		}
	}
}

func TestContractMemberQuestionDoesNotAttributeWholeMergedFundIncomeGroup(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("数据采购合同-抖音2026年Q1收入是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["contract_answer_status"]; got != "missing" {
		t.Fatalf("contract_answer_status = %v, want missing because merged customer-level amount is not attributable to one member contract", got)
	}
	if _, ok := res.Data["book_view"]; ok {
		t.Fatalf("book_view should be absent for unattributed member contract, got: %+v", res.Data["book_view"])
	}
}

func TestContractMemberQuestionDoesNotAttributeWholeMergedCostSettlementGroup(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("外包服务合同-B2026年Q1成本是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if got := res.Data["contract_answer_status"]; got != "missing" {
		t.Fatalf("contract_answer_status = %v, want missing because merged supplier-level amount is not attributable to one member contract", got)
	}
	if _, ok := res.Data["book_view"]; ok {
		t.Fatalf("book_view should be absent for unattributed member contract, got: %+v", res.Data["book_view"])
	}
}

func TestCompanyAggregateMetricIncludesMergedFundIncomeGroups(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年Q1收入是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "营收 4200.00 元") {
		t.Fatalf("message should include merged group revenue, got: %s", res.Message)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	if got := summary["contract_count"]; got != float64(6) && got != 6 {
		t.Fatalf("contract_count = %v, want 6", got)
	}
}

func TestCompanyAggregateMetricIncludesMergedCostSettlementGroups(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年Q1成本是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "合同成本 600.00 元") {
		t.Fatalf("message should include merged group cost, got: %s", res.Message)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	if got := summary["contract_count"]; got != float64(4) && got != 4 {
		t.Fatalf("contract_count = %v, want 4", got)
	}
	if got := summary["cost_paid"]; got != float64(570) {
		t.Fatalf("cost_paid = %v, want 570", got)
	}
}

func TestCompanyAggregateMetricPrefersContractAggregateFirst(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月收入、成本、利润分别是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "老板口径先看合同/项目汇总") {
		t.Fatalf("message should prefer contract aggregate, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "营收 1300.00 元") || !strings.Contains(res.Message, "合同成本 1008.00 元") || !strings.Contains(res.Message, "利润 292.00 元") {
		t.Fatalf("message should use contract aggregate numbers, got: %s", res.Message)
	}
	if got, _ := res.Data["source_priority"].(string); got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first", res.Data["source_priority"])
	}
	if got, _ := res.Data["query_pipeline"].(string); got != "orchestrator" {
		t.Fatalf("query_pipeline = %v, want orchestrator", res.Data["query_pipeline"])
	}
	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	wantSourceTables := []string{
		"tenant_uhub.fin_contracts",
		"tenant_uhub.fin_fund_income",
		"tenant_uhub.fin_fund_income_groups",
		"tenant_uhub.fin_fund_income_group_members",
		"tenant_uhub.fin_cost_settlements",
		"tenant_uhub.fin_cost_settlement_groups",
		"tenant_uhub.fin_cost_settlement_group_members",
	}
	for _, want := range wantSourceTables {
		if !containsString(sourceTables, want) {
			t.Fatalf("source_tables missing %s, got %#v", want, sourceTables)
		}
	}
	if strings.Contains(res.Message, "合并金额组") {
		t.Fatalf("boss-facing message should not expose merged group helper labels, got: %s", res.Message)
	}
}

func TestCompanyAggregateInvoiceOpenDetailDoesNotExposeMergedGroupLabel(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年Q1应付账款多少（已收票未付款）？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if strings.Contains(res.Message, "合并金额组") {
		t.Fatalf("boss-facing message should not expose merged group helper labels, got: %s", res.Message)
	}
	if strings.Contains(res.Message, "多合同合计") {
		t.Fatalf("boss-facing message should use rigorous merged group wording, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "合并行合计（覆盖合同/项目：") || !strings.Contains(res.Message, "外包服务合同-A") {
		t.Fatalf("message should identify merged group through real member contracts, got: %s", res.Message)
	}
}

func TestCompanyAggregateGMVPrefersContractAggregateFirst(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月GMV多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "老板口径先看合同/项目汇总") {
		t.Fatalf("message should prefer contract aggregate, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "营收 1300.00 元") {
		t.Fatalf("message should treat GMV as revenue-like contract metric, got: %s", res.Message)
	}
}

func TestCompanyAggregateMetricIncludesSourceNoteFromTableComment(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月收入是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "来源：") {
		t.Fatalf("message should include source note, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "《优集资金收入计算表-副本.xlsx》") {
		t.Fatalf("message should include workbook source, got: %s", res.Message)
	}
	sourceNote, _ := res.Data["source_note"].(string)
	if !strings.Contains(sourceNote, "25年Q4收入明细") || !strings.Contains(sourceNote, "26年Q1收入明细") {
		t.Fatalf("source_note should expose contract sheet lineage, got: %v", res.Data["source_note"])
	}
}

func TestCompanyAggregateMetricFallsBackWhenContractSummaryMissingCoverage(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`DELETE FROM fin_fund_income`,
		`DELETE FROM fin_cost_settlements`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司', '2025-10', '营业收入', 900, 900)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司', '2025-10', '营业成本', 600, 600)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司', '2025-10', '利润总额', 300, 300)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('南京优集数据科技有限公司', '2025-10', '净利润', 300, 300)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("prepare fallback data failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2025年10月收入是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "合同口径当前不能直接回答") {
		t.Fatalf("message should stop at strict contract source, got: %s", res.Message)
	}
	if strings.Contains(res.Message, "先说现金口径") || strings.Contains(res.Message, "已回退到现金+经营/财务口径") {
		t.Fatalf("message should not auto fallback to dual perspective core metric, got: %s", res.Message)
	}
	if got := res.Data["contract_fallback_reason"]; got == nil {
		t.Fatalf("contract_fallback_reason missing: %+v", res.Data)
	}
	if got, _ := res.Data["contract_answer_status"].(string); got != "missing" {
		t.Fatalf("contract_answer_status = %q, want missing", got)
	}
}

func TestProjectMetricQuestionUsesContractDimensionRouting(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("辽宁金程信息科技有限公司项目2025年收入多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "合同台账结算 3000.00 元") {
		t.Fatalf("message should use contract dimension result, got: %s", res.Message)
	}
}

func TestContractSourceAdapterReturnsCustomerContractFacts(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	adapter := query.NewContractSourceAdapter(engine)
	spec := query.BuildQuerySpec("辽宁金程信息科技有限公司2025年合同结算多少？其中10月到账多少？", contractAnchor())

	factSet, err := adapter.Fetch(context.Background(), spec)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if factSet.Source != "contracts" {
		t.Fatalf("source = %s, want contracts", factSet.Source)
	}
	assertFactValue(t, factSet, "contract_match_count", 1)
	assertFactValue(t, factSet, "contract_book_settlement", 3000)
	assertFactValue(t, factSet, "contract_book_invoice", 3000)
	assertFactValue(t, factSet, "contract_cash_received", 2734)
	assertFactValue(t, factSet, "contract_cash_received_subperiod", 1234)
}

func TestContractSourceAdapterHonorsSpecResolvedRelativePeriod(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractRelativeAnchorTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	adapter := query.NewContractSourceAdapter(engine)
	spec := query.BuildQuerySpec("飞未云科（深圳）技术有限公司今年合同结算多少？", contractAnchor())
	if spec.PeriodFrom != "2026-01" || spec.PeriodTo != "2026-04" {
		t.Fatalf("resolved period = %s~%s, want 2026-01~2026-04", spec.PeriodFrom, spec.PeriodTo)
	}

	factSet, err := adapter.Fetch(context.Background(), spec)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	assertFactValue(t, factSet, "contract_book_settlement", 900)
	assertFactValue(t, factSet, "contract_cash_received", 900)
}

func TestContractSourceAdapterFallsBackToContractRelativeAnchorWhenSpecPeriodMissing(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractRelativeAnchorWithNewerJournalTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	adapter := query.NewContractSourceAdapter(engine)
	spec := query.QuerySpec{
		OriginalQuestion: "飞未云科（深圳）技术有限公司本月合同结算多少？",
		QueryFamily:      query.QueryFamilyContractDimension,
		Entity:           "飞未云科（深圳）技术有限公司",
	}

	factSet, err := adapter.Fetch(context.Background(), spec)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	assertFactValue(t, factSet, "contract_book_settlement", 900)
	if got := factSet.Facts[0].TracePayload["period"]; got != "2026-03" {
		t.Fatalf("trace period = %v, want 2026-03", got)
	}
}

func TestContractQueryExposesSourceBackedFactSets(t *testing.T) {
	runParallelHeavyQueryTest(t)

	dbPath := buildContractQueryTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("辽宁金程信息科技有限公司2025年合同结算多少？其中10月到账多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	factSets, ok := res.Data["fact_sets"].([]query.FactSet)
	if !ok || len(factSets) == 0 {
		t.Fatalf("fact_sets missing or empty: %#v", res.Data["fact_sets"])
	}
	if factSets[0].Source != "contracts" {
		t.Fatalf("fact set source = %s, want contracts", factSets[0].Source)
	}
	assertFactValue(t, factSets[0], "contract_book_settlement", 3000)
	assertFactValue(t, factSets[0], "contract_cash_received_subperiod", 1234)
}

func buildContractQueryTestDB(t *testing.T) string {
	t.Helper()

	return cloneSQLiteFixture(t, "contract-query", func(dbPath string) {
		buildContractQueryTestDBAt(t, dbPath)
	})
}

func buildContractQueryTestDBAt(t *testing.T, dbPath string) {
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
			contract_content TEXT,
			created_at TEXT
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
			account_code TEXT,
			created_at TEXT
		)`,
		`CREATE TABLE fin_cost_settlement_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			year_month TEXT,
			source_report_type TEXT,
			source_sheet_name TEXT,
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			customer_name TEXT,
			quantity TEXT,
			settlement_amount REAL,
			is_invoiced TEXT,
			invoice_amount REAL,
			paid_amount REAL,
			account_code TEXT,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			created_at TEXT
		)`,
		`CREATE TABLE fin_cost_settlement_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER,
			contract_id TEXT,
			source_row_number INTEGER,
			created_at TEXT
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
			invoice_amount REAL,
			created_at TEXT
		)`,
		`CREATE TABLE fin_fund_income_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			year_month TEXT,
			source_report_type TEXT,
			source_sheet_name TEXT,
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			customer_name TEXT,
			quantity TEXT,
			settlement_amount REAL,
			received_amount REAL,
			is_invoiced TEXT,
			invoice_amount REAL,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			created_at TEXT
		)`,
		`CREATE TABLE fin_fund_income_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER,
			contract_id TEXT,
			source_row_number INTEGER,
			created_at TEXT
		)`,
		`CREATE TABLE meta_table_comments (
			table_name TEXT PRIMARY KEY,
			comment TEXT,
			updated_at TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table failed: %v", err)
		}
	}

	inserts := []string{
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C001', '辽宁金程信息科技有限公司', '行业商品数据采购合同-A01')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C002', '南京林悦智能科技有限公司', '技术服务采购合同-LY01')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C003', '南京众信数通智能科技有限公司', '数据服务合同-ZX01')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C004', '飞未云科（深圳）技术有限公司', '全品类商品价格数据-京东')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C005', '飞未云科（深圳）技术有限公司', '全品类商品销量数据-京东')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C010', 'Yipit data', '数据采购合同-快手')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C011', 'Yipit data', '数据采购合同-抖音')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C012', 'Yipit data', '数据采购合同-淘宝')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C013', 'Yipit data', '数据采购合同-京东')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C020', '上海合并供应商科技有限公司', '外包服务合同-A')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C021', '上海合并供应商科技有限公司', '外包服务合同-B')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C022', '上海合并供应商科技有限公司', '外包服务合同-C')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C023', '上海合并供应商科技有限公司', '外包服务合同-D')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C001', '2025-10', 'contract_fund_income', '25年Q4收入明细', 1000, 1234, '是', 1000)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C001', '2025-11', 'contract_fund_income', '25年Q4收入明细', 2000, 1500, '是', 2000)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C003', '2025-10', 'contract_fund_income', '25年Q4收入明细', 300, 280, '是', 300)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C004', '2026-01', 'contract_fund_income', '26年Q1收入明细', 2500, 2500, '是', 2500)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C005', '2026-03', 'contract_fund_income', '26年Q1收入明细', 1100, 1100, '是', 1100)`,
		`INSERT INTO fin_fund_income_groups(id, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, customer_name, quantity, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES (101, '2026-01', 'contract_fund_income', '26年Q1收入明细', 24, 27, 'Yipit data', '/', 100, 90, '是', 100)`,
		`INSERT INTO fin_fund_income_groups(id, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, customer_name, quantity, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES (102, '2026-02', 'contract_fund_income', '26年Q1收入明细', 24, 27, 'Yipit data', '/', 200, 180, '是', 200)`,
		`INSERT INTO fin_fund_income_groups(id, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, customer_name, quantity, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES (103, '2026-03', 'contract_fund_income', '26年Q1收入明细', 24, 27, 'Yipit data', '/', 300, 300, '是', 300)`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number) VALUES
		 (101, 'C010', 24), (101, 'C011', 25), (101, 'C012', 26), (101, 'C013', 27),
		 (102, 'C010', 24), (102, 'C011', 25), (102, 'C012', 26), (102, 'C013', 27),
		 (103, 'C010', 24), (103, 'C011', 25), (103, 'C012', 26), (103, 'C013', 27)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, is_invoiced, account_code) VALUES ('C002', '2025-10', 'contract_revenue_cost', '成本-月度结算', '1人月', 888, '是', '640101')`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, is_invoiced, account_code) VALUES ('C003', '2025-10', 'contract_revenue_cost', '成本-月度结算', '1项', 120, '是', '640101')`,
		`INSERT INTO fin_cost_settlement_groups(id, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, customer_name, quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code)
		 VALUES (201, '2026-01', 'contract_revenue_cost', '成本-月度结算', 24, 27, '上海合并供应商科技有限公司', '/', 100, '是', 100, 90, '640101')`,
		`INSERT INTO fin_cost_settlement_groups(id, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, customer_name, quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code)
		 VALUES (202, '2026-02', 'contract_revenue_cost', '成本-月度结算', 24, 27, '上海合并供应商科技有限公司', '/', 200, '是', 200, 180, '640101')`,
		`INSERT INTO fin_cost_settlement_groups(id, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, customer_name, quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code)
		 VALUES (203, '2026-03', 'contract_revenue_cost', '成本-月度结算', 24, 27, '上海合并供应商科技有限公司', '/', 300, '是', 300, 300, '640101')`,
		`INSERT INTO fin_cost_settlement_group_members(group_id, contract_id, source_row_number) VALUES
		 (201, 'C020', 24), (201, 'C021', 25), (201, 'C022', 26), (201, 'C023', 27),
		 (202, 'C020', 24), (202, 'C021', 25), (202, 'C022', 26), (202, 'C023', 27),
		 (203, 'C020', 24), (203, 'C021', 25), (203, 'C022', 26), (203, 'C023', 27)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount) VALUES ('南京优集数据科技有限公司', '2025-10-18', '南京林悦智能科技有限公司', '合同付款', 666, 0)`,
		`INSERT INTO bank_statement(company, transaction_date, counterparty_name, summary, debit_amount, credit_amount) VALUES ('南京优集数据科技有限公司', '2025-10-22', '南京众信数通智能科技有限公司', '合同付款', 88, 0)`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_contracts', 'financeqa_source: {"display":"《合同信息表》","file_names":["优集资金收入计算表-副本.xlsx","优集成本计算表-4.23-池.xlsx"]}')`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_fund_income', 'financeqa_source: {"display":"《优集资金收入计算表-副本.xlsx》的【25年Q4收入明细】和【26年Q1收入明细】","file_names":["优集资金收入计算表-副本.xlsx"],"sheet_names":["25年Q4收入明细","26年Q1收入明细"]}')`,
		`INSERT INTO meta_table_comments(table_name, comment) VALUES ('fin_cost_settlements', 'financeqa_source: {"display":"《优集成本计算表-4.23-池.xlsx》","file_names":["优集成本计算表-4.23-池.xlsx"]}')`,
	}
	for _, stmt := range inserts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert seed data failed: %v", err)
		}
	}
}

func contractAnchor() time.Time {
	return time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
}

func buildContractRelativeAnchorTestDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "contract-relative-anchor.db")
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
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2025-12', '2025-12-31', 'J-OLD-1', '6001', '主营业务收入', '贷', 1, '旧账锚点', '旧客户', 0, 1)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C-FW-REL-1', '飞未云科（深圳）技术有限公司', '飞未云科项目-2026')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-REL-1', '2025-03', 'contract_fund_income', '25年Q1收入明细', 300, 300, '是', 300)`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-REL-1', '2026-03', 'contract_fund_income', '26年Q1收入明细', 900, 900, '是', 900)`,
	}
	for _, stmt := range inserts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert seed data failed: %v", err)
		}
	}

	return dbPath
}

func buildContractRelativeAnchorWithNewerJournalTestDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "contract-relative-anchor-newer-journal.db")
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
		`INSERT INTO journal(company, period, voucher_date, voucher_no, account_code, account_name, direction, amount, summary, counterparty, debit_amount, credit_amount)
		 VALUES ('南京优集数据科技有限公司', '2026-04', '2026-04-30', 'J-NEW-1', '6001', '主营业务收入', '贷', 1, '4月账务更新', '其他客户', 0, 1)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C-FW-REL-NEW-1', '飞未云科（深圳）技术有限公司', '飞未云科项目-2026')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount)
		 VALUES ('C-FW-REL-NEW-1', '2026-03', 'contract_fund_income', '26年Q1收入明细', 900, 900, '是', 900)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("insert seed data failed: %v", err)
		}
	}

	return dbPath
}
