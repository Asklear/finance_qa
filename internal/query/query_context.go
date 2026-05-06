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
	if spec.QueryFamily == QueryFamilyContractDetail {
		return false
	}
	seedEntity := strings.TrimSpace(spec.Entity)
	meaningfulSeedEntity := seedEntity != "" && !looksLikeSyntheticQuestionFragment(seedEntity) && !looksLikeAccountFragment(seedEntity) && !looksLikePeriodOnlyEntity(seedEntity)
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
	route := e.resolveQueryRouting(question)

	return queryExecutionContext{
		engine:        e,
		question:      question,
		q:             route.normalizedQuestion,
		intent:        route.intent,
		spec:          route.spec,
		traceMap:      route.traceMap,
		anchor:        route.anchor,
		from:          route.spec.PeriodFrom,
		to:            route.spec.PeriodTo,
		cfg:           route.cfg,
		entity:        route.entity,
		hasRealEntity: route.hasRealEntity,
	}
}
