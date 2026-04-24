package support_test

import (
	"testing"

	"financeqa/internal/support"
)

func TestBuildSourceSummaryPrefersExplicitDocuments(t *testing.T) {
	data := map[string]any{
		"source_documents": []any{
			"《优集资金收入计算表-副本.xlsx》的【25年Q4收入明细】和【26年Q1收入明细】",
		},
		"supporting_source_documents": []any{
			"《合同信息表》",
		},
		"source_summary": "来源：旧字段",
		"source_note":    "来源：更旧字段",
	}

	got := support.BuildSourceSummary(data, "回答正文")
	want := "来源：《优集资金收入计算表-副本.xlsx》的【25年Q4收入明细】和【26年Q1收入明细】；补充参考：《合同信息表》"
	if got != want {
		t.Fatalf("BuildSourceSummary() = %q, want %q", got, want)
	}
}

func TestBuildSourceSummaryFallsBackToStructuredAndMessage(t *testing.T) {
	if got := support.BuildSourceSummary(map[string]any{
		"source_summary": "来源：《序时账》",
	}, "回答正文"); got != "来源：《序时账》" {
		t.Fatalf("source_summary fallback = %q", got)
	}

	if got := support.BuildSourceSummary(map[string]any{}, "回答正文\n来源：《银行流水》"); got != "来源：《银行流水》" {
		t.Fatalf("message fallback = %q", got)
	}
}
