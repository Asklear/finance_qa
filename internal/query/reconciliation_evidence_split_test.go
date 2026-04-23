package query

import "testing"

func TestAppendUniqueLedgerEvidenceDedupesAndPreservesOrder(t *testing.T) {
	base := []LedgerEvidence{
		{
			Source:      "bank_statement",
			VoucherDate: "2026-02-01",
			Summary:     "A",
			DebitAmount: 100,
		},
	}
	seen := map[string]struct{}{
		evidenceKey(base[0]): {},
	}
	incoming := []LedgerEvidence{
		base[0],
		{
			Source:       "journal",
			VoucherDate:  "2026-02-02",
			VoucherNo:    "V-1",
			AccountCode:  "6001",
			Summary:      "B",
			CreditAmount: 200,
		},
		{
			Source:      "journal",
			VoucherDate: "2026-02-03",
			VoucherNo:   "V-2",
			AccountCode: "1122",
			Summary:     "C",
			DebitAmount: 300,
		},
	}

	got := appendUniqueLedgerEvidence(base, seen, incoming)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].Summary != "A" || got[1].Summary != "B" || got[2].Summary != "C" {
		t.Fatalf("unexpected order: %+v", got)
	}
	if len(seen) != 3 {
		t.Fatalf("len(seen) = %d, want 3", len(seen))
	}
}
