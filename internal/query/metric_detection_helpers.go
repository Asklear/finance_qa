package query

import "financeqa/internal/accounting"

func isGenericMetricEntity(entity string) bool {
	key := normalizeEntityText(entity)
	if key == "" {
		return true
	}
	return isGenericMetricEntityWithConfig(entity, getRuleConfig())
}

func isGenericMetricEntityWithConfig(entity string, cfg RuleConfig) bool {
	key := normalizeEntityText(entity)
	if key == "" {
		return true
	}
	for _, s := range cfg.GenericMetricStopwords {
		if normalizeEntityText(s) == key {
			return true
		}
	}
	return false
}

func detectRequestedMetrics(q string) []string {
	return detectRequestedMetricsWithConfig(q, getRuleConfig())
}

func detectRequestedMetricsWithConfig(q string, cfg RuleConfig) []string {
	metrics := make([]string, 0, 3)
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
		metrics = append(metrics, detectCoreMetricWithConfig(q, cfg))
	}
	return metrics
}

func detectContractARAPMetric(q string) string {
	switch {
	case looksLikeSupplierInvoiceUnpaidQuestion(q):
		return "已收票未付款"
	case containsAny(q, []string{"含未开票未付款", "未开票未付款", "未开票未回款", "应收未收"}):
		return "应收"
	case containsAny(q, []string{"客户未付款", "客户没付款", "客户未支付"}):
		return "已开票未回款"
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

func looksLikeSupplierInvoiceUnpaidQuestion(q string) bool {
	if !containsAny(q, []string{"已开票未付款", "已开发票未付款", "开票未付款", "发票未付款", "未付款", "未支付"}) {
		return false
	}
	if containsAny(q, []string{"未回款", "未收款", "客户未付款", "客户没付款", "客户未支付"}) {
		return false
	}
	return containsAny(q, []string{"供应商", "采购", "成本", "应付", "收票", "收到发票", "已收发票", "已收票", "合同", "项目"})
}

func detectCoreMetric(q string) string {
	return detectCoreMetricWithConfig(q, getRuleConfig())
}

func detectCoreMetricWithConfig(q string, cfg RuleConfig) string {
	switch {
	case containsAny(q, cfg.MetricKeywords(metricKeyProfit)):
		return "利润"
	case containsAny(q, cfg.MetricKeywords(metricKeyCost)):
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
