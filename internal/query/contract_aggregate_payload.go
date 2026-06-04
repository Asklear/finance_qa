package query

import "strings"

func buildContractAggregateResultData(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	total := contractAggregateMetricValue(selection.PrimaryMetric, summary)
	data := map[string]any{
		"period":            summary.Period,
		"period_from":       summary.PeriodFrom,
		"period_to":         summary.PeriodTo,
		"requested_period":  summary.RequestedPeriod,
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
	semanticFamilies := contractAggregateSemanticFamilies(selection, summary)
	if summary.PeriodAdjusted || len(semanticFamilies) > 0 {
		overrides := map[string]any{}
		if summary.PeriodAdjusted {
			overrides["period_from"] = summary.PeriodFrom
			overrides["period_to"] = summary.PeriodTo
			overrides["time_scope"] = string(timeScopeFromPeriodRange(summary.PeriodFrom, summary.PeriodTo))
		}
		if len(semanticFamilies) > 0 {
			overrides["semantic_families"] = semanticFamilies
		}
		data["query_spec_overrides"] = overrides
	}
	return data
}

func contractAggregateSemanticFamilies(selection contractAggregateSelection, summary contractAggregateSummary) []string {
	q := summary.OriginalQuestion
	switch {
	case shouldUseCustomerRevenueAnalysisQuestion(q, getRuleConfig()):
		return []string{"customer_concentration", "revenue_customer_ranking"}
	case shouldUseContractReceivableOutstandingQuestion(q, getRuleConfig()) && strings.Contains(q, "催"):
		return []string{"receivable_outstanding", "collection_priority"}
	case shouldUseContractReceivableOutstandingQuestion(q, getRuleConfig()) || selection.IncludeReceivable:
		if strings.Contains(q, "催") || strings.Contains(q, "最该") {
			return []string{"receivable_outstanding", "collection_priority"}
		}
		return []string{"receivable_outstanding", "revenue_invoice_collection"}
	case shouldUseContractCostAnalysisQuestion(q, getRuleConfig()):
		return []string{"supplier_cost_ranking", "project_cost"}
	case selection.IncludeProfit:
		return []string{"business_margin", "project_settlement_cost"}
	case selection.IncludeRevenue && containsAny(q, []string{"比起来", "对比", "相比", "进入"}):
		return []string{"period_revenue_comparison", "business_settlement_trend"}
	default:
		return nil
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
	if selection.IncludeReceivable {
		metrics["应收"] = round2(summary.RevenueReceivable)
	}
	if selection.IncludePayable {
		metrics["应付"] = round2(summary.CostPayable)
	}
	if selection.IncludeInvoiceAR {
		metrics["已开票未回款"] = round2(summary.RevenueInvoiceOpen)
	}
	if selection.IncludeInvoiceAP {
		metrics["已收票未付款"] = round2(summary.CostInvoiceOpen)
	}
	return metrics
}

func buildContractAggregatePayloadSummary(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	payload := map[string]any{
		"scope":            summary.Scope,
		"entity":           summary.Entity,
		"period":           summary.Period,
		"period_from":      summary.PeriodFrom,
		"period_to":        summary.PeriodTo,
		"requested_period": summary.RequestedPeriod,
		"period_adjusted":  summary.PeriodAdjusted,
		"contract_count":   summary.ContractCount,
		"coverage":         buildContractAggregateCoverage(selection, summary),
	}
	if selection.IncludeRevenue {
		payload["revenue_settlement"] = round2(summary.RevenueSettlement)
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["invoice_amount"] = round2(summary.RevenueInvoiced)
		if summary.RevenueComparison != nil {
			payload["period_comparison"] = buildContractAggregateComparisonPayload(*summary.RevenueComparison)
		}
		if len(summary.RevenueCustomerRanking) > 0 {
			payload["revenue_customer_ranking"] = buildContractAggregateDimensionPayload(summary.RevenueCustomerRanking, "customer_name")
			payload["top_revenue_share"] = summary.TopRevenueShare
			payload["top2_revenue_share"] = summary.Top2RevenueShare
			payload["top2_revenue_settlement"] = round2(summary.Top2RevenueSettlement)
		}
		if len(summary.RevenueItems) > 0 {
			payload["revenue_items"] = buildRevenueItemPayload(summary.RevenueItems)
		}
	}
	if selection.IncludeReceivable {
		payload["revenue_settlement"] = round2(summary.RevenueSettlement)
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["receivable_amount"] = round2(summary.RevenueReceivable)
		payload["invoiced_unreceived_amount"] = round2(summary.RevenueInvoiceOpen)
		if len(summary.RevenueItems) > 0 {
			payload["receivable_open_items"] = buildRevenueItemPayload(filterOpenContractAggregateItems(summary.RevenueItems))
		}
		if len(summary.RevenueOpenRanking) > 0 {
			payload["receivable_open_customer_ranking"] = buildContractAggregateDimensionPayload(summary.RevenueOpenRanking, "customer_name")
		}
		if len(summary.RevenueOpenBuckets) > 0 {
			payload["receivable_open_customer_period_buckets"] = buildContractAggregateOpenBucketPayload(summary.RevenueOpenBuckets)
		}
	}
	if selection.IncludeInvoiceAR {
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["invoice_amount"] = round2(summary.RevenueInvoiced)
		payload["invoiced_unreceived_amount"] = round2(summary.RevenueInvoiceOpen)
		payload["invoice_open_items"] = buildRevenueInvoiceOpenItemPayload(summary.RevenueInvoiceOpenItems)
	}
	if selection.IncludeCost {
		payload["cost_settlement"] = round2(summary.CostSettlement)
		payload["cost_paid"] = round2(summary.CostPaid)
		if len(summary.CostSupplierRanking) > 0 {
			payload["cost_supplier_ranking"] = buildContractAggregateDimensionPayload(summary.CostSupplierRanking, "supplier_name")
		}
		if len(summary.CostItems) > 0 {
			payload["cost_items"] = buildCostItemPayload(summary.CostItems)
		}
	}
	if selection.IncludePayable {
		payload["cost_settlement"] = round2(summary.CostSettlement)
		payload["cost_paid"] = round2(summary.CostPaid)
		payload["payable_amount"] = round2(summary.CostPayable)
		payload["invoiced_unpaid_amount"] = round2(summary.CostInvoiceOpen)
	}
	if selection.IncludeInvoiceAP {
		payload["cost_paid"] = round2(summary.CostPaid)
		payload["invoice_amount"] = round2(summary.CostInvoiced)
		payload["invoiced_unpaid_amount"] = round2(summary.CostInvoiceOpen)
		payload["invoice_unpaid_items"] = buildCostInvoiceOpenItemPayload(summary.CostInvoiceOpenItems)
	}
	if selection.IncludeProfit {
		payload["profit"] = round2(summary.Profit)
		payload["gross_margin"] = round2(summary.Profit)
		payload["gross_margin_basis"] = "项目结算收入-项目成本"
		if summary.HasNetProfitContext {
			payload["net_profit_context"] = round2(summary.NetProfitContext)
			payload["net_profit_context_source"] = summary.NetProfitContextSource
		}
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["cost_paid"] = round2(summary.CostPaid)
	}
	return payload
}

func buildContractAggregateOpenBucketPayload(buckets []contractAggregateOpenBucket) []map[string]any {
	payload := make([]map[string]any, 0, len(buckets))
	for _, bucket := range buckets {
		payload = append(payload, map[string]any{
			"customer_name":        bucket.Name,
			"prior_label":          bucket.PriorLabel,
			"prior_from":           bucket.PriorFrom,
			"prior_to":             bucket.PriorTo,
			"prior_open_amount":    round2(bucket.PriorOpen),
			"current_label":        bucket.CurrentLabel,
			"current_from":         bucket.CurrentFrom,
			"current_to":           bucket.CurrentTo,
			"current_open_amount":  round2(bucket.CurrentOpen),
			"combined_open_amount": round2(bucket.TotalOpen),
		})
	}
	return payload
}

func buildContractAggregateComparisonPayload(cmp contractAggregatePeriodComparison) map[string]any {
	return map[string]any{
		"current_label":            cmp.CurrentLabel,
		"current_from":             cmp.CurrentFrom,
		"current_to":               cmp.CurrentTo,
		"current_revenue":          round2(cmp.CurrentRevenue),
		"baseline_label":           cmp.BaselineLabel,
		"baseline_from":            cmp.BaselineFrom,
		"baseline_to":              cmp.BaselineTo,
		"baseline_revenue":         round2(cmp.BaselineRevenue),
		"baseline_monthly_average": round2(cmp.BaselineMonthlyAverage),
		"difference_vs_average":    round2(cmp.DifferenceVsAverage),
		"ratio_vs_average":         cmp.RatioVsAverage,
	}
}

func buildContractAggregateDimensionPayload(rows []contractAggregateDimensionRow, nameKey string) []map[string]any {
	payload := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		payload = append(payload, map[string]any{
			nameKey:             row.Name,
			"settlement_amount": round2(row.SettlementAmount),
			"invoice_amount":    round2(row.InvoiceAmount),
			"movement_amount":   round2(row.MovementAmount),
			"open_amount":       round2(row.OpenAmount),
			"share":             row.Share,
		})
	}
	return payload
}

func buildRevenueItemPayload(items []contractAggregateOpenItem) []map[string]any {
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, map[string]any{
			"customer_name":     item.CustomerName,
			"contract_content":  item.ContractContent,
			"settlement_amount": round2(item.SettlementAmount),
			"invoice_amount":    round2(item.InvoiceAmount),
			"received_amount":   round2(item.ReceivedAmount),
			"unreceived_amount": round2(item.OpenAmount),
		})
	}
	return payload
}

