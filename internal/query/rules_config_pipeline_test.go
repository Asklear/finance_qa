package query

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRuleConfigAppliesDefaultThenFileThenEnv(t *testing.T) {
	rulesPath := filepath.Join(t.TempDir(), "rules.json")
	content := `{
  "schema_version": 2,
  "router": {
    "intents": {
      "arap": {
        "keywords": ["文件应收", "文件挂账"],
        "priority": 130
      }
    }
  },
  "reconciliation": {
    "residual_gap_escalation_amount": 880000
  },
  "counterparty": {
    "thresholds": {
      "min_confidence": 0.22
    }
  }
}`
	if err := os.WriteFile(rulesPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	env := map[string]string{
		"FINANCEQA_INTENT_ARAP_KEYWORDS":                          "环境应收,环境挂账",
		"FINANCEQA_INTENT_PRIORITY":                               `{"arap":180}`,
		"FINANCEQA_ROLE_MIN_CONFIDENCE":                           "0.45",
		"FINANCEQA_RECONCILIATION_RESIDUAL_GAP_ESCALATION_AMOUNT": "120000",
	}
	cfg := buildRuleConfig(rulesPath, func(key string) string {
		return env[key]
	})

	if got := cfg.IntentARAPKeywords; len(got) != 2 || got[0] != "环境应收" || got[1] != "环境挂账" {
		t.Fatalf("IntentARAPKeywords = %v, want env override", got)
	}
	if got := cfg.IntentPriority["arap"]; got != 180 {
		t.Fatalf("IntentPriority[arap] = %d, want env override 180", got)
	}
	if got := cfg.RoleMinConfidence; got != 0.45 {
		t.Fatalf("RoleMinConfidence = %v, want env override 0.45", got)
	}
	if got := cfg.ReconciliationResidualGapEscalationThreshold(); got != 120000 {
		t.Fatalf("ReconciliationResidualGapEscalationThreshold = %v, want env override 120000", got)
	}
	if got := cfg.ContractPriorityKeywords(); len(got) == 0 {
		t.Fatalf("default contract priority keywords should remain available")
	}
}
