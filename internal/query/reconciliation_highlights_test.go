package query

import "testing"

func TestSelectReconciliationHighlightsFiltersAndSortsByMagnitude(t *testing.T) {
	snapshots := []counterpartySnapshot{
		{
			Name:            "无依据对象",
			BankIn:          5000,
			ComparisonBasis: "",
		},
		{
			Name:            "客户A",
			BankIn:          1000,
			RevenueNet:      800,
			ComparisonBasis: "historical_receipt",
		},
		{
			Name:            "供应商B",
			BankOut:         3000,
			BookCost:        2600,
			ComparisonBasis: "supplier_payment_or_cost",
		},
		{
			Name:            "客户C",
			BankIn:          2000,
			RevenueNet:      1800,
			ComparisonBasis: "vat_gap_only",
		},
	}

	got := selectReconciliationHighlights(snapshots, 2)
	if len(got) != 2 {
		t.Fatalf("len(highlights) = %d, want 2", len(got))
	}
	if got[0].Name != "供应商B" {
		t.Fatalf("highlights[0] = %s, want 供应商B", got[0].Name)
	}
	if got[1].Name != "客户C" {
		t.Fatalf("highlights[1] = %s, want 客户C", got[1].Name)
	}
}

func TestSelectReconciliationHighlightsHonorsLimitAfterFiltering(t *testing.T) {
	snapshots := []counterpartySnapshot{
		{Name: "A", BankIn: 100, ComparisonBasis: "recognized_revenue"},
		{Name: "B", BankIn: 200, ComparisonBasis: "recognized_revenue"},
		{Name: "C", BankIn: 300, ComparisonBasis: "recognized_revenue"},
	}

	got := selectReconciliationHighlights(snapshots, 1)
	if len(got) != 1 {
		t.Fatalf("len(highlights) = %d, want 1", len(got))
	}
	if got[0].Name != "C" {
		t.Fatalf("highlights[0] = %s, want C", got[0].Name)
	}
}
