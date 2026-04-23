package query

import (
	"strings"
	"testing"
	"time"
)

func TestExtractFrameTraceStringsAggregatesAcrossAllFacts(t *testing.T) {
	factSets := []FactSet{
		{
			Source: "core_metrics",
			Facts: []Fact{
				{
					MetricKey: "cash_receipts",
					TracePayload: map[string]any{
						"executed_sql": []string{"sql-1"},
					},
				},
				{
					MetricKey: "accrual_revenue",
					TracePayload: map[string]any{
						"executed_sql": []string{"sql-2", "sql-1"},
					},
				},
			},
		},
	}

	got := extractFrameTraceStrings(factSets, "executed_sql")
	if len(got) != 2 || got[0] != "sql-1" || got[1] != "sql-2" {
		t.Fatalf("extractFrameTraceStrings() = %#v, want [sql-1 sql-2]", got)
	}
}

func TestComposeARAPResultUsesSingleScopeEntityRollforwardForEntityPayableQuestion(t *testing.T) {
	spec := BuildQuerySpec("南京林悦智能科技有限公司3月应付账款多少？", time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC))
	spec.Entity = "南京林悦智能科技有限公司"
	spec.QueryFamily = QueryFamilyARAP
	spec.AuthoritativeSourceRequired = true
	spec.OpeningPeriodAware = true

	frame := AnswerFrame{
		Spec: spec,
		Plan: QueryPlan{
			QueryFamily:  QueryFamilyARAP,
			Capabilities: []SourceCapability{SourceCapabilityOfficialARAP, SourceCapabilityOpenItemEvidence},
		},
		FactSets: []FactSet{
			{
				Source: "arap",
				Facts: []Fact{
					{
						MetricKey: "official_arap_total",
						Value:     2530000,
						TracePayload: map[string]any{
							"result_data": map[string]any{
								"account":                 "应付账款",
								"entity":                  "南京林悦智能科技有限公司",
								"period":                  "2026-03",
								"source":                  "journal_entity_rollforward",
								"type":                    "payable",
								"total":                   2530000,
								"opening_balance":         4435815,
								"current_increase":        10100.19,
								"current_decrease":        1915915.19,
								"open_item_closing_total": 1450100.19,
							},
						},
					},
				},
			},
		},
	}

	result, err := composeARAPResult(frame)
	if err != nil {
		t.Fatalf("composeARAPResult() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("composeARAPResult() success = false")
	}
	if got := anyToFloat64(result.Data["total"]); got != 2530000 {
		t.Fatalf("total = %.2f, want 2530000.00", got)
	}
	if got := anyToString(result.Data["account"]); got != "应付账款" {
		t.Fatalf("account = %q, want 应付账款", got)
	}
	for _, want := range []string{"2530000.00", "4435815.00", "10100.19", "1915915.19"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message = %q, want substring %q", result.Message, want)
		}
	}
}
