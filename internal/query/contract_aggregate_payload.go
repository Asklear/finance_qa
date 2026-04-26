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
		"scope":          summary.Scope,
		"entity":         summary.Entity,
		"contract_count": summary.ContractCount,
		"coverage":       buildContractAggregateCoverage(selection, summary),
	}
	if selection.IncludeRevenue {
		payload["revenue_settlement"] = round2(summary.RevenueSettlement)
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["invoice_amount"] = round2(summary.RevenueInvoiced)
		if len(summary.RevenueItems) > 0 {
			payload["revenue_items"] = buildRevenueItemPayload(summary.RevenueItems)
		}
	}
	if selection.IncludeReceivable {
		payload["revenue_settlement"] = round2(summary.RevenueSettlement)
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["receivable_amount"] = round2(summary.RevenueReceivable)
		payload["invoiced_unreceived_amount"] = round2(summary.RevenueInvoiceOpen)
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
		payload["revenue_received"] = round2(summary.RevenueReceived)
		payload["cost_paid"] = round2(summary.CostPaid)
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
	if selection.IncludeReceivable {
		view["合同应收"] = round2(summary.RevenueReceivable)
		view["已开票未回款"] = round2(summary.RevenueInvoiceOpen)
	}
	if selection.IncludePayable {
		view["合同应付"] = round2(summary.CostPayable)
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
