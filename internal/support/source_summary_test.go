package support

import "testing"

func TestBuildSourceSummaryPrefersDocumentsAndDedupes(t *testing.T) {
	data := map[string]any{
		"source_documents": []any{" 合同收入表 ", "合同收入表", ""},
		"supporting_source_documents": []string{
			"合同文件",
			"合同文件",
			"发票文件",
		},
		"source_summary": "来源：旧字段",
	}

	got := BuildSourceSummary(data, "回答正文")
	want := "来源：合同收入表；补充参考：合同文件；发票文件"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestBuildSourceSummaryFallsBackToStructuredFieldsAndMessage(t *testing.T) {
	if got := BuildSourceSummary(map[string]any{"source_note": " 来源：序时账 "}, ""); got != "来源：序时账" {
		t.Fatalf("source_note fallback = %q", got)
	}
	if got := BuildSourceSummary(map[string]any{}, "正文\n 来源：银行流水 \n尾部"); got != "来源：银行流水" {
		t.Fatalf("message fallback = %q", got)
	}
	if got := BuildSourceSummary(map[string]any{}, "正文"); got != "" {
		t.Fatalf("empty fallback = %q", got)
	}
}
