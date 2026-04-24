package query

func (e *Engine) queryFallback(q, from, to, err string) Result {
	if r := e.ruleFallback(q, from, to); r.Success {
		return r
	}
	return e.buildAmbiguousFallbackResult(q, from, to)
}

func (e *Engine) ruleFallback(q, from, to string) Result {
	cfg := getRuleConfig()
	entity := e.extractNamedEntity(q)
	hasRealEntity := e.isRealBusinessEntity(q, entity)
	switch resolveFallbackRoute(fallbackRouteContext{
		q:             q,
		entity:        entity,
		hasRealEntity: hasRealEntity,
		from:          from,
		to:            to,
		cfg:           cfg,
	}) {
	case fallbackRouteReconciliation:
		return e.queryReconciliation(q, from, to)
	case fallbackRouteCoreMetric:
		return e.queryDualPerspectiveForCoreMetric(q, from, to)
	case fallbackRouteSupplierPayments:
		return e.querySupplierPayments(from, to)
	case fallbackRouteHRBreakdown:
		return e.queryHRBreakdown(from, to)
	case fallbackRouteMonthlyExpense:
		return e.queryMonthlyExpenseFromBank(from, to)
	case fallbackRouteEntityReadiness:
		return e.queryEntityDataReady(entity, from, to)
	case fallbackRouteProjectIncomeCost:
		return e.queryProjectIncomeCost(entity, from, to, q)
	case fallbackRouteCounterpartyAmount:
		return e.queryCounterpartyAmountFallback(q, entity, from, to)
	default:
		return Result{Success: false}
	}
}
