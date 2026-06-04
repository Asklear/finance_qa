package query

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFinalizeQueryResultInjectsTraceSpecAndSourceAttribution(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "query-finalize.sqlite")
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	ctx := queryExecutionContext{
		engine: engine,
		traceMap: map[string]any{
			"router_version": "v2",
			"final_intent":   IntentFallback,
		},
		spec: QuerySpec{
			QueryFamily:        QueryFamilySupplierPayments,
			MetricKind:         MetricKindCost,
			PeriodFrom:         "2026-03",
			PeriodTo:           "2026-03",
			PerspectivePolicy:  PerspectiveCashThenAccrual,
			OpeningPeriodAware: true,
			LexiconProfile:     "rules_config",
		},
	}

	res := finalizeQueryResult(ctx, Result{Success: true, Message: "供应商付款统计完成"})
	if res.Data == nil {
		t.Fatalf("expected data envelope to be initialized")
	}
	if _, ok := res.Data["intent_trace"].(map[string]any); !ok {
		t.Fatalf("intent_trace missing: %+v", res.Data)
	}
	querySpec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := querySpec["query_family"]; got != QueryFamilySupplierPayments {
		t.Fatalf("query_family = %v, want %v", got, QueryFamilySupplierPayments)
	}
	if got := querySpec["opening_period_aware"]; got != true {
		t.Fatalf("opening_period_aware = %v, want true", got)
	}
	if got := res.Data["answer_method"]; got != "sql" {
		t.Fatalf("answer_method = %v, want sql", got)
	}
	sourceNote, _ := res.Data["source_note"].(string)
	if !strings.Contains(sourceNote, "《银行流水》") {
		t.Fatalf("source_note = %q, want bank statement attribution", sourceNote)
	}
	if !strings.Contains(res.Message, "来源：") {
		t.Fatalf("message should append source note, got: %s", res.Message)
	}
}

func TestFinalizeQueryResultCarriesSemanticFamiliesOverride(t *testing.T) {
	ctx := queryExecutionContext{
		spec: QuerySpec{
			QueryFamily: QueryFamilyGeneral,
			PeriodFrom:  "2026-01",
			PeriodTo:    "2026-03",
			TimeScope:   TimeScopeQuarter,
		},
	}

	res := finalizeQueryResult(ctx, Result{
		Success: true,
		Message: "现金余额已计算",
		Data: map[string]any{
			"query_spec_overrides": map[string]any{
				"semantic_families": []string{"cash_balance", "bank_cash_flow", "balance_sheet"},
			},
		},
	})
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	families := anySourceStringSlice(spec["semantic_families"])
	for _, want := range []string{"cash_balance", "bank_cash_flow", "balance_sheet"} {
		if !containsString(families, want) {
			t.Fatalf("semantic_families = %#v, want %q", families, want)
		}
	}
}
