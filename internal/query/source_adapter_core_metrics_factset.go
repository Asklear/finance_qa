package query

import "strings"

func buildCoreMetricsFactSet(spec QuerySpec, coverage coreMetricCoverage, unified *unifiedCoreMetrics, sqls, logs []string) FactSet {
	const sourceName = "core_metrics"
	authority := AuthorityDerived
	if strings.Contains(unified.AccrualFrom, "income_statement") {
		authority = AuthorityOfficial
	}

	tracePayload := buildCoreMetricsTracePayload(spec, coverage, unified, sqls, logs)
	facts := make([]Fact, 0, 8)
	appendFact := func(metricKey string, value float64, authorityLevel AuthorityLevel) {
		facts = append(facts, Fact{
			Source:         sourceName,
			MetricKey:      metricKey,
			Entity:         spec.Entity,
			PeriodFrom:     coverage.ActualFrom,
			PeriodTo:       coverage.ActualTo,
			Value:          round2(value),
			AuthorityLevel: authorityLevel,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		})
	}

	appendFact("cash_receipts", unified.Cash.Income, AuthoritySupporting)
	appendFact("cash_payments", unified.Cash.Expense, AuthoritySupporting)
	appendFact("accrual_revenue", unified.Accrual.Revenue, authority)
	appendFact("accrual_cost", unified.Accrual.TotalCost, authority)
	appendFact("accrual_profit", unified.Accrual.NetProfit, authority)
	appendFact("accrual_total_profit", unified.Accrual.Profit, authority)

	if unified.Bridge != nil {
		appendFact("cash_bridge_adjusted_operating_cash", bridgeAdjustedEstimatedCash(unified.Bridge), AuthorityDerived)
		appendFact("cash_bridge_bank_net_cash", unified.Bridge.BankNetCash, AuthorityDerived)
	}

	return FactSet{
		Source: sourceName,
		Facts:  facts,
	}
}
