package query

type coreMetricRequest struct {
	RequestedMetrics  []string
	PrimaryMetric     string
	MetricLabel       string
	ExplicitNetProfit bool
}

func resolveCoreMetricRequest(question, multiMetricLabel string) coreMetricRequest {
	requestedMetrics := detectRequestedMetrics(question)
	explicitNetProfit := asksExplicitNetProfit(question)
	if explicitNetProfit {
		requestedMetrics = []string{"净利润"}
	}
	if len(requestedMetrics) == 0 {
		requestedMetrics = []string{detectCoreMetric(question)}
	}

	primaryMetric := firstMetricOrDefault(requestedMetrics, detectCoreMetric(question))
	metricLabel := multiMetricLabel
	if metricLabel == "" {
		metricLabel = primaryMetric
	}
	if len(requestedMetrics) == 1 {
		metricLabel = primaryMetric
	}
	if explicitNetProfit {
		metricLabel = "净利润"
	}

	return coreMetricRequest{
		RequestedMetrics:  requestedMetrics,
		PrimaryMetric:     primaryMetric,
		MetricLabel:       metricLabel,
		ExplicitNetProfit: explicitNetProfit,
	}
}

func buildCoreMetricMetricsMap(book monthlyBookView) map[string]any {
	return map[string]any{
		"收入":  round2(book.Revenue),
		"成本":  round2(book.TotalCost),
		"利润":  round2(book.Profit),
		"净利润": round2(book.NetProfit),
	}
}

func buildCoreMetricSummaryPayload(from, to, source string, book monthlyBookView) map[string]any {
	payload := map[string]any{
		"source":                source,
		"revenue":               book.Revenue,
		"cost":                  book.TotalCost,
		"profit":                book.Profit,
		"non_operating_income":  book.NonOperatingIncome,
		"non_operating_expense": book.NonOperatingExpense,
		"net_profit":            book.NetProfit,
	}
	if from == to {
		year, month := parsePeriod(to)
		payload["year"] = year
		payload["month"] = month
	} else {
		payload["from"] = from
		payload["to"] = to
	}
	return payload
}
