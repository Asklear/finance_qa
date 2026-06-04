package query

import "testing"

func TestLargeTransactionIntentRequiresTransactionGranularity(t *testing.T) {
	intent, _ := ClassifyIntentV2("本月进账多少")
	if intent == IntentLargeTransactionQuery {
		t.Fatalf("plain inbound amount question should not route to large transactions")
	}

	intent, _ = ClassifyIntentV2("这季度有哪几笔大额的进账和支出")
	if intent != IntentLargeTransactionQuery {
		t.Fatalf("large transaction roster intent = %s, want %s", intent, IntentLargeTransactionQuery)
	}
}
