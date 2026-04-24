package query

func buildCoreMetricsMissingFactSet(spec QuerySpec, coverage coreMetricCoverage) FactSet {
	return FactSet{
		Source: "core_metrics",
		Facts: []Fact{
			{
				Source:         "core_metrics",
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
	}
}
