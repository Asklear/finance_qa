package query

import (
	"strings"
	"testing"
)

func TestContractAggregateFallbackReasonMentionsMissingCoverage(t *testing.T) {
	summary := contractAggregateSummary{
		RequestedMetrics:   []string{"利润"},
		HasRevenueCoverage: true,
		HasCostCoverage:    false,
	}

	reason := contractAggregateFallbackReason(summary.RequestedMetrics, summary)
	if !strings.Contains(reason, "项目成本") {
		t.Fatalf("fallback reason should mention missing project cost, got: %s", reason)
	}
	if strings.Contains(reason, "已回退") || strings.Contains(reason, "现金+经营/财务") {
		t.Fatalf("fallback reason should not claim an automatic fallback, got: %s", reason)
	}
}

func TestBuildContractAggregateResultSnapshotIncludesCashAndAccountViews(t *testing.T) {
	spec := QuerySpec{
		OriginalQuestion: "2025年10月收入、成本、利润分别是多少？",
	}
	summary := contractAggregateSummary{
		Period:            "2025-10",
		RequestedMetrics:  []string{"收入", "成本", "利润"},
		RevenueSettlement: 1300,
		RevenueReceived:   1200,
		RevenueInvoiced:   1180,
		CostSettlement:    1008,
		Profit:            292,
		SourceTables:      []string{"tenant_uhub.fin_contracts", "tenant_uhub.fin_fund_income", "tenant_uhub.fin_cost_settlements"},
	}

	message, data := buildContractAggregateResultSnapshot(spec, summary)
	if !strings.Contains(message, "老板口径先看项目汇总") {
		t.Fatalf("message should disclose project aggregate priority, got: %s", message)
	}
	if !strings.Contains(message, "补充项目现金回款 1200.00 元") {
		t.Fatalf("message should disclose project cash receipts, got: %s", message)
	}
	moneyView, ok := data["money_view"].(map[string]any)
	if !ok {
		t.Fatalf("money_view missing: %+v", data)
	}
	if moneyView["回款"] != float64(1200) {
		t.Fatalf("money_view[回款] = %v, want 1200", moneyView["回款"])
	}
	if moneyView["净现金"] != float64(1200) {
		t.Fatalf("money_view[净现金] = %v, want 1200", moneyView["净现金"])
	}
	accountView, ok := data["account_view"].(map[string]any)
	if !ok {
		t.Fatalf("account_view missing: %+v", data)
	}
	if accountView["利润"] != float64(292) {
		t.Fatalf("account_view[利润] = %v, want 292", accountView["利润"])
	}
	if accountView["营收"] != float64(1300) {
		t.Fatalf("account_view[营收] = %v, want 1300", accountView["营收"])
	}
	if got := data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first", got)
	}
}

func TestBuildContractAggregateResultSnapshotUsesProjectLabelsForCompanyAggregate(t *testing.T) {
	spec := QuerySpec{
		OriginalQuestion: "从2025年10月到2026年5月底，所有项目的应收未收、应付未付是多少？",
	}
	summary := contractAggregateSummary{
		Period:            "2025-10~2026-05",
		RequestedMetrics:  []string{"收入", "成本", "应收", "应付"},
		RevenueSettlement: 25321196.46,
		RevenueReceived:   19959700.89,
		RevenueInvoiced:   20083501.96,
		RevenueReceivable: 5361495.57,
		CostSettlement:    11159793.22,
		CostPaid:          8911445.76,
		CostPayable:       2249454.49,
		SourceTables:      []string{"tenant_uhub.fin_contracts", "tenant_uhub.fin_fund_income", "tenant_uhub.fin_cost_settlements"},
	}

	message, data := buildContractAggregateResultSnapshot(spec, summary)
	for _, want := range []string{"老板口径先看项目汇总", "项目结算收入（营收） 25321196.46 元", "项目成本 11159793.22 元", "项目应收（应收未收） 5361495.57 元", "项目应付（应付未付/未付款） 2249454.49 元"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message should include %q, got: %s", want, message)
		}
	}
	for _, forbidden := range []string{"老板口径先看合同/项目汇总", "合同结算", "合同成本", "合同应收", "合同应付"} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("message should not expose %q, got: %s", forbidden, message)
		}
	}
	accountView, ok := data["account_view"].(map[string]any)
	if !ok {
		t.Fatalf("account_view missing: %#v", data["account_view"])
	}
	for _, want := range []string{"项目结算", "项目成本", "项目应收", "项目应付"} {
		if _, ok := accountView[want]; !ok {
			t.Fatalf("account_view should include %s, got: %#v", want, accountView)
		}
	}
	for _, forbidden := range []string{"合同成本", "合同应收", "合同应付"} {
		if _, ok := accountView[forbidden]; ok {
			t.Fatalf("account_view should not expose %s, got: %#v", forbidden, accountView)
		}
	}
}

