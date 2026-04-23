package query

import (
	"context"
	"fmt"
	"strings"
)

type CoreMetricsSourceAdapter struct {
	engine *Engine
}

func NewCoreMetricsSourceAdapter(engine *Engine) *CoreMetricsSourceAdapter {
	return &CoreMetricsSourceAdapter{engine: engine}
}

func (a *CoreMetricsSourceAdapter) Name() string {
	return "core_metrics"
}

func (a *CoreMetricsSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{
		SourceCapabilityCashReceipts,
		SourceCapabilityBankCashReceipts,
		SourceCapabilityCashPayments,
		SourceCapabilityAccrualRevenue,
		SourceCapabilityAccrualCost,
		SourceCapabilityAccrualProfit,
		SourceCapabilityCashBridge,
	}
}

func (a *CoreMetricsSourceAdapter) Fetch(_ context.Context, spec QuerySpec) (FactSet, error) {
	coverage := a.engine.resolveCoreMetricCoverage(spec.PeriodFrom, spec.PeriodTo)
	if !coverage.HasData {
		return FactSet{
			Source: a.Name(),
			Facts: []Fact{
				{
					Source:         a.Name(),
					MetricKey:      "data_readiness",
					Entity:         spec.Entity,
					PeriodFrom:     spec.PeriodFrom,
					PeriodTo:       spec.PeriodTo,
					CoverageStatus: CoverageMissing,
					AuthorityLevel: AuthoritySupporting,
					TracePayload: map[string]any{
						"requested_from": spec.PeriodFrom,
						"requested_to":   spec.PeriodTo,
						"available_to":   coverage.AvailableTo,
						"truncated":      coverage.Truncated,
					},
				},
			},
		}, nil
	}

	unified, sqls, logs, err := a.engine.computeUnifiedCoreMetrics(coverage.ActualFrom, coverage.ActualTo)
	if err != nil {
		return FactSet{}, err
	}
	snapshot := buildCoreMetricDualSnapshot(spec.OriginalQuestion, spec, coverage, unified)
	logs = append(logs, fmt.Sprintf("[核心指标-默认双口径] metric=%s requested=%v cash=%.2f accrual=%.2f", snapshot.Metric, snapshot.RequestedMetrics, snapshot.CashValue, snapshot.AccrualValue))
	logs = append(logs, fmt.Sprintf("[覆盖范围] requested=%s actual=%s available_to=%s truncated=%t data_ready=true", displayPeriod(spec.PeriodFrom, spec.PeriodTo), unified.Period, coverage.AvailableTo, coverage.Truncated))
	sqls = appendUniqueStrings(sqls,
		"dual_perspective(cash): ComputeCashFlow over bank_statement in selected period",
		"dual_perspective(accrual): aggregate monthlyBookSummary across selected period and cross-check with income_statement.cumulative_amount when available",
		"coverage_guard: inspect latest available period across income_statement / balance_detail / journal / bank_statement",
	)
	factSet := buildCoreMetricsFactSet(spec, coverage, unified, sqls, logs)
	if len(factSet.Facts) > 0 && factSet.Facts[0].TracePayload != nil {
		factSet.Facts[0].TracePayload["result_message"] = snapshot.Message
		factSet.Facts[0].TracePayload["result_data"] = cloneMap(snapshot.Data)
	}
	return factSet, nil
}

func buildCoreMetricsFactSet(spec QuerySpec, coverage coreMetricCoverage, unified *unifiedCoreMetrics, sqls, logs []string) FactSet {
	const sourceName = "core_metrics"
	authority := AuthorityDerived
	if strings.Contains(unified.AccrualFrom, "income_statement") {
		authority = AuthorityOfficial
	}

	tracePayload := map[string]any{
		"coverage": map[string]any{
			"requested_from": spec.PeriodFrom,
			"requested_to":   spec.PeriodTo,
			"actual_from":    coverage.ActualFrom,
			"actual_to":      coverage.ActualTo,
			"available_to":   coverage.AvailableTo,
			"truncated":      coverage.Truncated,
		},
		"accrual_source":    unified.AccrualFrom,
		"range_validation":  unified.AccrualValidation,
		"consistency_guard": unified.Guard,
		"executed_sql":      append([]string{}, sqls...),
		"calculation_logs":  append([]string{}, logs...),
		"display_period":    unified.Period,
	}

	facts := []Fact{
		{
			Source:         sourceName,
			MetricKey:      "cash_receipts",
			Entity:         spec.Entity,
			PeriodFrom:     coverage.ActualFrom,
			PeriodTo:       coverage.ActualTo,
			Value:          round2(unified.Cash.Income),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         sourceName,
			MetricKey:      "cash_payments",
			Entity:         spec.Entity,
			PeriodFrom:     coverage.ActualFrom,
			PeriodTo:       coverage.ActualTo,
			Value:          round2(unified.Cash.Expense),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         sourceName,
			MetricKey:      "accrual_revenue",
			Entity:         spec.Entity,
			PeriodFrom:     coverage.ActualFrom,
			PeriodTo:       coverage.ActualTo,
			Value:          round2(unified.Accrual.Revenue),
			AuthorityLevel: authority,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         sourceName,
			MetricKey:      "accrual_cost",
			Entity:         spec.Entity,
			PeriodFrom:     coverage.ActualFrom,
			PeriodTo:       coverage.ActualTo,
			Value:          round2(unified.Accrual.TotalCost),
			AuthorityLevel: authority,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         sourceName,
			MetricKey:      "accrual_profit",
			Entity:         spec.Entity,
			PeriodFrom:     coverage.ActualFrom,
			PeriodTo:       coverage.ActualTo,
			Value:          round2(unified.Accrual.NetProfit),
			AuthorityLevel: authority,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         sourceName,
			MetricKey:      "accrual_total_profit",
			Entity:         spec.Entity,
			PeriodFrom:     coverage.ActualFrom,
			PeriodTo:       coverage.ActualTo,
			Value:          round2(unified.Accrual.Profit),
			AuthorityLevel: authority,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
	}
	if unified.Bridge != nil {
		facts = append(facts,
			Fact{
				Source:         sourceName,
				MetricKey:      "cash_bridge_adjusted_operating_cash",
				Entity:         spec.Entity,
				PeriodFrom:     coverage.ActualFrom,
				PeriodTo:       coverage.ActualTo,
				Value:          round2(bridgeAdjustedEstimatedCash(unified.Bridge)),
				AuthorityLevel: AuthorityDerived,
				CoverageStatus: CoverageFull,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
			Fact{
				Source:         sourceName,
				MetricKey:      "cash_bridge_bank_net_cash",
				Entity:         spec.Entity,
				PeriodFrom:     coverage.ActualFrom,
				PeriodTo:       coverage.ActualTo,
				Value:          round2(unified.Bridge.BankNetCash),
				AuthorityLevel: AuthorityDerived,
				CoverageStatus: CoverageFull,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
		)
	}

	return FactSet{
		Source: sourceName,
		Facts:  facts,
	}
}
