package query

func buildContractAggregateResultData(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	total := contractAggregateMetricValue(selection.PrimaryMetric, summary)
	return map[string]any{
		"period":            summary.Period,
		"requested_period":  summary.Period,
		"metric":            selection.PrimaryMetric,
		"requested_metrics": selection.RequestedMetrics,
		"total":             round2(total),
		"metrics":           buildContractAggregateMetricMap(selection, summary),
		"source_priority":   "contract_first",
		"source_tables":     append([]string{}, summary.SourceTables...),
		"contract_summary":  buildContractAggregatePayloadSummary(selection, summary),
		"money_view":        buildContractAggregateMoneyView(selection, summary),
		"account_view":      buildContractAggregateAccountView(selection, summary),
	}
}

func buildContractAggregateMetricMap(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	metrics := map[string]any{}
	if selection.IncludeRevenue {
		metrics["收入"] = round2(summary.RevenueSettlement)
	}
	if selection.IncludeCost {
		metrics["成本"] = round2(summary.CostSettlement)
	}
	if selection.IncludeProfit {
		metrics["利润"] = round2(summary.Profit)
	}
	return metrics
}

func buildContractAggregatePayloadSummary(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	payload := map[string]any{
		"scope":          summary.Scope,
		"entity":         summary.Entity,
		"contract_count": summary.ContractCount,
		"coverage":       buildContractAggregateCoverage(selection, summary),
	}
	if selection.IncludeRevenue {
		payload["revenue_settlement"] = round2(summary.RevenueSettlement)
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["invoice_amount"] = round2(summary.RevenueInvoiced)
	}
	if selection.IncludeCost {
		payload["cost_settlement"] = round2(summary.CostSettlement)
		payload["cost_paid"] = round2(summary.CostPaid)
	}
	if selection.IncludeProfit {
		payload["profit"] = round2(summary.Profit)
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["cost_paid"] = round2(summary.CostPaid)
	}
	return payload
}

func buildContractAggregateCoverage(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	coverage := map[string]any{}
	if selection.IncludeRevenue {
		coverage["收入"] = summary.HasRevenueCoverage
	}
	if selection.IncludeCost {
		coverage["成本"] = summary.HasCostCoverage
	}
	if selection.IncludeProfit {
		coverage["利润"] = summary.HasRevenueCoverage && summary.HasCostCoverage
	}
	return coverage
}

func buildContractAggregateMoneyView(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	view := map[string]any{
		"说明": "合同现金口径",
	}
	switch {
	case selection.IncludeProfit && !selection.IncludeRevenue && !selection.IncludeCost:
		view["回款"] = round2(summary.RevenueReceived)
		view["付款"] = round2(summary.CostPaid)
		view["净现金"] = round2(summary.RevenueReceived - summary.CostPaid)
	case selection.IncludeCost && !selection.IncludeRevenue && !selection.IncludeProfit:
		view["付款"] = round2(summary.CostPaid)
	case selection.IncludeRevenue && !selection.IncludeCost && !selection.IncludeProfit:
		view["到账"] = round2(summary.RevenueReceived)
	default:
		if selection.IncludeRevenue {
			view["回款"] = round2(summary.RevenueReceived)
		}
		if selection.IncludeCost {
			view["付款"] = round2(summary.CostPaid)
		}
		if selection.IncludeProfit || (selection.IncludeRevenue && selection.IncludeCost) {
			view["净现金"] = round2(summary.RevenueReceived - summary.CostPaid)
		}
	}
	return view
}

func buildContractAggregateAccountView(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	view := map[string]any{
		"说明": "合同经营口径",
	}
	if selection.IncludeRevenue {
		view["营收"] = round2(summary.RevenueSettlement)
		view["已开票"] = round2(summary.RevenueInvoiced)
	}
	if selection.IncludeCost {
		view["合同成本"] = round2(summary.CostSettlement)
	}
	if selection.IncludeProfit {
		view["利润"] = round2(summary.Profit)
	}
	return view
}
