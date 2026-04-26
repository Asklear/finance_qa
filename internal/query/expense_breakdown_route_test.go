package query

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldUseExpenseBreakdownRecognizesFlexibleBossPhrasing(t *testing.T) {
	yes := []string{
		"2026年3月整体支出按大类拆一下",
		"3月钱都花哪了，按类别看",
		"2026年3月费用构成",
		"本月成本按大类拆开",
		"3月付款分类看一下",
		"2026年3月整体支出按合同拆一下",
		"2026年3月花销掰开看看",
	}
	for _, q := range yes {
		if !shouldUseExpenseBreakdown(q) {
			t.Fatalf("shouldUseExpenseBreakdown(%q) = false, want true", q)
		}
	}

	no := []string{
		"2026年3月收入、成本、利润分别是多少",
		"2026年3月利润是多少",
		"2026年3月收入按客户拆分",
		"2026年3月人力成本细拆",
	}
	for _, q := range no {
		if shouldUseExpenseBreakdown(q) {
			t.Fatalf("shouldUseExpenseBreakdown(%q) = true, want false", q)
		}
	}
}

func TestShouldUseExpenseBreakdownUsesConfiguredTriggerKeywords(t *testing.T) {
	rulesPath := filepath.Join(t.TempDir(), "rules.json")
	rulesJSON := `{
  "schema_version": 2,
  "router": {
    "expense_breakdown": {
      "trigger_keywords": ["掰开"],
      "expense_keywords": ["花销"]
    }
  }
}`
	if err := os.WriteFile(rulesPath, []byte(rulesJSON), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	if !shouldUseExpenseBreakdown("2026年3月花销掰开看看") {
		t.Fatalf("configured trigger and expense keywords should route to expense breakdown")
	}
}
