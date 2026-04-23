package query

import (
	"fmt"
	"strings"
)

func buildContractAggregateResultSnapshot(spec QuerySpec, summary contractAggregateSummary) (string, map[string]any) {
	requestedMetrics := append([]string{}, summary.RequestedMetrics...)
	primaryMetric := firstMetricOrDefault(requestedMetrics, detectCoreMetric(spec.OriginalQuestion))
	total := contractAggregateMetricValue(primaryMetric, summary)

	scopeLabel := fmt.Sprintf("%s 老板口径先看合同/项目汇总", summary.Period)
	if summary.Entity != "" {
		scopeLabel = fmt.Sprintf("[%s] %s 老板口径先看合同/项目汇总", summary.Entity, summary.Period)
	}

	metricParts := make([]string, 0, 3)
	if contractAggregateIncludesMetric(requestedMetrics, "收入") {
		metricParts = append(metricParts, fmt.Sprintf("营收 %.2f 元", summary.RevenueSettlement))
	}
	if contractAggregateIncludesMetric(requestedMetrics, "成本") {
		metricParts = append(metricParts, fmt.Sprintf("合同成本 %.2f 元", summary.CostSettlement))
	}
	if contractAggregateIncludesMetric(requestedMetrics, "利润") {
		metricParts = append(metricParts, fmt.Sprintf("利润 %.2f 元", summary.Profit))
	}
	if len(metricParts) == 0 {
		metricParts = append(metricParts, fmt.Sprintf("营收 %.2f 元", summary.RevenueSettlement))
	}

	message := fmt.Sprintf("%s：%s。",
		scopeLabel,
		strings.Join(metricParts, "，"))
	if supplement := buildContractAggregateSupplement(requestedMetrics, summary); supplement != "" {
		message += supplement
	}

	metrics := buildContractAggregateMetricMap(requestedMetrics, summary)
	moneyView := buildContractAggregateMoneyView(requestedMetrics, summary)
	accountView := buildContractAggregateAccountView(requestedMetrics, summary)

	data := map[string]any{
		"period":            summary.Period,
		"requested_period":  summary.Period,
		"metric":            primaryMetric,
		"requested_metrics": requestedMetrics,
		"total":             round2(total),
		"metrics":           metrics,
		"source_priority": "contract_first",
		"source_tables":   append([]string{}, summary.SourceTables...),
		"contract_summary": buildContractAggregatePayloadSummary(requestedMetrics, summary),
		"money_view":       moneyView,
		"account_view":     accountView,
	}
	return message, data
}

func contractAggregateMetricValue(metric string, summary contractAggregateSummary) float64 {
	switch strings.TrimSpace(metric) {
	case "成本":
		return round2(summary.CostSettlement)
	case "利润":
		return round2(summary.Profit)
	default:
		return round2(summary.RevenueSettlement)
	}
}

func contractAggregateIncludesMetric(requestedMetrics []string, metric string) bool {
	if len(requestedMetrics) == 0 {
		return true
	}
	for _, requested := range requestedMetrics {
		if strings.TrimSpace(requested) == metric {
			return true
		}
	}
	return false
}

func buildContractAggregateMetricMap(requestedMetrics []string, summary contractAggregateSummary) map[string]any {
	metrics := map[string]any{}
	if contractAggregateIncludesMetric(requestedMetrics, "收入") {
		metrics["收入"] = round2(summary.RevenueSettlement)
	}
	if contractAggregateIncludesMetric(requestedMetrics, "成本") {
		metrics["成本"] = round2(summary.CostSettlement)
	}
	if contractAggregateIncludesMetric(requestedMetrics, "利润") {
		metrics["利润"] = round2(summary.Profit)
	}
	return metrics
}

func buildContractAggregatePayloadSummary(requestedMetrics []string, summary contractAggregateSummary) map[string]any {
	payload := map[string]any{
		"scope":          summary.Scope,
		"entity":         summary.Entity,
		"contract_count": summary.ContractCount,
		"coverage":       buildContractAggregateCoverage(requestedMetrics, summary),
	}
	if contractAggregateIncludesMetric(requestedMetrics, "收入") {
		payload["revenue_settlement"] = round2(summary.RevenueSettlement)
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["invoice_amount"] = round2(summary.RevenueInvoiced)
	}
	if contractAggregateIncludesMetric(requestedMetrics, "成本") {
		payload["cost_settlement"] = round2(summary.CostSettlement)
		payload["cost_paid"] = round2(summary.CostPaid)
	}
	if contractAggregateIncludesMetric(requestedMetrics, "利润") {
		payload["profit"] = round2(summary.Profit)
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["cost_paid"] = round2(summary.CostPaid)
	}
	return payload
}

func buildContractAggregateCoverage(requestedMetrics []string, summary contractAggregateSummary) map[string]any {
	coverage := map[string]any{}
	if contractAggregateIncludesMetric(requestedMetrics, "收入") {
		coverage["收入"] = summary.HasRevenueCoverage
	}
	if contractAggregateIncludesMetric(requestedMetrics, "成本") {
		coverage["成本"] = summary.HasCostCoverage
	}
	if contractAggregateIncludesMetric(requestedMetrics, "利润") {
		coverage["利润"] = summary.HasRevenueCoverage && summary.HasCostCoverage
	}
	return coverage
}

