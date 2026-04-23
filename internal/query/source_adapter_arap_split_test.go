package query

import (
	"testing"
	"time"
)

func TestBuildARAPOpenItemFactsUsesScopedMetricKeysForDualScopeQuestions(t *testing.T) {
	spec := BuildQuerySpec("南京林悦智能科技有限公司的应收/应付是多少？", time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC))
	spec.Entity = "南京林悦智能科技有限公司"
	scope := arapScope{
		typ:         "payable",
		accountName: "应付账款",
		codePrefix:  "2202",
		metricLabel: "arap",
	}
	data := map[string]any{
		"source":                    "journal_open_items",
		"total":                     float64(450),
		"historical_settlement":     float64(50),
		"current_period_settlement": float64(10),
	}

	facts := buildARAPOpenItemFacts(spec, scope, data)
	if len(facts) != 3 {
		t.Fatalf("facts len = %d, want 3", len(facts))
	}
	if facts[0].MetricKey != "openitem_payable_closing_total" {
		t.Fatalf("metric key = %s, want openitem_payable_closing_total", facts[0].MetricKey)
	}
	if facts[1].MetricKey != "openitem_payable_historical_settlement" {
		t.Fatalf("metric key = %s, want openitem_payable_historical_settlement", facts[1].MetricKey)
	}
	if facts[2].MetricKey != "openitem_payable_current_settlement" {
		t.Fatalf("metric key = %s, want openitem_payable_current_settlement", facts[2].MetricKey)
	}
}

func TestResolveARAPMetricLabelUsesGenericAndScopedKeys(t *testing.T) {
	single := BuildQuerySpec("2026年3月应付账款情况", time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC))
	singleScope := arapScope{typ: "payable", metricLabel: "arap"}
	if got := resolveARAPMetricLabel(single, singleScope); got != "arap" {
		t.Fatalf("single-scope label = %q, want %q", got, "arap")
	}

	dual := BuildQuerySpec("南京林悦智能科技有限公司的应收/应付是多少？", time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC))
	dualScope := arapScope{typ: "payable", metricLabel: "arap"}
	if got := resolveARAPMetricLabel(dual, dualScope); got != "payable" {
		t.Fatalf("dual-scope label = %q, want %q", got, "payable")
	}
}
