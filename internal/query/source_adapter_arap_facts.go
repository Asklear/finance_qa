package query

func buildARAPFactsFromResult(spec QuerySpec, scope arapScope, result Result, official bool) []Fact {
	return buildARAPFactsFromData(spec, scope, result.Data, official)
}

func resolveARAPMetricLabel(spec QuerySpec, scope arapScope) string {
	if len(detectARAPScopes(spec)) > 1 {
		return scope.typ
	}
	return scope.metricLabel
}

func buildARAPFactsFromData(spec QuerySpec, scope arapScope, data map[string]any, official bool) []Fact {
	if data == nil {
		return nil
	}
	if source := anyToString(data["source"]); source == "balance_sheet" || source == "journal_entity_rollforward" {
		official = true
	}
	authority := AuthoritySupporting
	sourceName := "journal_open_items"
	if official {
		authority = AuthorityOfficial
		sourceName = "balance_sheet"
	}
	label := resolveARAPMetricLabel(spec, scope)
	tracePayload := map[string]any{
		"result_source": data["source"],
		"period":        spec.PeriodTo,
		"entity":        spec.Entity,
		"result_data":   cloneMap(data),
	}
	facts := []Fact{
		{
			Source:         sourceName,
			MetricKey:      "official_" + label + "_total",
			Entity:         spec.Entity,
			PeriodFrom:     spec.PeriodTo,
			PeriodTo:       spec.PeriodTo,
			Value:          anyToFloat64(data["total"]),
			AuthorityLevel: authority,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
	}
	if opening := anyToFloat64(data["opening_balance"]); opening != 0 || official {
		facts = append(facts, Fact{
			Source:         sourceName,
			MetricKey:      "official_" + label + "_opening_balance",
			Entity:         spec.Entity,
			PeriodFrom:     spec.PeriodTo,
			PeriodTo:       spec.PeriodTo,
			Value:          opening,
			AuthorityLevel: authority,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		})
	}
	if !official {
		facts = append(facts, buildARAPOpenItemFacts(spec, scope, data)...)
	}
	return facts
}

func buildARAPOpenItemFacts(spec QuerySpec, scope arapScope, data map[string]any) []Fact {
	suffix := ""
	if resolveARAPMetricLabel(spec, scope) != scope.metricLabel {
		suffix = "_" + scope.typ
	}
	tracePayload := map[string]any{
		"result_source": data["source"],
		"period":        spec.PeriodTo,
		"entity":        spec.Entity,
		"result_data":   cloneMap(data),
	}
	return []Fact{
		{
			Source:         "journal_open_items",
			MetricKey:      "openitem" + suffix + "_closing_total",
			Entity:         spec.Entity,
			PeriodFrom:     spec.PeriodTo,
			PeriodTo:       spec.PeriodTo,
			Value:          anyToFloat64(data["total"]),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         "journal_open_items",
			MetricKey:      "openitem" + suffix + "_historical_settlement",
			Entity:         spec.Entity,
			PeriodFrom:     spec.PeriodTo,
			PeriodTo:       spec.PeriodTo,
			Value:          anyToFloat64(data["historical_settlement"]),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
		{
			Source:         "journal_open_items",
			MetricKey:      "openitem" + suffix + "_current_settlement",
			Entity:         spec.Entity,
			PeriodFrom:     spec.PeriodTo,
			PeriodTo:       spec.PeriodTo,
			Value:          anyToFloat64(data["current_period_settlement"]),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
	}
}
