package query

func buildCoreMetricsTracePayload(spec QuerySpec, coverage coreMetricCoverage, unified *unifiedCoreMetrics, sqls, logs []string) map[string]any {
	return map[string]any{
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
}

func attachCoreMetricsSnapshotTrace(factSet FactSet, snapshot coreMetricDualSnapshot) FactSet {
	if len(factSet.Facts) == 0 || factSet.Facts[0].TracePayload == nil {
		return factSet
	}
	factSet.Facts[0].TracePayload["result_message"] = snapshot.Message
	factSet.Facts[0].TracePayload["result_data"] = cloneMap(snapshot.Data)
	return factSet
}
