package query

import "testing"

func TestBuildCustomerContractCashSummaryIncludesSubPeriodBreakdown(t *testing.T) {
	got := buildCustomerContractCashSummary(3000, "2025-10", 1234)
	want := "实际到账 3000.00 元，其中10月到账 1234.00 元"
	if got != want {
		t.Fatalf("buildCustomerContractCashSummary() = %q, want %q", got, want)
	}
}
