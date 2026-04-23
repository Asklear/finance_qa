package query

import (
	"fmt"
	"sort"
	"strings"
)

func (e *Engine) bookSummaryForRange(from, to string) (monthlyBookView, string, map[string]any, []string, []string, error) {
	periods, err := periodsBetween(from, to)
	if err != nil {
		return monthlyBookView{}, "", nil, nil, nil, err
	}
	if len(periods) == 0 {
		return monthlyBookView{}, "", nil, nil, nil, fmt.Errorf("no periods resolved for range %s~%s", from, to)
	}
	if len(periods) == 1 {
		year, month := parsePeriod(periods[0])
		book, source, err := e.monthlyBookSummary(year, month)
		if err != nil {
			return monthlyBookView{}, "", nil, nil, nil, err
		}
		return book, source, nil,
			[]string{
				"monthlyBookSummary(income_statement): SELECT item_name, current_amount FROM income_statement WHERE ... AND period = ?",
				"monthlyBookSummary(fallback_journal): ComputeMonthlyFromJournal + ComputeIncomeStatement when income_statement missing required rows",
			},
			[]string{fmt.Sprintf("[期间汇总] single_period=%s source=%s", periods[0], source)},
			nil
	}

	if cumulativeBook, source, validation, sqls, logs, ok, err := e.cumulativeBookSummaryForRange(from, to); err != nil {
		return monthlyBookView{}, "", nil, nil, nil, err
	} else if ok {
		return cumulativeBook, source, validation, sqls, logs, nil
	}

	var total monthlyBookView
	sourceCounts := map[string]int{}
	logs := make([]string, 0, len(periods)+2)
	for _, period := range periods {
		year, month := parsePeriod(period)
		book, source, err := e.monthlyBookSummary(year, month)
		if err != nil {
			return monthlyBookView{}, "", nil, nil, nil, err
		}
		sourceCounts[source]++
		total = sumMonthlyBookView(total, book)
		logs = append(logs, fmt.Sprintf("[期间汇总] period=%s source=%s revenue=%.2f cost=%.2f profit=%.2f", period, source, book.Revenue, book.TotalCost, book.Profit))
	}
	logs = append(logs, fmt.Sprintf("[期间汇总] aggregated_period=%s total_revenue=%.2f total_cost=%.2f total_profit=%.2f", displayPeriod(from, to), total.Revenue, total.TotalCost, total.Profit))

	validation, validationSQLs, validationLogs, err := e.validateIncomeStatementRangeTotals(from, to)
	if err != nil {
		return monthlyBookView{}, "", nil, nil, nil, err
	}
	return total, "range_monthly_book_summary(" + formatSourceCounts(sourceCounts) + ")", validation,
		append([]string{
			"range_book_summary: sum monthlyBookSummary over each period in selected range",
			"monthlyBookSummary(income_statement): SELECT item_name, current_amount FROM income_statement WHERE ... AND period = ?",
			"monthlyBookSummary(fallback_journal): ComputeMonthlyFromJournal + ComputeIncomeStatement when income_statement missing required rows",
		}, validationSQLs...),
		append(logs, validationLogs...),
		nil
}

func sumMonthlyBookView(base, add monthlyBookView) monthlyBookView {
	return monthlyBookView{
		Revenue:             round2(base.Revenue + add.Revenue),
		Cost:                round2(base.Cost + add.Cost),
		TaxSurcharge:        round2(base.TaxSurcharge + add.TaxSurcharge),
		SellingExpense:      round2(base.SellingExpense + add.SellingExpense),
		AdminExpense:        round2(base.AdminExpense + add.AdminExpense),
		FinanceExpense:      round2(base.FinanceExpense + add.FinanceExpense),
		NonOperatingIncome:  round2(base.NonOperatingIncome + add.NonOperatingIncome),
		NonOperatingExpense: round2(base.NonOperatingExpense + add.NonOperatingExpense),
		OperatingProfit:     round2(base.OperatingProfit + add.OperatingProfit),
		Profit:              round2(base.Profit + add.Profit),
		NetProfit:           round2(base.NetProfit + add.NetProfit),
		IncomeTax:           round2(base.IncomeTax + add.IncomeTax),
		TotalCost:           round2(base.TotalCost + add.TotalCost),
	}
}

func formatSourceCounts(sourceCounts map[string]int) string {
	if len(sourceCounts) == 0 {
		return "unknown"
	}
	keys := make([]string, 0, len(sourceCounts))
	for key := range sourceCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, sourceCounts[key]))
	}
	return strings.Join(parts, ",")
}
