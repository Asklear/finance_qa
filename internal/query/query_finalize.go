package query

import "fmt"

func (e *Engine) queryIdentityResult(entity string) Result {
	role, _ := e.detectEntityRole(entity)
	return Result{
		Success: true,
		Message: fmt.Sprintf("识别结果: [%s] 是 [%s]", entity, role),
		Data:    map[string]any{"entity": entity, "role": role},
		ExecutedSQL: []string{
			"detectEntityRole: SELECT SUM(debit_amount), SUM(credit_amount) FROM bank_statement WHERE counterparty_name LIKE ?",
			"detectEntityRole: SELECT account_code, summary FROM journal WHERE summary LIKE ? OR account_name LIKE ?",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[身份识别] entity=%s role=%s", entity, role),
		},
	}
}

func buildQuerySpecEnvelope(spec QuerySpec) map[string]any {
	return map[string]any{
		"query_family":                  spec.QueryFamily,
		"metric_kind":                   spec.MetricKind,
		"entity":                        spec.Entity,
		"period_from":                   spec.PeriodFrom,
		"period_to":                     spec.PeriodTo,
		"sub_period":                    spec.SubPeriod,
		"time_scope":                    spec.TimeScope,
		"perspective_policy":            spec.PerspectivePolicy,
		"needs_contract_dimension":      spec.NeedsContractDimension,
		"prefer_contract_aggregate":     spec.PreferContractAggregate,
		"readiness_check_required":      spec.ReadinessCheckRequired,
		"authoritative_source_required": spec.AuthoritativeSourceRequired,
		"opening_period_aware":          spec.OpeningPeriodAware,
		"lexicon_profile":               spec.LexiconProfile,
	}
}

func finalizeQueryResult(ctx queryExecutionContext, r Result) Result {
	if r.Data == nil {
		r.Data = map[string]any{}
	}
	r.Data["intent_trace"] = ctx.traceMap
	r.Data["query_spec"] = buildQuerySpecEnvelope(ctx.spec)
	if ctx.engine != nil {
		r = ctx.engine.annotateSourceAttribution(ctx.spec, r)
	}
	return r.withTraceData()
}

func (ctx queryExecutionContext) finalize(r Result) Result {
	return finalizeQueryResult(ctx, r)
}
