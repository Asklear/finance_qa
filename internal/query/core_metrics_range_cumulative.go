package query

import "fmt"

type cumulativeMetricAccumulator struct {
	LatestPeriod       string
	LatestCumulative   float64
	HasLatest          bool
	PreviousPeriod     string
	PreviousCumulative float64
	HasPrevious        bool
}

func (e *Engine) cumulativeBookSummaryForRange(from, to string) (monthlyBookView, string, map[string]any, []string, []string, bool, error) {
	hasCumulative, err := e.tableHasColumn("income_statement", "cumulative_amount")
	if err != nil || !hasCumulative {
		return monthlyBookView{}, "", nil, nil, nil, false, err
	}

	startBoundary, boundaryLog := e.resolveCumulativeRangeLowerBound(from, to)
	previousRequired := startBoundary < from
	matchers := buildIncomeStatementRangeMatchers(getRuleConfig())
	accumulators := map[string]*cumulativeMetricAccumulator{}
	for _, matcher := range matchers {
		accumulators[matcher.key] = &cumulativeMetricAccumulator{}
	}

	rows, err := e.db.Query(`
SELECT period, item_name, COALESCE(cumulative_amount, 0)
FROM income_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
ORDER BY period, item_name
`, e.Company, e.Company, startBoundary, to)
	if err != nil {
		return monthlyBookView{}, "", nil, nil, nil, false, err
	}
	defer rows.Close()

	matchedRows := 0
	for rows.Next() {
		var period, itemName string
		var cumulativeAmount float64
		if err := rows.Scan(&period, &itemName, &cumulativeAmount); err != nil {
			return monthlyBookView{}, "", nil, nil, nil, false, err
		}
		matchedKey := ""
		for _, matcher := range matchers {
			if matchIncomeStatementItem(itemName, matcher.patterns) {
				matchedKey = matcher.key
				break
			}
		}
		if matchedKey == "" {
			continue
		}
		matchedRows++
		acc := accumulators[matchedKey]
		if period <= to && (!acc.HasLatest || period >= acc.LatestPeriod) {
			acc.LatestPeriod = period
			acc.LatestCumulative = cumulativeAmount
			acc.HasLatest = true
		}
		if previousRequired && period < from && (!acc.HasPrevious || period >= acc.PreviousPeriod) {
			acc.PreviousPeriod = period
			acc.PreviousCumulative = cumulativeAmount
			acc.HasPrevious = true
		}
	}
	if err := rows.Err(); err != nil {
		return monthlyBookView{}, "", nil, nil, nil, false, err
	}
	if matchedRows == 0 || !accumulators["revenue"].HasLatest {
		return monthlyBookView{}, "", nil, nil, nil, false, nil
	}
	if previousRequired && !accumulators["revenue"].HasPrevious {
		return monthlyBookView{}, "", nil, nil, nil, false, nil
	}

	book := buildCumulativeDeltaBook(accumulators)
	validationItems := map[string]any{}
	for _, key := range []string{"revenue", "cost", "selling_expense", "admin_expense", "finance_expense", "tax_surcharge", "non_operating_income", "non_operating_expense", "profit_total", "net_profit"} {
		acc := accumulators[key]
		if acc == nil || !acc.HasLatest {
			continue
		}
		validationItems[key] = map[string]any{
			"latest_period":        acc.LatestPeriod,
			"latest_cumulative":    round2(acc.LatestCumulative),
			"previous_period":      acc.PreviousPeriod,
			"previous_cumulative":  round2(acc.PreviousCumulative),
			"range_delta_selected": deltaFromCumulativeAccumulator(acc),
			"passed":               true,
		}
	}

	logs := []string{
		boundaryLog,
		fmt.Sprintf("[区间累计口径] from=%s to=%s revenue=%.2f total_cost=%.2f profit=%.2f net_profit=%.2f", from, to, book.Revenue, book.TotalCost, book.Profit, book.NetProfit),
	}
	sqls := []string{
		"range_book_summary(cumulative): SELECT period, item_name, cumulative_amount FROM income_statement WHERE ... AND period BETWEEN ? AND ?",
	}
	validation := map[string]any{
		"basis":            "income_statement_cumulative_delta",
		"from":             from,
		"to":               to,
		"opening_boundary": startBoundary,
		"passed":           true,
		"items":            validationItems,
	}
	return book, "income_statement_cumulative_delta", validation, sqls, logs, true, nil
}

func buildCumulativeDeltaBook(accumulators map[string]*cumulativeMetricAccumulator) monthlyBookView {
	delta := func(key string) float64 {
		return deltaFromCumulativeAccumulator(accumulators[key])
	}

	book := monthlyBookView{
		Revenue:             delta("revenue"),
		Cost:                delta("cost"),
		TaxSurcharge:        delta("tax_surcharge"),
		SellingExpense:      delta("selling_expense"),
		AdminExpense:        delta("admin_expense"),
		FinanceExpense:      delta("finance_expense"),
		NonOperatingIncome:  delta("non_operating_income"),
		NonOperatingExpense: delta("non_operating_expense"),
		IncomeTax:           delta("income_tax"),
	}
	book.TotalCost = round2(book.Cost + book.TaxSurcharge + book.SellingExpense + book.AdminExpense + book.FinanceExpense)
	book.OperatingProfit = delta("operating_profit")
	if book.OperatingProfit == 0 {
		book.OperatingProfit = round2(book.Revenue - book.TotalCost)
	}
	book.Profit = delta("profit_total")
	if book.Profit == 0 {
		book.Profit = round2(book.OperatingProfit + book.NonOperatingIncome - book.NonOperatingExpense)
	}
	book.NetProfit = delta("net_profit")
	if book.NetProfit == 0 && book.IncomeTax != 0 {
		book.NetProfit = round2(book.Profit - book.IncomeTax)
	}
	if book.NetProfit == 0 {
		book.NetProfit = book.Profit
	}
	return book
}

func deltaFromCumulativeAccumulator(acc *cumulativeMetricAccumulator) float64 {
	if acc == nil || !acc.HasLatest {
		return 0
	}
	previous := 0.0
	if acc.HasPrevious {
		previous = acc.PreviousCumulative
	}
	return round2(acc.LatestCumulative - previous)
}

func buildIncomeStatementRangeMatchers(cfg RuleConfig) []incomeStatementMetricMatcher {
	return []incomeStatementMetricMatcher{
		{key: "revenue", patterns: cfg.IncomeStatementPatterns("revenue")},
		{key: "cost", patterns: cfg.IncomeStatementPatterns("cost")},
		{key: "selling_expense", patterns: cfg.IncomeStatementPatterns("selling_expense")},
		{key: "admin_expense", patterns: cfg.IncomeStatementPatterns("admin_expense")},
		{key: "finance_expense", patterns: cfg.IncomeStatementPatterns("finance_expense")},
		{key: "tax_surcharge", patterns: cfg.IncomeStatementPatterns("tax_surcharge")},
		{key: "non_operating_income", patterns: cfg.IncomeStatementPatterns("non_operating_income")},
		{key: "non_operating_expense", patterns: cfg.IncomeStatementPatterns("non_operating_expense")},
		{key: "operating_profit", patterns: cfg.IncomeStatementPatterns("operating_profit")},
		{key: "profit_total", patterns: cfg.IncomeStatementPatterns("profit_total")},
		{key: "net_profit", patterns: cfg.IncomeStatementPatterns("net_profit")},
		{key: "income_tax", patterns: cfg.IncomeStatementPatterns("income_tax")},
	}
}
