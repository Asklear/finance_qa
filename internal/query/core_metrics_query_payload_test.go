package query

import (
	"reflect"
	"testing"

	"financeqa/internal/accounting"
)

func TestBuildMonthlyCoreMetricResultDataIncludesCoreViews(t *testing.T) {
	book := monthlyBookView{
		Revenue:             1000,
		Cost:                700,
		TaxSurcharge:        20,
		SellingExpense:      30,
		AdminExpense:        40,
		FinanceExpense:      10,
		NonOperatingIncome:  15,
		NonOperatingExpense: 5,
		OperatingProfit:     200,
		Profit:              210,
		NetProfit:           180,
		IncomeTax:           30,
		TotalCost:           800,
	}
	cumulative := &accounting.IncomeStatementResult{
		Period:          "2026-03",
		Revenue:         3100,
		Cost:            2400,
		TaxSurcharge:    80,
		SellingExpense:  90,
		AdminExpense:    120,
		FinanceExpense:  25,
		NonOpIncome:     20,
		NonOpExpense:    8,
		OperatingProfit: 385,
		TotalProfit:     397,
		IncomeTax:       57,
		NetProfit:       340,
	}
	cash := &accounting.CashPerspective{
		Income:  900,
		Expense: 650,
		Net:     250,
	}
	bridge := map[string]any{"经营现金净额估算": float64(260)}

	got := buildMonthlyCoreMetricResultData(2026, 3, "income_statement", book, cumulative, cash, bridge)

	monthly, ok := got["monthly"].(map[string]any)
	if !ok {
		t.Fatalf("monthly payload missing: %+v", got)
	}
	if monthly["year"] != 2026 || monthly["month"] != 3 {
		t.Fatalf("monthly year/month = %v/%v, want 2026/3", monthly["year"], monthly["month"])
	}
	if monthly["source"] != "income_statement" {
		t.Fatalf("monthly source = %v, want income_statement", monthly["source"])
	}
	if monthly["profit"] != float64(210) {
		t.Fatalf("monthly profit = %v, want 210", monthly["profit"])
	}

	if got["cumulative"] != cumulative {
		t.Fatalf("cumulative payload = %#v, want original pointer %#v", got["cumulative"], cumulative)
	}
	if !reflect.DeepEqual(got["profit_cash_bridge"], bridge) {
		t.Fatalf("profit_cash_bridge = %#v, want %#v", got["profit_cash_bridge"], bridge)
	}
	if got["现金流入"] != float64(900) || got["现金流出"] != float64(650) || got["净现金流"] != float64(250) {
		t.Fatalf("cash summary mismatch: %+v", got)
	}

	bookView, ok := got["财务做账口径(看利润)"].(map[string]any)
	if !ok {
		t.Fatalf("book view missing: %+v", got)
	}
	if bookView["营业收入"] != float64(1000) {
		t.Fatalf("book revenue = %v, want 1000", bookView["营业收入"])
	}
	if bookView["营业成本及费用"] != float64(800) {
		t.Fatalf("book total cost = %v, want 800", bookView["营业成本及费用"])
	}
	if bookView["账面利润"] != float64(210) {
		t.Fatalf("book profit = %v, want 210", bookView["账面利润"])
	}
}
