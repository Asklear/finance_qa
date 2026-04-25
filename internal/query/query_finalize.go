package query

import (
	"fmt"
	"strings"
)

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
	envelope := map[string]any{
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
		"source_constraint":             spec.SourceConstraint,
		"lexicon_profile":               spec.LexiconProfile,
	}
	if spec.BossRewrite.Metric != "" {
		envelope["boss_rewrite"] = buildBossRewriteEnvelope(spec.BossRewrite)
	}
	return envelope
}

func buildBossRewriteEnvelope(rewrite BossQueryRewrite) map[string]any {
	return map[string]any{
		"metric":                rewrite.Metric,
		"scope":                 rewrite.Scope,
		"entity":                rewrite.Entity,
		"period_from":           rewrite.PeriodFrom,
		"period_to":             rewrite.PeriodTo,
		"sub_period":            rewrite.SubPeriod,
		"granularity":           rewrite.Granularity,
		"perspective":           rewrite.Perspective,
		"source_constraint":     rewrite.SourceConstraint,
		"requires_source_probe": rewrite.RequiresSourceProbe,
	}
}

func applyQuerySpecOverrides(spec QuerySpec, data map[string]any) QuerySpec {
	if data == nil {
		return spec
	}
	raw, ok := data["query_spec_overrides"].(map[string]any)
	if !ok {
		return spec
	}
	overrides := cloneMap(raw)
	if len(overrides) == 0 {
		return spec
	}
	if periodFrom := strings.TrimSpace(anyToString(overrides["period_from"])); periodFrom != "" {
		spec.PeriodFrom = periodFrom
	}
	if periodTo := strings.TrimSpace(anyToString(overrides["period_to"])); periodTo != "" {
		spec.PeriodTo = periodTo
	}
	if subPeriod := strings.TrimSpace(anyToString(overrides["sub_period"])); subPeriod != "" {
		spec.SubPeriod = subPeriod
	}
	if timeScope := strings.TrimSpace(anyToString(overrides["time_scope"])); timeScope != "" {
		spec.TimeScope = TimeScope(timeScope)
	}
	if perspective := strings.TrimSpace(anyToString(overrides["perspective_policy"])); perspective != "" {
		spec.PerspectivePolicy = PerspectivePolicy(perspective)
	}
	return spec
}

func finalizeQueryResult(ctx queryExecutionContext, r Result) Result {
	if r.Data == nil {
		r.Data = map[string]any{}
	}
	r.Data["intent_trace"] = ctx.traceMap
	r.Data["query_spec"] = buildQuerySpecEnvelope(applyQuerySpecOverrides(ctx.spec, r.Data))
	if ctx.engine != nil {
		r = ctx.engine.annotateSourceAttribution(ctx.spec, r)
	}
	return r.withTraceData()
}

func (ctx queryExecutionContext) finalize(r Result) Result {
	return finalizeQueryResult(ctx, r)
}
