package query

func buildSupplierPaymentTracePayload(summary supplierPaymentSummary) map[string]any {
	return map[string]any{
		"period":       summary.Period,
		"suppliers":    cloneAny(summary.Suppliers),
		"excluded":     cloneAny(summary.Excluded),
		"executed_sql": append([]string{}, summary.ExecutedSQL...),
		"logs":         append([]string{}, summary.Logs...),
	}
}

func buildSupplierPaymentFactSet(spec QuerySpec, summary supplierPaymentSummary) FactSet {
	tracePayload := buildSupplierPaymentTracePayload(summary)

	facts := []Fact{
		{
			Source:         "supplier_payments",
			MetricKey:      "supplier_payment_count",
			Entity:         spec.Entity,
			PeriodFrom:     spec.PeriodFrom,
			PeriodTo:       spec.PeriodTo,
			Value:          float64(len(summary.Suppliers)),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         "supplier_payments",
			MetricKey:      "supplier_payment_total",
			Entity:         spec.Entity,
			PeriodFrom:     spec.PeriodFrom,
			PeriodTo:       spec.PeriodTo,
			Value:          summary.Total,
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         "supplier_payments",
			MetricKey:      "supplier_payment_roster",
			Entity:         spec.Entity,
			PeriodFrom:     spec.PeriodFrom,
			PeriodTo:       spec.PeriodTo,
			Value:          summary.Total,
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
	}

	return FactSet{
		Source: "supplier_payments",
		Facts:  facts,
	}
}
