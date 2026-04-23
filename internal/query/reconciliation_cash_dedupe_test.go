package query

import "testing"

func TestSummarizeCounterpartyCashEvidenceDedupesJournalBankLinesAgainstBankStatement(t *testing.T) {
	evidence := []LedgerEvidence{
		{
			Source:       "bank_statement",
			VoucherDate:  "2026-01-05T00:00:00Z",
			CreditAmount: 1250400,
		},
		{
			Source:      "journal",
			VoucherDate: "2026-01-05T00:00:00Z",
			VoucherNo:   "记-0006",
			AccountCode: "100201",
			Summary:     "飞未云科(深圳)技术有限公司转账",
			DebitAmount: 1250400,
		},
		{
			Source:      "journal",
			VoucherDate: "2026-02-12T00:00:00Z",
			VoucherNo:   "记-0029",
			AccountCode: "100201",
			Summary:     "飞未云科(深圳)技术有限公司转账",
			DebitAmount: 450100,
		},
	}

	bankIn, bankOut := summarizeCounterpartyCashEvidence(evidence)
	if bankIn != 1700500 {
		t.Fatalf("bankIn = %.2f, want 1700500.00", bankIn)
	}
	if bankOut != 0 {
		t.Fatalf("bankOut = %.2f, want 0", bankOut)
	}
}
