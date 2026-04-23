package query

import (
	"database/sql"
	"fmt"
	"strings"
)

type cumulativeValidationAccumulator struct {
	CurrentSum   float64
	PreviousCumu sql.NullFloat64
	LatestCumu   sql.NullFloat64
	PreviousAt   string
	LatestAt     string
}

func (e *Engine) validateIncomeStatementRangeTotals(from, to string) (map[string]any, []string, []string, error) {
	hasCumulative, err := e.tableHasColumn("income_statement", "cumulative_amount")
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("[区间校验] skipped: detect cumulative_amount failed: %v", err)}, nil
	}
	if !hasCumulative {
		return nil, nil, []string{"[区间校验] skipped: income_statement has no cumulative_amount column"}, nil
	}

	startBoundary, boundaryLog := e.resolveCumulativeRangeLowerBound(from, to)
	rows, err := e.db.Query(`
SELECT period, item_name, COALESCE(current_amount, 0), COALESCE(cumulative_amount, 0)
FROM income_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
ORDER BY period, item_name
`, e.Company, e.Company, startBoundary, to)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	matchers := buildIncomeStatementValidationMatchers(getRuleConfig())
	accumulators := map[string]*cumulativeValidationAccumulator{}
	for _, matcher := range matchers {
		accumulators[matcher.key] = &cumulativeValidationAccumulator{}
	}

	matchedRows := 0
	for rows.Next() {
		var period string
		var itemName string
		var currentAmount float64
		var cumulativeAmount float64
		if err := rows.Scan(&period, &itemName, &currentAmount, &cumulativeAmount); err != nil {
			return nil, nil, nil, err
		}
		matchedKey := matchIncomeStatementKey(itemName, matchers)
		if matchedKey == "" {
			continue
		}
		matchedRows++
		acc := accumulators[matchedKey]
		if period >= from && period <= to {
			acc.CurrentSum = round2(acc.CurrentSum + currentAmount)
			if !acc.LatestCumu.Valid || period >= acc.LatestAt {
				acc.LatestCumu = sql.NullFloat64{Float64: cumulativeAmount, Valid: true}
				acc.LatestAt = period
			}
			continue
		}
		if period < from && (!acc.PreviousCumu.Valid || period >= acc.PreviousAt) {
			acc.PreviousCumu = sql.NullFloat64{Float64: cumulativeAmount, Valid: true}
			acc.PreviousAt = period
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if matchedRows == 0 {
		return nil, nil, []string{"[区间校验] skipped: no income_statement rows matched validation categories"}, nil
	}

	result := buildRangeValidationResult(from, to, startBoundary, matchers, accumulators)
	if result == nil {
		return nil, nil, []string{"[区间校验] skipped: no comparable cumulative rows in selected range"}, nil
	}
	return result, []string{
		"range_validation(income_statement): compare SUM(current_amount) with cumulative_amount delta over selected range",
	}, buildRangeValidationLogs(boundaryLog, matchers, accumulators), nil
}

func buildRangeValidationResult(from, to, startBoundary string, matchers []incomeStatementMetricMatcher, accumulators map[string]*cumulativeValidationAccumulator) map[string]any {
	items := map[string]any{}
	passed := true
	for _, matcher := range matchers {
		acc := accumulators[matcher.key]
		if acc == nil || (!acc.LatestCumu.Valid && acc.CurrentSum == 0) {
			continue
		}
		summary := summarizeCumulativeValidationAccumulator(*acc)
		passed = passed && summary.Passed
		items[matcher.key] = map[string]any{
			"current_sum":      summary.CurrentSum,
			"cumulative_delta": summary.CumulativeDelta,
			"diff":             summary.Diff,
			"passed":           summary.Passed,
			"latest_period":    summary.LatestPeriod,
			"previous_period":  summary.PreviousPeriod,
		}
	}
	if len(items) == 0 {
		return nil
	}
	return map[string]any{
		"basis":            "sum_current_amount_vs_cumulative_delta",
		"from":             from,
		"to":               to,
		"opening_boundary": startBoundary,
		"passed":           passed,
		"items":            items,
	}
}

func buildRangeValidationLogs(boundaryLog string, matchers []incomeStatementMetricMatcher, accumulators map[string]*cumulativeValidationAccumulator) []string {
	logs := make([]string, 0, len(accumulators)+1)
	if boundaryLog != "" {
		logs = append(logs, boundaryLog)
	}
	for _, matcher := range matchers {
		acc := accumulators[matcher.key]
		if acc == nil || (!acc.LatestCumu.Valid && acc.CurrentSum == 0) {
			continue
		}
		summary := summarizeCumulativeValidationAccumulator(*acc)
		logs = append(logs, fmt.Sprintf("[区间校验] item=%s current_sum=%.2f cumulative_delta=%.2f diff=%.2f passed=%t", matcher.key, summary.CurrentSum, summary.CumulativeDelta, summary.Diff, summary.Passed))
	}
	if len(logs) == 0 {
		return []string{"[区间校验] skipped: no comparable cumulative rows in selected range"}
	}
	return logs
}

func matchIncomeStatementKey(itemName string, matchers []incomeStatementMetricMatcher) string {
	for _, matcher := range matchers {
		for _, pattern := range matcher.patterns {
			if strings.Contains(itemName, pattern) {
				return matcher.key
			}
		}
	}
	return ""
}

func buildIncomeStatementValidationMatchers(cfg RuleConfig) []incomeStatementMetricMatcher {
	return []incomeStatementMetricMatcher{
		{key: "revenue", patterns: cfg.IncomeStatementPatterns("revenue")},
		{key: "cost", patterns: cfg.IncomeStatementPatterns("cost")},
		{key: "selling_expense", patterns: cfg.IncomeStatementPatterns("selling_expense")},
		{key: "admin_expense", patterns: cfg.IncomeStatementPatterns("admin_expense")},
		{key: "finance_expense", patterns: cfg.IncomeStatementPatterns("finance_expense")},
		{key: "tax_surcharge", patterns: cfg.IncomeStatementPatterns("tax_surcharge")},
		{key: "profit", patterns: dedupeNonEmpty(append(append([]string{}, cfg.IncomeStatementPatterns("net_profit")...), cfg.IncomeStatementPatterns("profit_total")...))},
	}
}
