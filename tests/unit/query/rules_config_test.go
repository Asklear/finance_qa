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