func TestBuildContractAggregateResultSnapshotRevenueOnlyOmitsPlaceholderCostAndProfit(t *testing.T) {
	spec := QuerySpec{
		OriginalQuestion: "2025年10月收入是多少？",
	}
	summary := contractAggregateSummary{
		Period:            "2025-10",
		RequestedMetrics:  []string{"收入"},
		RevenueSettlement: 1300,
		RevenueReceived:   1200,
		RevenueInvoiced:   1180,
		CostSettlement:    1008,
		CostPaid:          900,
		Profit:            292,
		SourceTables:      []string{"tenant_uhub.fin_contracts", "tenant_uhub.fin_fund_income"},
	}

	message, data := buildContractAggregateResultSnapshot(spec, summary)
	if strings.Contains(message, "项目成本") || strings.Contains(message, "利润") {
		t.Fatalf("revenue-only snapshot message should not mention cost/profit, got: %s", message)
	}
	metrics, ok := data["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics missing: %#v", data["metrics"])
	}
	if len(metrics) != 1 || metrics["收入"] != float64(1300) {
		t.Fatalf("metrics = %#v, want only 收入=1300", metrics)
	}
	if _, ok := metrics["成本"]; ok {
		t.Fatalf("metrics should omit 成本 for revenue-only query: %#v", metrics)
	}
	if _, ok := metrics["利润"]; ok {
		t.Fatalf("metrics should omit 利润 for revenue-only query: %#v", metrics)
	}
	accountView, ok := data["account_view"].(map[string]any)
	if !ok {
		t.Fatalf("account_view missing: %#v", data["account_view"])
	}
	if _, ok := accountView["项目成本"]; ok {
		t.Fatalf("account_view should omit 项目成本 for revenue-only query: %#v", accountView)
	}
	if _, ok := accountView["利润"]; ok {
		t.Fatalf("account_view should omit 利润 for revenue-only query: %#v", accountView)
	}
	if accountView["项目结算"] != float64(1300) || accountView["已开票"] != float64(1180) {
		t.Fatalf("account_view = %#v, want 项目结算=1300 and 已开票=1180", accountView)
	}
	moneyView, ok := data["money_view"].(map[string]any)
	if !ok {
		t.Fatalf("money_view missing: %#v", data["money_view"])
	}
	if moneyView["到账"] != float64(1200) {
		t.Fatalf("money_view = %#v, want 到账=1200", moneyView)
	}
	if _, ok := moneyView["付款"]; ok {
		t.Fatalf("money_view should omit 付款 for revenue-only query: %#v", moneyView)
	}
	contractSummary, ok := data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %#v", data["contract_summary"])
	}
	if _, ok := contractSummary["cost_settlement"]; ok {
		t.Fatalf("contract_summary should omit cost_settlement for revenue-only query: %#v", contractSummary)
	}
	if _, ok := contractSummary["profit"]; ok {
		t.Fatalf("contract_summary should omit profit for revenue-only query: %#v", contractSummary)
	}
}