func filterOpenContractAggregateItems(items []contractAggregateOpenItem) []contractAggregateOpenItem {
	out := make([]contractAggregateOpenItem, 0, len(items))
	for _, item := range items {
		if item.OpenAmount > 0 {
			out = append(out, item)
		}
	}
	return out
}

func buildCostItemPayload(items []contractAggregateOpenItem) []map[string]any {
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, map[string]any{
			"supplier_name":     item.CustomerName,
			"contract_content":  item.ContractContent,
			"settlement_amount": round2(item.SettlementAmount),
			"invoice_amount":    round2(item.InvoiceAmount),
			"paid_amount":       round2(item.ReceivedAmount),
			"unpaid_amount":     round2(item.OpenAmount),
		})
	}
	return payload
}

func buildCostInvoiceOpenItemPayload(items []contractAggregateOpenItem) []map[string]any {
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, map[string]any{
			"supplier_name":    item.CustomerName,
			"contract_content": item.ContractContent,
			"invoice_amount":   round2(item.InvoiceAmount),
			"paid_amount":      round2(item.ReceivedAmount),
			"open_amount":      round2(item.OpenAmount),
		})
	}
	return payload
}

func buildRevenueInvoiceOpenItemPayload(items []contractAggregateOpenItem) []map[string]any {
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, map[string]any{
			"customer_name":    item.CustomerName,
			"contract_content": item.ContractContent,
			"invoice_amount":   round2(item.InvoiceAmount),
			"received_amount":  round2(item.ReceivedAmount),
			"open_amount":      round2(item.OpenAmount),
		})
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
	if selection.IncludeReceivable {
		coverage["应收"] = summary.HasRevenueCoverage
	}
	if selection.IncludePayable {
		coverage["应付"] = summary.HasCostCoverage
	}
	if selection.IncludeInvoiceAR {
		coverage["已开票未回款"] = summary.HasRevenueCoverage
	}
	if selection.IncludeInvoiceAP {
		coverage["已收票未付款"] = summary.HasCostCoverage
	}
	return coverage
}

