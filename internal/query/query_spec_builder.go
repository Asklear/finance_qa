package query

import (
	"strings"
	"time"
)

func buildQuerySpec(question string, anchor time.Time, cfg RuleConfig) QuerySpec {
	q := NormalizeQuestion(question)
	intent, _ := classifyIntentV2(q, cfg)

	from, to := ExtractPeriodWithNow(q, anchor)
	needsContractDimension := shouldUseContractDimension(q)
	if needsContractDimension {
		from, to = extractContractQuestionPeriods(q, anchor)
	}
	subPeriod, _ := extractReceiptSubPeriod(q, from, to)
	entity := extractNamedEntityFromQuestion(q)

	spec := QuerySpec{
		OriginalQuestion:            question,
		NormalizedQuestion:          q,
		Intent:                      intent,
		QueryFamily:                 detectQueryFamily(q, intent, entity, from, to, cfg, needsContractDimension),
		MetricKind:                  detectMetricKind(q, cfg),
		Entity:                      entity,
		PeriodFrom:                  from,
		PeriodTo:                    to,
		SubPeriod:                   subPeriod,
		TimeScope:                   detectTimeScope(q, from, to, anchor),
		PerspectivePolicy:           detectPerspectivePolicy(q, intent, needsContractDimension, cfg),
		NeedsContractDimension:      needsContractDimension,
		PreferContractAggregate:     false,
		ReadinessCheckRequired:      strings.Contains(q, "数据出来"),
		OpeningPeriodAware:          isOpeningPeriodQuestion(q),
		AuthoritativeSourceRequired: isAuthoritativeSourceQuestion(q),
		LexiconProfile:              "rules_config",
	}
	spec.PreferContractAggregate = shouldPreferContractAggregate(q, intent, spec.QueryFamily, spec.MetricKind, cfg)

	if spec.QueryFamily == QueryFamilyARAP {
		spec.AuthoritativeSourceRequired = true
		spec.OpeningPeriodAware = true
		spec.PerspectivePolicy = PerspectiveOfficialThenEvidence
		spec.PreferContractAggregate = false
	}
	if spec.QueryFamily == QueryFamilyReadiness {
		spec.ReadinessCheckRequired = true
		spec.PreferContractAggregate = false
	}
	if spec.QueryFamily == QueryFamilyContractDimension {
		spec.NeedsContractDimension = true
		spec.PerspectivePolicy = PerspectiveCashThenAccrual
		spec.PreferContractAggregate = false
	}

	return spec
}

func reconcileQuerySpec(spec QuerySpec, resolvedEntity string, cfg RuleConfig) QuerySpec {
	if strings.TrimSpace(resolvedEntity) != "" {
		spec.Entity = strings.TrimSpace(resolvedEntity)
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
	spec.PerspectivePolicy = detectPerspectivePolicy(spec.NormalizedQuestion, spec.Intent, spec.NeedsContractDimension, cfg)
	spec.PreferContractAggregate = shouldPreferContractAggregate(spec.NormalizedQuestion, spec.Intent, spec.QueryFamily, spec.MetricKind, cfg)

	if spec.QueryFamily == QueryFamilyARAP {
		spec.AuthoritativeSourceRequired = true
		spec.OpeningPeriodAware = true
		spec.PerspectivePolicy = PerspectiveOfficialThenEvidence
		spec.PreferContractAggregate = false
	}
	if spec.QueryFamily == QueryFamilyReadiness {
		spec.ReadinessCheckRequired = true
		spec.PreferContractAggregate = false
	}
	if spec.QueryFamily == QueryFamilyContractDimension {
		spec.NeedsContractDimension = true
		spec.PerspectivePolicy = PerspectiveCashThenAccrual
		spec.PreferContractAggregate = false
	}

	return spec
}
