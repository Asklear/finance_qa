package query_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/query"
)

func writeRulesConfigFile(t *testing.T, content string) string {
	t.Helper()

	rulesPath := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(rulesPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}
	return rulesPath
}

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

func TestRulesConfigLoadsNestedSchema(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "intents": {
      "arap": {
        "keywords": ["往来余额", "挂账余额"],
        "priority": 180,
        "min_confidence": 0.72,
        "conflicts": ["fallback"],
        "high_priority_phrases": ["应收挂账"]
      }
    }
  },
  "counterparty": {
    "thresholds": {
      "mixed_min_ratio": 0.33
    }
  }
}`)

	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	cfg := query.CurrentRuleConfig()
	if got := cfg.IntentARAPKeywords; len(got) != 2 || got[0] != "往来余额" || got[1] != "挂账余额" {
		t.Fatalf("nested arap keywords not loaded, got=%v", got)
	}
	if got := cfg.HighPriorityPhrases["arap"]; len(got) != 1 || got[0] != "应收挂账" {
		t.Fatalf("nested high_priority_phrases not loaded, got=%v", got)
	}
	if got := cfg.IntentPriority["arap"]; got != 180 {
		t.Fatalf("nested intent_priority not loaded, got=%d", got)
	}
	if got := cfg.IntentConflicts["arap"]; len(got) != 1 || got[0] != "fallback" {
		t.Fatalf("nested intent_conflicts not loaded, got=%v", got)
	}
	if got := cfg.IntentMinConfidence["arap"]; got != 0.72 {
		t.Fatalf("nested intent_min_confidence not loaded, got=%v", got)
	}
	if cfg.RoleMixedMinRatio != 0.33 {
		t.Fatalf("nested role threshold not loaded, got=%v", cfg.RoleMixedMinRatio)
	}
}

func TestRulesConfigStillLoadsLegacyFlatSchema(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "high_priority_phrases": {
    "arap": ["预收款", "应收账款"]
  },
  "intent_priority": {
    "arap": 100,
    "fallback": 10
  },
  "intent_conflicts": {
    "arap": ["fallback"]
  },
  "intent_min_confidence": {
    "arap": 0.65
  }
}`)

	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)
	t.Setenv("FINANCEQA_INTENT_PRIORITY", `{"arap":180}`)
	t.Setenv("FINANCEQA_INTENT_MIN_CONFIDENCE", `{"arap":0.7}`)

	cfg := query.CurrentRuleConfig()
	if got := cfg.HighPriorityPhrases["arap"]; len(got) != 2 || got[0] != "预收款" || got[1] != "应收账款" {
		t.Fatalf("high_priority_phrases not loaded, got=%v", got)
	}
	if got := cfg.IntentPriority["arap"]; got != 180 {
		t.Fatalf("intent_priority env override not effective, got=%d", got)
	}
	if got := cfg.IntentConflicts["arap"]; len(got) != 1 || got[0] != "fallback" {
		t.Fatalf("intent_conflicts not loaded, got=%v", got)
	}
	if got := cfg.IntentMinConfidence["arap"]; got != 0.7 {
		t.Fatalf("intent_min_confidence env override not effective, got=%v", got)
	}
}

func TestRuleLexiconAccessors(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "intents": {
      "large_transaction": {"keywords": ["峰值来款"]},
      "identity": {"keywords": ["什么来头"]},
      "precise": {"keywords": ["结余存量"]}
    },
    "metric_keywords": {
      "profit": ["净赚"]
    },
    "hr_breakdown_keywords": ["细拆"],
    "counterparty_classification_question_keywords": ["归类判断"],
    "profit_single_view_block_keywords": ["不一致"]
  },
  "counterparty": {
    "roles": {
      "customer": ["客证"],
      "supplier": ["供证"],
      "employee": ["员证"]
    },
    "tax": {
      "output": ["销证"],
      "input": ["进证"]
    }
  },
  "internal_party": {
    "org_suffixes": ["中心"],
    "account_context_keywords": ["内部代发"]
  }
}`)

	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	cfg := query.CurrentRuleConfig()
	if got := cfg.IntentKeywords(query.IntentLargeTransactionQuery); len(got) != 1 || got[0] != "峰值来款" {
		t.Fatalf("intent lexicon accessor large_transaction = %v", got)
	}
	if got := cfg.IntentKeywords(query.IntentIdentityQuery); len(got) != 1 || got[0] != "什么来头" {
		t.Fatalf("intent lexicon accessor identity = %v", got)
	}
	if got := cfg.IntentKeywords(query.IntentPrecise); len(got) != 1 || got[0] != "结余存量" {
		t.Fatalf("intent lexicon accessor precise = %v", got)
	}
	if got := cfg.MetricKeywords("profit"); len(got) != 1 || got[0] != "净赚" {
		t.Fatalf("metric lexicon accessor profit = %v", got)
	}
	if got := cfg.HRBreakdownKeywords(); len(got) != 1 || got[0] != "细拆" {
		t.Fatalf("hr breakdown lexicon accessor = %v", got)
	}
	if got := cfg.CounterpartyClassificationQuestionKeywords(); len(got) != 1 || got[0] != "归类判断" {
		t.Fatalf("counterparty classification lexicon accessor = %v", got)
	}
	if got := cfg.ProfitSingleViewBlockKeywords(); len(got) != 1 || got[0] != "不一致" {
		t.Fatalf("profit single view block lexicon accessor = %v", got)
	}
	if got := cfg.CounterpartyRoleKeywords(query.CounterpartyCustomer); len(got) != 1 || got[0] != "客证" {
		t.Fatalf("counterparty customer lexicon accessor = %v", got)
	}
	if got := cfg.CounterpartyRoleKeywords(query.CounterpartySupplier); len(got) != 1 || got[0] != "供证" {
		t.Fatalf("counterparty supplier lexicon accessor = %v", got)
	}
	if got := cfg.CounterpartyRoleKeywords(query.CounterpartyEmployee); len(got) != 1 || got[0] != "员证" {
		t.Fatalf("counterparty employee lexicon accessor = %v", got)
	}
	if got := cfg.TaxKeywords(query.TaxSideOutput); len(got) != 1 || got[0] != "销证" {
		t.Fatalf("tax output lexicon accessor = %v", got)
	}
	if got := cfg.TaxKeywords(query.TaxSideInput); len(got) != 1 || got[0] != "进证" {
		t.Fatalf("tax input lexicon accessor = %v", got)
	}
	if got := cfg.InternalPartyOrgSuffixes(); len(got) != 1 || got[0] != "中心" {
		t.Fatalf("internal party org suffix accessor = %v", got)
	}
	if got := cfg.InternalPartyAccountContextKeywords(); len(got) != 1 || got[0] != "内部代发" {
		t.Fatalf("internal party context keyword accessor = %v", got)
	}
}
