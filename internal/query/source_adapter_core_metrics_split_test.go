package query

import (
	"testing"

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
)

func TestBuildCoreMetricsMissingFactSet(t *testing.T) {
	spec := QuerySpec{
		Entity:     "飞未云科",
		PeriodFrom: "2026-04",
		PeriodTo:   "2026-04",
	}
	coverage := coreMetricCoverage{
		RequestedFrom: "2026-04",
		RequestedTo:   "2026-04",
		AvailableTo:   "2026-03",
		Truncated:     true,
		HasData:       false,
	}

	factSet := buildCoreMetricsMissingFactSet(spec, coverage)
	if factSet.Source != "core_metrics" {
		t.Fatalf("source = %s, want core_metrics", factSet.Source)
	}
	if len(factSet.Facts) != 1 {
		t.Fatalf("fact count = %d, want 1", len(factSet.Facts))
	}
	fact := factSet.Facts[0]
	if fact.MetricKey != "data_readiness" {
		t.Fatalf("metric key = %s, want data_readiness", fact.MetricKey)
	}
	if fact.CoverageStatus != CoverageMissing {
		t.Fatalf("coverage status = %s, want %s", fact.CoverageStatus, CoverageMissing)
	}
	trace := fact.TracePayload
	if trace["requested_from"] != "2026-04" || trace["available_to"] != "2026-03" {
		t.Fatalf("trace payload = %+v, want requested_from=2026-04 and available_to=2026-03", trace)
	}
}

func TestAttachCoreMetricsSnapshotTrace(t *testing.T) {
	factSet := FactSet{
		Source: "core_metrics",
		Facts: []Fact{
			{
				MetricKey:    "cash_receipts",
				TracePayload: map[string]any{"coverage": map[string]any{"requested_to": "2026-03"}},
			},
		},
	}
	snapshot := coreMetricDualSnapshot{
		Message: "2026-03 收入 100 元",
		Data:    map[string]any{"period": "2026-03", "money_value": float64(100)},
	}

	got := attachCoreMetricsSnapshotTrace(factSet, snapshot)
	trace := got.Facts[0].TracePayload
	if trace["result_message"] != snapshot.Message {
		t.Fatalf("result_message = %v, want %v", trace["result_message"], snapshot.Message)
	}
	resultData, ok := trace["result_data"].(map[string]any)
	if !ok {
		t.Fatalf("result_data missing: %+v", trace)
	}
	if resultData["period"] != "2026-03" || resultData["money_value"] != float64(100) {
		t.Fatalf("result_data = %+v, want period=2026-03 money_value=100", resultData)
	}
}

func TestBuildCoreMetricsFactSetIncludesBridgeAndTracePayload(t *testing.T) {
	spec := QuerySpec{
		OriginalQuestion: "2026年3月利润是多少？",
		Entity:           "飞未云科",
		PeriodFrom:       "2026-03",
		PeriodTo:         "2026-03",
	}
	coverage := coreMetricCoverage{
		RequestedFrom: "2026-03",
		RequestedTo:   "2026-03",
		ActualFrom:    "2026-03",
		ActualTo:      "2026-03",
		AvailableTo:   "2026-03",
		HasData:       true,
	}
	unified := &unifiedCoreMetrics{
		Period: "2026-03",
		Cash: accounting.CashPerspective{
			Income:  500,
			Expense: 800,
			Net:     -300,
		},
		Accrual: monthlyBookView{
			Revenue:   1000,
			TotalCost: 900,
			Profit:    100,
			NetProfit: 100,
		},
		AccrualFrom:       "income_statement",
		AccrualValidation: map[string]any{"basis": "income_statement"},
		Bridge: &analysis.ProfitCashBridge{
			AdjustedOperatingCashEstimate: 305,
			BankNetCash:                   -300,
		},
		Guard: map[string]any{"passed": true},
	}
	sqls := []string{"sql:a"}
	logs := []string{"log:a"}

	factSet := buildCoreMetricsFactSet(spec, coverage, unified, sqls, logs)
	assertCoreMetricFactValue(t, factSet, "cash_receipts", 500)
	assertCoreMetricFactValue(t, factSet, "accrual_revenue", 1000)
	assertCoreMetricFactValue(t, factSet, "cash_bridge_adjusted_operating_cash", 305)
	assertCoreMetricFactValue(t, factSet, "cash_bridge_bank_net_cash", -300)

	trace := factSet.Facts[0].TracePayload
	if trace["accrual_source"] != "income_statement" {
		t.Fatalf("accrual_source = %v, want income_statement", trace["accrual_source"])
	}
	coveragePayload, ok := trace["coverage"].(map[string]any)
	if !ok {
		t.Fatalf("coverage payload missing: %+v", trace)
	}
	if coveragePayload["actual_from"] != "2026-03" || coveragePayload["available_to"] != "2026-03" {
		t.Fatalf("coverage payload = %+v, want actual_from/available_to 2026-03", coveragePayload)
	}
}

func assertCoreMetricFactValue(t *testing.T, factSet FactSet, metricKey string, want float64) {
	t.Helper()
	for _, fact := range factSet.Facts {
		if fact.MetricKey == metricKey {
			if fact.Value != want {
				t.Fatalf("%s value = %v, want %v", metricKey, fact.Value, want)
			}
			return
		}
	}
	t.Fatalf("metricKey %s not found in facts: %+v", metricKey, factSet.Facts)
}
