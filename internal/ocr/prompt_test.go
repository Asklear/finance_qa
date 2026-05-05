package ocr_test

import (
	"strings"
	"testing"

	"financeqa/internal/ocr"
)

func TestGeminiPromptRequiresStrictJSONAndEvidence(t *testing.T) {
	prompt := ocr.GeminiExtractionPrompt()
	for _, want := range []string{
		"只返回合法 JSON",
		"document_type",
		"payment_schedule",
		"pages",
		"page_number",
		"ocr_text_excerpt",
		"不要凭文件名猜测",
		"价税合计",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}
