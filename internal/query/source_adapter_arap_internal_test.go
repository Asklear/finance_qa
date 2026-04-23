package query

import (
	"testing"
	"time"
)

func TestBuildARAPFactSetFromQueryResultUsesOfficialAndOpenItemData(t *testing.T) {
	spec := BuildQuerySpec("2026年3月应付账款情况", time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC))
	result := Result{
		Success: true,
		Data: map[string]any{
			"source":          "balance_sheet",
			"total":           float64(1200),
			"opening_balance": float64(1000),
			"open_item_analysis": map[string]any{
				"source":                    "journal_open_items",
				"total":                     float64(200),
				"historical_settlement":     float64(0),
				"current_period_settlement": float64(0),
			},
		},
	}

	factSet, ok := buildARAPFactSetFromQueryResult(spec, result)
	if !ok {
		t.Fatalf("expected fact set from query result")
	}
	assertInternalFactValue(t, factSet, "official_arap_total", 1200)
	assertInternalFactValue(t, factSet, "official_arap_opening_balance", 1000)
	assertInternalFactValue(t, factSet, "openitem_closing_total", 200)
}

func TestBuildARAPFactSetFromEntityQueryResultIncludesReceivableAndPayable(t *testing.T) {
	spec := BuildQuerySpec("南京林悦智能科技有限公司的应收/应付是多少？", time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC))
	spec.Entity = "南京林悦智能科技有限公司"
	result := Result{
		Success: true,
		Data: map[string]any{
			"receivable": map[string]any{
				"source":                    "journal_open_items",
				"total":                     float64(300),
				"historical_settlement":     float64(100),
				"current_period_settlement": float64(0),
			},
			"payable": map[string]any{
				"source":          "balance_sheet",
				"total":           float64(500),
				"opening_balance": float64(200),
				"open_item_analysis": map[string]any{
					"source":                    "journal_open_items",
					"total":                     float64(450),
					"historical_settlement":     float64(50),
					"current_period_settlement": float64(0),
				},
			},
		},
	}

	factSet, ok := buildARAPFactSetFromQueryResult(spec, result)
	if !ok {
		t.Fatalf("expected entity arap fact set from query result")
	}
	assertInternalFactValue(t, factSet, "openitem_receivable_closing_total", 300)
	assertInternalFactValue(t, factSet, "official_payable_total", 500)
	assertInternalFactValue(t, factSet, "official_payable_opening_balance", 200)
	assertInternalFactValue(t, factSet, "openitem_payable_closing_total", 450)
}

func assertInternalFactValue(t *testing.T, factSet FactSet, metricKey string, want float64) {
	t.Helper()
	for _, fact := range factSet.Facts {
		if fact.MetricKey == metricKey {
			if fact.Value != want {
				t.Fatalf("%s = %.2f, want %.2f", metricKey, fact.Value, want)
			}
			return
		}
	}
	t.Fatalf("metric %s not found in fact set: %+v", metricKey, factSet.Facts)
}
