package query

import (
	"strings"
	"time"
)

func buildQuerySpec(question string, anchor time.Time, cfg RuleConfig) QuerySpec {
	q := NormalizeQuestion(question)
	intent, _ := classifyIntentV2(q, cfg)

	from, to := ExtractPeriodWithNow(q, anchor)
	isContractDetail := shouldUseContractDetailQuestion(q)
	needsContractDimension := !isContractDetail && !shouldUseExpenseBreakdownWithConfig(q, cfg) && shouldUseContractDimensionWithConfig(q, cfg)
	if needsContractDimension {
		from, to = extractContractQuestionPeriods(q, anchor)
	}
	subPeriod, _ := extractReceiptSubPeriod(q, from, to)
	entity := extractNamedEntityFromQuestion(q)
	if looksLikeBossRewriteNonEntity(entity) {
		entity = ""
	}
	if shouldForceCompanyScopeContractAggregateWithConfig(q, cfg) {
		entity = ""
	}
	rewrite := RewriteBossQueryWithConfig(q, anchor, cfg)

	spec := QuerySpec{
		OriginalQuestion:       question,
		NormalizedQuestion:     q,
		Intent:                 intent,
		QueryFamily:            detectQueryFamily(q, intent, entity, from, to, cfg, needsContractDimension),
		MetricKind:             detectMetricKind(q, cfg),
		Entity:                 entity,
		PeriodFrom:             from,
		PeriodTo:               to,
		SubPeriod:              subPeriod,
		TimeScope:              detectTimeScope(q, from, to, anchor),
		BossRewrite:            rewrite,
		SourceConstraint:       rewrite.SourceConstraint,
		NeedsContractDimension: needsContractDimension,
		LexiconProfile:         "rules_config",
		AsOf:                   anchor.Format("2006-01-02"),
	}
	return applyQuerySpecPolicy(spec, deriveQuerySpecPolicy(spec, cfg))
}

func reconcileQuerySpec(spec QuerySpec, resolvedEntity string, cfg RuleConfig) QuerySpec {
	spec.Entity = strings.TrimSpace(resolvedEntity)
	if looksLikeBossRewriteNonEntity(spec.Entity) {
		spec.Entity = ""
	}

	spec.QueryFamily = detectQueryFamily(
		spec.NormalizedQuestion,
		spec.Intent,
		spec.Entity,
		spec.PeriodFrom,
		spec.PeriodTo,
		cfg,
		spec.NeedsContractDimension,
	)
	return applyQuerySpecPolicy(spec, deriveQuerySpecPolicy(spec, cfg))
}
