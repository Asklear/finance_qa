package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeywordsManagerLoadsCustomJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "query_keywords.json")
	payload := `{
	  "intents": {
	    "alpha": {
	      "keywords": ["Alpha"],
	      "special_keywords": ["特殊口径"]
	    },
	    "beta": {
	      "keywords": ["Beta"]
	    },
	    "monthly_summary": {
	      "keywords": ["本月"],
	      "special_keywords": ["特殊口径"]
	    }
	  },
	  "transaction_types": {
	    "receipt": {
	      "keywords": ["cash-in"],
	      "sql_field": "credit_amount",
	      "description": "cash receipts"
	    }
	  },
	  "database_fields": {
	    "bank_statement": {
	      "income_field": "credit_amount",
	      "expense_field": "debit_amount"
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write keywords config: %v", err)
	}

	mgr := NewKeywordsManager(path)
	if !mgr.CheckKeywordsInText("please run ALPHA query", "intents.alpha.keywords") {
		t.Fatal("expected keyword matching to be case-insensitive")
	}
	if got := mgr.GetCalculationType("show cash-in total"); got != "receipt" {
		t.Fatalf("calculation type = %q, want receipt", got)
	}
	if got := mgr.GetSQLField("receipt"); got != "credit_amount" {
		t.Fatalf("sql field = %q, want credit_amount", got)
	}
	fields, ok := mgr.GetDatabaseFields("bank_statement")
	if !ok {
		t.Fatal("expected bank_statement fields")
	}
	if fields.IncomeField != "credit_amount" || fields.ExpenseField != "debit_amount" {
		t.Fatalf("unexpected database fields: %+v", fields)
	}
	if !mgr.HasMonthlySummarySpecialKeyword("这个月特殊口径收入") {
		t.Fatal("expected special keyword lookup through raw config")
	}
	if got := mgr.GetIntentNames(); len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "monthly_summary" {
		t.Fatalf("intent names = %+v, want sorted alpha/beta/monthly_summary", got)
	}
}

func TestKeywordsManagerFallsBackToDefaultsOnMissingOrInvalidFile(t *testing.T) {
	t.Parallel()

	missing := NewKeywordsManager(filepath.Join(t.TempDir(), "missing.json"))
	if !missing.CheckKeywordsInText("本月总收入", "intents.monthly_summary.keywords") {
		t.Fatal("missing file should use default monthly summary keywords")
	}

	invalidPath := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalidPath, []byte(`{`), 0o644); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	invalid := NewKeywordsManager(invalidPath)
	if got := invalid.GetCalculationType("请计算利润"); got != "profit" {
		t.Fatalf("invalid file calculation type = %q, want profit from defaults", got)
	}
}
