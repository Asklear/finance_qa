package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/config"
)

func TestKeywordsManagerFallsBackToDefaultsWhenFileMissing(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.json")
	mgr := config.NewKeywordsManager(missing)

	if !mgr.CheckKeywordsInText("本月总收入", "intents.monthly_summary.keywords") {
		t.Fatal("expected default monthly summary keywords to match")
	}
	if got := mgr.GetCalculationType("请算利润"); got != "profit" {
		t.Fatalf("expected calculation type profit, got %q", got)
	}
}

func TestKeywordsManagerLoadsJSONAndExposesIntentNames(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "query_keywords.json")
	json := `{
	  "intents": {
	    "custom_intent": {
	      "keywords": ["自定义"]
	    }
	  },
	  "transaction_types": {
	    "expense": {
	      "keywords": ["支出"],
	      "sql_field": "debit_amount",
	      "description": "支出类交易"
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatalf("write test keywords: %v", err)
	}

	mgr := config.NewKeywordsManager(path)
	if !mgr.CheckKeywordsInText("这是自定义查询", "intents.custom_intent.keywords") {
		t.Fatal("expected custom intent keyword match")
	}

	names := mgr.GetIntentNames()
	if len(names) != 1 || names[0] != "custom_intent" {
		t.Fatalf("unexpected intent names: %+v", names)
	}
}
