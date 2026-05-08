package query

import "fmt"

func (a *CoreMetricsSourceAdapter) fetchFactSet(spec QuerySpec) (FactSet, error) {
	cfg := a.runtime.currentRuleConfig()
	request := resolveCoreMetricRequestWithConfig(spec.OriginalQuestion, metricDisplayName(string(spec.MetricKind)), cfg)
	coverage := a.runtime.resolveCoreMetricCoverageForRequest(spec.PeriodFrom, spec.PeriodTo, request)
	if !coverage.HasData {
		return buildCoreMetricsMissingFactSet(spec, coverage), nil
	}

	unified, sqls, logs, err := a.runtime.computeUnifiedCoreMetrics(coverage.ActualFrom, coverage.ActualTo)
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
	return attachCoreMetricsSnapshotTrace(buildCoreMetricsFactSet(spec, coverage, unified, sqls, logs), snapshot), nil
}
