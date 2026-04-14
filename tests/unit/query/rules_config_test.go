package query_test

import (
	"strings"
	"testing"

	"financeqa/internal/query"
)

func TestMetricStopwordsCanBeConfiguredByEnv(t *testing.T) {
	t.Setenv("FINANCEQA_METRIC_STOPWORDS", "收入,成本,利润,飞未")

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
	if strings.Contains(res.Message, "[飞未]") {
		t.Fatalf("custom metric stopwords should block entity route, got: %s", res.Message)
	}
}

func TestCounterpartyRoleThresholdCanBeConfiguredByEnv(t *testing.T) {
	t.Setenv("FINANCEQA_ROLE_MIN_PRIMARY_SCORE", "10")

	evidence := []query.LedgerEvidence{
		{Source: "bank_statement", Counterparty: "金程", CreditAmount: 1130, Summary: "历史应收回款"},
		{Source: "journal", Counterparty: "金程", AccountCode: "1122", AccountName: "应收账款", Summary: "回款冲销"},
		{Source: "journal", Counterparty: "金程", AccountCode: "6001", AccountName: "主营业务收入", Summary: "销售收入"},
	}
	got := query.ClassifyCounterparty("金程", evidence)
	if got.Role != query.CounterpartyUnknown {
		t.Fatalf("expected unknown when threshold is high, got role=%s score=%v confidence=%.3f", got.Role, got.Scores, got.Confidence)
	}
}

func TestIntentKeywordsCanBeConfiguredByEnv(t *testing.T) {
	t.Setenv("FINANCEQA_INTENT_ARAP_KEYWORDS", "供应商账款,往来余额")

	if got := query.ClassifyIntent("供应商账款情况"); got != query.IntentARAPQuery {
		t.Fatalf("ClassifyIntent with custom arap keywords = %s, want %s", got, query.IntentARAPQuery)
	}
}

func TestHighFrequencyIntentKeywordsCanBeConfiguredByEnv(t *testing.T) {
	t.Setenv("FINANCEQA_INTENT_HR_COST_KEYWORDS", "人员费用")
	if got := query.ClassifyIntent("2026年2月人员费用多少"); got != query.IntentFallback {
		t.Fatalf("ClassifyIntent with custom hr keywords = %s, want %s", got, query.IntentFallback)
	}

	t.Setenv("FINANCEQA_INTENT_TAX_KEYWORDS", "销项")
	if got := query.ClassifyIntent("2026年2月销项多少"); got != query.IntentTaxQuery {
		t.Fatalf("ClassifyIntent with custom tax keywords = %s, want %s", got, query.IntentTaxQuery)
	}

	t.Setenv("FINANCEQA_INTENT_HEALTH_KEYWORDS", "稳不稳")
	if got := query.ClassifyIntent("公司现在稳不稳"); got != query.IntentFallback {
		t.Fatalf("ClassifyIntent with custom health keywords = %s, want %s", got, query.IntentFallback)
	}
}

func TestFallbackHRCostKeywordsCanBeConfiguredByEnv(t *testing.T) {
	t.Setenv("FINANCEQA_INTENT_HR_COST_KEYWORDS", "人员费用")

	dbPath := buildEntityRoutingTestDB(t)
	engine, err := query.NewEngine(dbPath, testCompany)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月人员费用多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if total, ok := res.Data["total"].(float64); !ok || total != 300 {
		t.Fatalf("custom hr keyword should route to hr cost fallback, got %v", res.Data["total"])
	}
}
