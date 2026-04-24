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
	request := resolveCoreMetricRequest(question, metricDisplayName(detectCoreMetric(question)))
	requestedMetrics := request.RequestedMetrics
	explicitNetProfit := request.ExplicitNetProfit
	metric := request.MetricLabel
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
	data := buildCoreMetricSharedResultFields(bookSource, book, displayedBookProfit, cashFlowSummary, bridgeMap)
	data["period"] = period
	data["metric"] = metric
	data["requested_metrics"] = requestedMetrics
	data["account_value"] = accountValue
	data["total"] = accountValue
	data["metrics"] = buildCoreMetricMetricsMap(book)
	data["monthly"] = buildCoreMetricMonthlyPayload(year, month, bookSource, book)
	return data
}