func TestBuildContractAggregateResultSnapshotCostOnlyUsesPaidCashView(t *testing.T) {
	spec := QuerySpec{
		OriginalQuestion: "2025年10月成本是多少？",
	}
	summary := contractAggregateSummary{
		Period:            "2025-10",
		RequestedMetrics:  []string{"成本"},
		RevenueSettlement: 1300,
		RevenueReceived:   1200,
		RevenueInvoiced:   1180,
		CostSettlement:    1008,
		CostPaid:          900,
		Profit:            292,
		SourceTables:      []string{"tenant_uhub.fin_contracts", "tenant_uhub.fin_cost_settlements"},
	}

	message, data := buildContractAggregateResultSnapshot(spec, summary)
	if strings.Contains(message, "现金回款") {
		t.Fatalf("cost-only snapshot message should not mention cash receipts, got: %s", message)
	}
	if !strings.Contains(message, "项目现金付款 900.00 元") {
		t.Fatalf("cost-only snapshot message should mention project payment, got: %s", message)
	}
	moneyView, ok := data["money_view"].(map[string]any)
	if !ok {
		t.Fatalf("money_view missing: %#v", data["money_view"])
	}
	if moneyView["付款"] != float64(900) {
		t.Fatalf("money_view = %#v, want 付款=900", moneyView)
	}
	if _, ok := moneyView["到账"]; ok {
		t.Fatalf("money_view should omit 到账 for cost-only query: %#v", moneyView)
	}
	accountView, ok := data["account_view"].(map[string]any)
	if !ok {
		t.Fatalf("account_view missing: %#v", data["account_view"])
	}
	if accountView["项目成本"] != float64(1008) {
		t.Fatalf("account_view = %#v, want 项目成本=1008", accountView)
	}
	if _, ok := accountView["项目结算"]; ok {
		t.Fatalf("account_view should omit 项目结算 for cost-only query: %#v", accountView)
	}
	if _, ok := accountView["利润"]; ok {
		t.Fatalf("account_view should omit 利润 for cost-only query: %#v", accountView)
	}
}

func TestBuildContractAggregateFactSetRevenueOnlyKeepsRelevantFacts(t *testing.T) {
	spec := QuerySpec{
		OriginalQuestion: "2025年10月收入是多少？",
		PeriodFrom:       "2025-10",
		PeriodTo:         "2025-10",
	}
	summary := contractAggregateSummary{
		Period:             "2025-10",
		RequestedMetrics:   []string{"收入"},
		RevenueSettlement:  1300,
		RevenueReceived:    1200,
		RevenueInvoiced:    1180,
		HasRevenueCoverage: true,
		SourceTables:       []string{"tenant_uhub.fin_contracts", "tenant_uhub.fin_fund_income"},
	}

	factSet := buildContractAggregateFactSet(spec, summary)
	if len(factSet.Facts) != 2 {
		t.Fatalf("fact count = %d, want 2", len(factSet.Facts))
	}
	keys := map[string]struct{}{}
	for _, fact := range factSet.Facts {
		keys[fact.MetricKey] = struct{}{}
	}
	if _, ok := keys["contract_aggregate_revenue"]; !ok {
		t.Fatalf("fact keys should include contract_aggregate_revenue: %#v", keys)
	}
	if _, ok := keys["contract_aggregate_cash_received"]; !ok {
		t.Fatalf("fact keys should include contract_aggregate_cash_received: %#v", keys)
	}
	if _, ok := keys["contract_aggregate_cost"]; ok {
		t.Fatalf("fact keys should omit contract_aggregate_cost: %#v", keys)
	}
	if _, ok := keys["contract_aggregate_profit"]; ok {
		t.Fatalf("fact keys should omit contract_aggregate_profit: %#v", keys)
	}
}