func buildContractAggregateMoneyView(requestedMetrics []string, summary contractAggregateSummary) map[string]any {
	view := map[string]any{
		"说明": "合同现金口径",
	}
	includeRevenue := contractAggregateIncludesMetric(requestedMetrics, "收入")
	includeCost := contractAggregateIncludesMetric(requestedMetrics, "成本")
	includeProfit := contractAggregateIncludesMetric(requestedMetrics, "利润")

	switch {
	case includeProfit && !includeRevenue && !includeCost:
		view["回款"] = round2(summary.RevenueReceived)
		view["付款"] = round2(summary.CostPaid)
		view["净现金"] = round2(summary.RevenueReceived - summary.CostPaid)
	case includeCost && !includeRevenue && !includeProfit:
		view["付款"] = round2(summary.CostPaid)
	case includeRevenue && !includeCost && !includeProfit:
		view["到账"] = round2(summary.RevenueReceived)
	default:
		if includeRevenue {
			view["回款"] = round2(summary.RevenueReceived)
		}
		if includeCost {
			view["付款"] = round2(summary.CostPaid)
		}
		if includeProfit || (includeRevenue && includeCost) {
			view["净现金"] = round2(summary.RevenueReceived - summary.CostPaid)
		}
	}
	return view
}

func buildContractAggregateAccountView(requestedMetrics []string, summary contractAggregateSummary) map[string]any {
	view := map[string]any{
		"说明": "合同经营口径",
	}
	if contractAggregateIncludesMetric(requestedMetrics, "收入") {
		view["营收"] = round2(summary.RevenueSettlement)
		view["已开票"] = round2(summary.RevenueInvoiced)
	}
	if contractAggregateIncludesMetric(requestedMetrics, "成本") {
		view["合同成本"] = round2(summary.CostSettlement)
	}
	if contractAggregateIncludesMetric(requestedMetrics, "利润") {
		view["利润"] = round2(summary.Profit)
	}
	return view
}

func buildContractAggregateSupplement(requestedMetrics []string, summary contractAggregateSummary) string {
	includeRevenue := contractAggregateIncludesMetric(requestedMetrics, "收入")
	includeCost := contractAggregateIncludesMetric(requestedMetrics, "成本")
	includeProfit := contractAggregateIncludesMetric(requestedMetrics, "利润")

	switch {
	case includeProfit && !includeRevenue && !includeCost:
		return fmt.Sprintf("补充合同现金净额 %.2f 元（回款 %.2f 元，付款 %.2f 元）。",
			round2(summary.RevenueReceived-summary.CostPaid),
			round2(summary.RevenueReceived),
			round2(summary.CostPaid))
	case includeCost && !includeRevenue && !includeProfit:
		return fmt.Sprintf("补充合同现金付款 %.2f 元。", round2(summary.CostPaid))
	case includeRevenue && !includeCost && !includeProfit:
		return fmt.Sprintf("补充合同现金到账 %.2f 元，已开票 %.2f 元。", round2(summary.RevenueReceived), round2(summary.RevenueInvoiced))
	default:
		parts := make([]string, 0, 4)
		if includeRevenue {
			parts = append(parts, fmt.Sprintf("合同现金回款 %.2f 元", round2(summary.RevenueReceived)))
			parts = append(parts, fmt.Sprintf("已开票 %.2f 元", round2(summary.RevenueInvoiced)))
		}
		if includeCost {
			parts = append(parts, fmt.Sprintf("合同现金付款 %.2f 元", round2(summary.CostPaid)))
		}
		if includeProfit || (includeRevenue && includeCost) {
			parts = append(parts, fmt.Sprintf("净现金 %.2f 元", round2(summary.RevenueReceived-summary.CostPaid)))
		}
		if len(parts) == 0 {
			return ""
		}
		return "补充" + strings.Join(parts, "，") + "。"
	}
}

func contractAggregateCanAnswer(requestedMetrics []string, summary contractAggregateSummary) bool {
	for _, metric := range requestedMetrics {
		switch strings.TrimSpace(metric) {
		case "收入":
			if !summary.HasRevenueCoverage {
				return false
			}
		case "成本":
			if !summary.HasCostCoverage {
				return false
			}
		case "利润":
			if !(summary.HasRevenueCoverage && summary.HasCostCoverage) {
				return false
			}
		}
	}
	return len(requestedMetrics) > 0
}

func contractAggregateFallbackReason(requestedMetrics []string, summary contractAggregateSummary) string {
	missing := make([]string, 0, 2)
	for _, metric := range requestedMetrics {
		switch strings.TrimSpace(metric) {
		case "收入":
			if !summary.HasRevenueCoverage {
				missing = append(missing, "营收结算")
			}
		case "成本":
			if !summary.HasCostCoverage {
				missing = append(missing, "合同成本")
			}
		case "利润":
			if !summary.HasRevenueCoverage {
				missing = append(missing, "营收结算")
			}
			if !summary.HasCostCoverage {
				missing = append(missing, "合同成本")
			}
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("合同/项目汇总表当前缺少%s，已回退到现金+经营/财务口径", joinWithComma(dedupeStrings(missing)))
}
