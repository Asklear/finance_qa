package query

import "financeqa/internal/accounting"

func isGenericMetricEntity(entity string) bool {
	key := normalizeEntityText(entity)
	if key == "" {
		return true
	}
	cfg := getRuleConfig()
	for _, s := range cfg.GenericMetricStopwords {
		if normalizeEntityText(s) == key {
			return true
		}
	}
	return false
}

func detectRequestedMetrics(q string) []string {
	metrics := make([]string, 0, 3)
	cfg := getRuleConfig()
	if side := detectContractARAPMetric(q); side != "" {
		return []string{side}
	}
	if containsAny(q, cfg.MetricKeywords(metricKeyRevenue)) {
		metrics = append(metrics, "收入")
	}
	if containsAny(q, cfg.MetricKeywords(metricKeyCost)) {
		metrics = append(metrics, "成本")
	}
	if containsAny(q, cfg.MetricKeywords(metricKeyProfit)) {
		metrics = append(metrics, "利润")
	}
	if len(metrics) == 0 {
		if containsAny(q, cfg.IntentKeywords(IntentMonthlySummary)) {
			return []string{"收入", "成本", "利润"}
		}
		metrics = append(metrics, detectCoreMetric(q))
	}
	return metrics
}

func detectContractARAPMetric(q string) string {
	switch {
	case containsAny(q, []string{"已收发票未付款", "已收票未付款", "收到发票未付款", "供应商发票未付款"}):
		return "已收票未付款"
	case containsAny(q, []string{"已开发票未收款", "已开票未收款", "已开票未回款", "已开票未付款"}):
		return "已开票未回款"
	case containsAny(q, []string{"应付账款", "应付", "已收发票未付款", "已收票未付款", "收到发票未付款", "供应商发票未付款"}):
		return "应付"
	case containsAny(q, []string{"应收账款", "应收", "已开发票未收款", "已开票未收款", "已开票未回款", "已开票未付款"}):
		return "应收"
	default:
		return ""
	}
}

func detectCoreMetric(q string) string {
	switch {
	case containsAny(q, getRuleConfig().MetricKeywords(metricKeyProfit)):
		return "利润"
	case containsAny(q, getRuleConfig().MetricKeywords(metricKeyCost)):
		return "成本"
	default:
		return "收入"
	}
}

func pickMetricValue(metric string, dual *accounting.DualPerspective) (float64, float64) {
	switch metric {
	case "利润":
		return dual.Cash.Net, dual.Accrual.Profit
	case "成本":
		return dual.Cash.Expense, dual.Accrual.TotalCost
	case "销售额", "收入":
		return dual.Cash.Income, dual.Accrual.Revenue
	default:
		return dual.Cash.Income, dual.Accrual.Revenue
	}
}
