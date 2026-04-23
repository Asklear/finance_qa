package query

import (
	"testing"

	"financeqa/internal/accounting"
)

func TestBuildCoreMetricBookViewIncludesDisplayedAndNetProfit(t *testing.T) {
	book := monthlyBookView{
		Revenue:             100,
		TotalCost:           80,
		NonOperatingIncome:  5,
		NonOperatingExpense: 2,
		Profit:              23,
		NetProfit:           20,
	}

	got := buildCoreMetricBookView(book, 20)

	if got["营业收入"] != float64(100) {
		t.Fatalf("营业收入 = %v, want 100", got["营业收入"])
	}
	if got["营业成本及费用"] != float64(80) {
		t.Fatalf("营业成本及费用 = %v, want 80", got["营业成本及费用"])
	}
	if got["营业外收入"] != float64(5) {
		t.Fatalf("营业外收入 = %v, want 5", got["营业外收入"])
	}
	if got["营业外支出"] != float64(2) {
		t.Fatalf("营业外支出 = %v, want 2", got["营业外支出"])
	}
	if got["账面利润"] != float64(20) {
		t.Fatalf("账面利润 = %v, want displayed 20", got["账面利润"])
	}
	if got["净利润"] != float64(20) {
		t.Fatalf("净利润 = %v, want 20", got["净利润"])
	}
}

func TestBuildCoreMetricCashFlowSummary(t *testing.T) {
	cash := &accounting.CashPerspective{
		Income:  300,
		Expense: 120,
		Net:     180,
	}

	got := buildCoreMetricCashFlowSummary(cash)

	if got["现金流入"] != float64(300) {
		t.Fatalf("现金流入 = %v, want 300", got["现金流入"])
	}
	if got["现金流出"] != float64(120) {
		t.Fatalf("现金流出 = %v, want 120", got["现金流出"])
	}
	if got["净现金流"] != float64(180) {
		t.Fatalf("净现金流 = %v, want 180", got["净现金流"])
	}
}