func buildContractAggregateMoneyView(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	view := map[string]any{
		"说明": "项目现金口径",
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
	case selection.IncludeReceivable && !selection.IncludePayable:
		view["已到账"] = round2(summary.RevenueReceived)
	case selection.IncludePayable && !selection.IncludeReceivable:
		view["已付款"] = round2(summary.CostPaid)
	case selection.IncludeInvoiceAR && !selection.IncludeInvoiceAP:
		view["已到账"] = round2(summary.RevenueReceived)
	case selection.IncludeInvoiceAP && !selection.IncludeInvoiceAR:
		view["已付款"] = round2(summary.CostPaid)
	default:
		if selection.IncludeRevenue {
			view["回款"] = round2(summary.RevenueReceived)
		}
		if selection.IncludeReceivable {
			view["已到账"] = round2(summary.RevenueReceived)
		}
		if selection.IncludeCost {
			view["付款"] = round2(summary.CostPaid)
		}
		if selection.IncludePayable {
			view["已付款"] = round2(summary.CostPaid)
		}
		if selection.IncludeInvoiceAR {
			view["已到账"] = round2(summary.RevenueReceived)
		}
		if selection.IncludeInvoiceAP {
			view["已付款"] = round2(summary.CostPaid)
		}
		if selection.IncludeProfit || (selection.IncludeRevenue && selection.IncludeCost) {
			view["净现金"] = round2(summary.RevenueReceived - summary.CostPaid)
		}
	}
	return view
}

func buildContractAggregateAccountView(selection contractAggregateSelection, summary contractAggregateSummary) map[string]any {
	view := map[string]any{
		"说明": "项目经营口径",
	}
	if selection.IncludeRevenue {
		view["营收"] = round2(summary.RevenueSettlement)
		view["已开票"] = round2(summary.RevenueInvoiced)
	}
	if selection.IncludeCost {
		view["项目成本"] = round2(summary.CostSettlement)
	}
	if selection.IncludeProfit {
		view["利润"] = round2(summary.Profit)
		view["项目毛利"] = round2(summary.Profit)
		if summary.HasNetProfitContext {
			view["财报净利"] = round2(summary.NetProfitContext)
		}
	}
	if selection.IncludeReceivable {
		view["项目应收"] = round2(summary.RevenueReceivable)
		view["已开票未回款"] = round2(summary.RevenueInvoiceOpen)
	}
	if selection.IncludePayable {
		view["项目应付"] = round2(summary.CostPayable)
		view["已收票未付款"] = round2(summary.CostInvoiceOpen)
	}
	if selection.IncludeInvoiceAR {
		view["已开票未回款"] = round2(summary.RevenueInvoiceOpen)
	}
	if selection.IncludeInvoiceAP {
		view["已收票未付款"] = round2(summary.CostInvoiceOpen)
	}
	return view
}
