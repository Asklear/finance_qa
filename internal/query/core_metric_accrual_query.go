package query

import (
	"context"
	"fmt"
	"strings"

	"financeqa/internal/analysis"
)

func (e *Engine) queryAccrualCoreMetrics(question, from, to string) Result {
	year, month := parsePeriod(to)
	e.calc.ResetTrace()

	book, bookSource, err := e.monthlyBookSummary(year, month)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	cash, _ := e.calc.ComputeCashFlow(e.Company, from, to)
	logs := append([]string{}, e.calc.CalculationLogs...)
	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	requestedMetrics := detectRequestedMetrics(question)
	explicitNetProfit := asksExplicitNetProfit(question)
	if explicitNetProfit {
		requestedMetrics = []string{"净利润"}
	}
	if len(requestedMetrics) == 0 {
		requestedMetrics = []string{detectCoreMetric(question)}
	}
	metric := metricDisplayName(detectCoreMetric(question))
	if explicitNetProfit {
		metric = "净利润"
	}
	if len(requestedMetrics) == 1 {
		metric = requestedMetrics[0]
	}
	accountValue := round2(metricValueFromBook(metric, book))
	displayedBookProfit := book.Profit
	if explicitNetProfit {
		displayedBookProfit = book.NetProfit
	}

	var bridgeMap map[string]any
	if containsString(requestedMetrics, "利润") {
		if bridge, bridgeErr := analysis.AnalyzeProfitCashBridgeWithDB(context.Background(), e.db, e.Company, to); bridgeErr == nil {
			bridgeMap = bridgeToMap(&bridge)
			logs = append(logs, fmt.Sprintf("[核心指标-单口径] period=%s profit=%.2f net_profit=%.2f estimated_operating_cash=%.2f", to, book.Profit, book.NetProfit, bridge.EstimatedOperatingCash))
			sqls = appendUniqueStrings(sqls,
				"profit_cash_bridge(balance_detail): SELECT closing_debit, closing_credit FROM balance_detail WHERE ... AND period IN (?, previous_period) AND account_code IN ('1602','1122','1123','1221','2202','2203','2211','2221','2241','22210101','22210106')",
				"profit_cash_bridge(income_statement): SELECT current_amount FROM income_statement WHERE ... AND period = ? AND item_name LIKE '%净利润%'",
			)
		}
	}

	sqls, logs = appendCoreMetricBookSummaryTrace(sqls, logs, to, bookSource)
	logs = append(logs, fmt.Sprintf("[核心指标-单口径] period=%s source=%s requested=%v metric=%s account_value=%.2f", to, bookSource, requestedMetrics, metric, accountValue))

	result := Result{
		Success:         true,
		AnswerMethod:    "sql",
		Message:         buildAccrualCoreMetricsMessage(to, requestedMetrics, book),
		Data:            buildAccrualCoreMetricResultData(to, year, month, bookSource, requestedMetrics, metric, accountValue, displayedBookProfit, book, buildCoreMetricCashFlowSummary(cash), bridgeMap),
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
	return annotateJournalTaxDisclosure(result, strings.Contains(bookSource, "journal"))
}

func (e *Engine) queryAccrualProfitOnly(from, to string) Result {
	return e.queryAccrualCoreMetrics("利润", from, to)
}

func buildAccrualCoreMetricResultData(period string, year, month int, bookSource string, requestedMetrics []string, metric string, accountValue, displayedBookProfit float64, book monthlyBookView, cashFlowSummary, bridgeMap map[string]any) map[string]any {
	return map[string]any{
		"period":            period,
		"metric":            metric,
		"requested_metrics": requestedMetrics,
		"account_value":     accountValue,
		"total":             accountValue,
		"metrics": map[string]any{
			"收入":  round2(book.Revenue),
			"成本":  round2(book.TotalCost),
			"利润":  round2(book.Profit),
			"净利润": round2(book.NetProfit),
		},
		"monthly": map[string]any{
			"year":                  year,
			"month":                 month,
			"source":                bookSource,
			"revenue":               book.Revenue,
			"cost":                  book.TotalCost,
			"profit":                book.Profit,
			"net_profit":            book.NetProfit,
			"non_operating_income":  book.NonOperatingIncome,
			"non_operating_expense": book.NonOperatingExpense,
			"income_tax":            book.IncomeTax,
			"operating_profit":      book.OperatingProfit,
		},
		"财务做账口径(看利润)":        buildCoreMetricBookView(book, displayedBookProfit),
		"现金流入":               cashFlowSummary["现金流入"],
		"现金流出":               cashFlowSummary["现金流出"],
		"净现金流":               cashFlowSummary["净现金流"],
		"cash_flow":          cashFlowSummary,
		"profit_cash_bridge": bridgeMap,
	}
}
