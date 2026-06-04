package query

import "testing"

func TestResolveContractAggregateSelectionUsesRequestedMetricsAndPrimaryMetric(t *testing.T) {
	spec := QuerySpec{
		OriginalQuestion: "2025年10月利润是多少？",
	}
	summary := contractAggregateSummary{
		RequestedMetrics: []string{"利润"},
	}

	selection := resolveContractAggregateSelection(spec, summary)
	if selection.PrimaryMetric != "利润" {
		t.Fatalf("PrimaryMetric = %s, want 利润", selection.PrimaryMetric)
	}
	if !selection.IncludeProfit {
		t.Fatalf("IncludeProfit = false, want true")
	}
	if selection.IncludeRevenue {
		t.Fatalf("IncludeRevenue = true, want false")
	}
	if selection.IncludeCost {
		t.Fatalf("IncludeCost = true, want false")
	}
}

func TestBuildContractAggregateScopeLabelIncludesEntityWhenPresent(t *testing.T) {
	noEntity := contractAggregateSummary{
		Period: "2025-10",
	}
	if got := buildContractAggregateScopeLabel(noEntity); got != "2025-10 老板口径先看项目汇总" {
		t.Fatalf("scope label without entity = %s", got)
	}

	withEntity := contractAggregateSummary{
		Entity: "飞未云科",
		Period: "2025-10",
	}
	if got := buildContractAggregateScopeLabel(withEntity); got != "[飞未云科] 2025-10 老板口径先看项目汇总" {
		t.Fatalf("scope label with entity = %s", got)
	}
}
