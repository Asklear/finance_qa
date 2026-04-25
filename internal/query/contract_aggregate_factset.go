package query

import "strings"

func buildContractAggregateFactSet(spec QuerySpec, summary contractAggregateSummary) FactSet {
	canAnswer := contractAggregateCanAnswer(summary.RequestedMetrics, summary)
	fallbackReason := contractAggregateFallbackReason(summary.RequestedMetrics, summary)
	resultMessage, resultData := buildContractAggregateResultSnapshot(spec, summary)
	tracePayload := map[string]any{
		"scope":             summary.Scope,
		"entity":            summary.Entity,
		"period":            summary.Period,
		"requested_metrics": append([]string{}, summary.RequestedMetrics...),
		"source_tables":     append([]string{}, summary.SourceTables...),
		"executed_sql":      append([]string{}, summary.ExecutedSQL...),
		"calculation_logs":  append([]string{}, summary.CalculationLogs...),
		"coverage": map[string]any{
			"收入":     summary.HasRevenueCoverage,
			"成本":     summary.HasCostCoverage,
			"利润":     summary.HasRevenueCoverage && summary.HasCostCoverage,
			"应收":     summary.HasRevenueCoverage,
			"应付":     summary.HasCostCoverage,
			"已开票未回款": summary.HasRevenueCoverage,
			"已收票未付款": summary.HasCostCoverage,
		},
		"can_answer":      canAnswer,
		"fallback_reason": fallbackReason,
		"result_message":  resultMessage,
		"result_data":     resultData,
	}

	facts := make([]Fact, 0, 6)
	appendFact := func(metricKey string, value float64, covered bool) {
		facts = append(facts, Fact{
			Source:         "contract_aggregate",
			MetricKey:      metricKey,
			Entity:         summary.Entity,
			PeriodFrom:     spec.PeriodFrom,
			PeriodTo:       spec.PeriodTo,
			Value:          round2(value),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: coverageStatusFromBool(covered),
			Confidence:     1,
			TracePayload:   tracePayload,
		})
	}

	includeRevenue := contractAggregateIncludesMetric(summary.RequestedMetrics, "收入")
	includeCost := contractAggregateIncludesMetric(summary.RequestedMetrics, "成本")
	includeProfit := contractAggregateIncludesMetric(summary.RequestedMetrics, "利润")
	includeReceivable := contractAggregateIncludesMetric(summary.RequestedMetrics, "应收")
	includePayable := contractAggregateIncludesMetric(summary.RequestedMetrics, "应付")
	includeInvoiceAR := contractAggregateIncludesMetric(summary.RequestedMetrics, "已开票未回款")
	includeInvoiceAP := contractAggregateIncludesMetric(summary.RequestedMetrics, "已收票未付款")

	if includeRevenue {
		appendFact("contract_aggregate_revenue", summary.RevenueSettlement, summary.HasRevenueCoverage)
		appendFact("contract_aggregate_cash_received", summary.RevenueReceived, summary.HasRevenueCoverage)
	}
	if includeCost {
		appendFact("contract_aggregate_cost", summary.CostSettlement, summary.HasCostCoverage)
		appendFact("contract_aggregate_cash_paid", summary.CostPaid, summary.HasCostCoverage)
	}
	if includeProfit {
		appendFact("contract_aggregate_profit", summary.Profit, summary.HasRevenueCoverage && summary.HasCostCoverage)
		appendFact("contract_aggregate_cash_net", summary.RevenueReceived-summary.CostPaid, summary.HasRevenueCoverage && summary.HasCostCoverage)
	}
	if includeReceivable {
		appendFact("contract_aggregate_receivable", summary.RevenueReceivable, summary.HasRevenueCoverage)
		appendFact("contract_aggregate_invoiced_unreceived", summary.RevenueInvoiceOpen, summary.HasRevenueCoverage)
	}
	if includePayable {
		appendFact("contract_aggregate_payable", summary.CostPayable, summary.HasCostCoverage)
		appendFact("contract_aggregate_invoiced_unpaid", summary.CostInvoiceOpen, summary.HasCostCoverage)
	}
	if includeInvoiceAR {
		appendFact("contract_aggregate_invoiced_unreceived", summary.RevenueInvoiceOpen, summary.HasRevenueCoverage)
	}
	if includeInvoiceAP {
		appendFact("contract_aggregate_invoiced_unpaid", summary.CostInvoiceOpen, summary.HasCostCoverage)
	}

	return FactSet{
		Source: "contract_aggregate",
		Facts:  facts,
	}
}

func buildContractAggregateMissingFactSet(spec QuerySpec, reason string) FactSet {
	if strings.TrimSpace(reason) == "" {
		reason = "合同/项目汇总表暂不可用"
	}
	tracePayload := map[string]any{
		"requested_metrics": detectRequestedMetrics(spec.OriginalQuestion),
		"can_answer":        false,
		"fallback_reason":   reason,
		"source_tables":     getRuleConfig().ContractSourceTables(contractAggregateRole),
	}
	return FactSet{
		Source: "contract_aggregate",
		Facts: []Fact{
			{
				Source:         "contract_aggregate",
				MetricKey:      "contract_aggregate_missing",
				Entity:         spec.Entity,
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				AuthorityLevel: AuthoritySupporting,
				CoverageStatus: CoverageMissing,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
		},
	}
}

func coverageStatusFromBool(ok bool) CoverageStatus {
	if ok {
		return CoverageFull
	}
	return CoverageMissing
}
