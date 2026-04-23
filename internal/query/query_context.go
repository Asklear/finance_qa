package query

import (
	"strings"
	"time"
)

type queryExecutionContext struct {
	engine   *Engine
	question string
	q        string
	intent   Intent
	spec     QuerySpec
	traceMap map[string]any
	anchor   time.Time
	from     string
	to       string
	cfg      RuleConfig
	entity   string

	hasRealEntity bool
}

func shouldResolveEntityDeeply(spec QuerySpec) bool {
	seedEntity := strings.TrimSpace(spec.Entity)
	meaningfulSeedEntity := seedEntity != "" && !looksLikeSyntheticQuestionFragment(seedEntity) && !looksLikeAccountFragment(seedEntity)
	if meaningfulSeedEntity {
		return true
	}
	switch spec.QueryFamily {
	case QueryFamilyHRCost:
		return false
	case QueryFamilyCounterparty, QueryFamilyContractDimension, QueryFamilyReadiness:
		return true
	case QueryFamilyARAP:
		return strings.Contains(spec.NormalizedQuestion, "项目") && meaningfulSeedEntity
	}
	switch spec.Intent {
	case IntentIdentityQuery:
		return true
	case IntentFallback:
		return meaningfulSeedEntity
	default:
		return false
	}
}

func (e *Engine) Query(question string) Result {
	ctx := e.prepareQueryExecutionContext(question)
	return e.executeQuery(ctx)
}

func (e *Engine) prepareQueryExecutionContext(question string) queryExecutionContext {
	q := NormalizeQuestion(question)
	resolved := ResolveCompanyMention(q, e.available)
	if resolved != "" && resolved != e.Company {
		e.Company = resolved
	}

	intent, intentTrace := ClassifyIntentV2(q)
	spec := BuildQuerySpec(q, e.getLatestPeriodAnchor())
	traceMap := map[string]any{
		"router_version": intentTrace.RouterVersion,
		"matched":        append([]string{}, intentTrace.Matched...),
		"scores":         intentTrace.Scores,
		"final_intent":   intentTrace.FinalIntent,
		"confidence":     intentTrace.Confidence,
	}

	entity := spec.Entity
	if shouldResolveEntityDeeply(spec) {
		entity = e.extractNamedEntity(q)
	}
	spec = reconcileQuerySpec(spec, entity, getRuleConfig())
	entity = spec.Entity
	hasRealEntity := e.isRealBusinessEntity(q, entity)
	if e.shouldPrioritizeContractQuery(q, entity, hasRealEntity) {
		if matched := e.matchContractSubjectByName(q); matched != "" {
			entity = matched
			spec.Entity = matched
		}
		spec.QueryFamily = QueryFamilyContractDimension
		spec.NeedsContractDimension = true
		spec.PerspectivePolicy = PerspectiveCashThenAccrual
		spec.PreferContractAggregate = false
		hasRealEntity = true
	}

	return queryExecutionContext{
		engine:        e,
		question:      question,
		q:             q,
		intent:        intent,
		spec:          spec,
		traceMap:      traceMap,
		anchor:        e.getLatestPeriodAnchor(),
		from:          spec.PeriodFrom,
		to:            spec.PeriodTo,
		cfg:           getRuleConfig(),
		entity:        entity,
		hasRealEntity: hasRealEntity,
	}
}
