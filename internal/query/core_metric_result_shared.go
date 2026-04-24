package query

import "fmt"

func buildCoreMetricSharedResultFields(bookSource string, book monthlyBookView, displayedBookProfit float64, cashFlowSummary, bridgeMap map[string]any) map[string]any {
	return map[string]any{
		"source_tables":          sourceTablesForCoreMetric(bookSource, true),
		"profit_cash_bridge":     bridgeMap,
		"现金流入":                 cashFlowSummary["现金流入"],
		"现金流出":                 cashFlowSummary["现金流出"],
		"净现金流":                 cashFlowSummary["净现金流"],
		"财务做账口径(看利润)":          buildCoreMetricBookView(book, displayedBookProfit),
		"cash_flow":             cashFlowSummary,
	}
}

func buildCoreMetricMonthlyPayload(year, month int, bookSource string, book monthlyBookView) map[string]any {
	payload := buildCoreMetricSummaryPayload(
		formatYearMonth(year, month),
		formatYearMonth(year, month),
		bookSource,
		book,
	)
	payload["cost_detail"] = map[string]any{
		"operating_cost":  book.Cost,
		"tax_surcharge":   book.TaxSurcharge,
		"selling_expense": book.SellingExpense,
		"admin_expense":   book.AdminExpense,
		"finance_expense": book.FinanceExpense,
	}
	return payload
}

func formatYearMonth(year, month int) string {
	return fmt.Sprintf("%04d-%02d", year, month)
}
