package query

func (e *Engine) executeQuery(ctx queryExecutionContext) Result {
	if result, spec, ok := e.tryCompoundSourceQuery(ctx); ok {
		ctx.spec = spec
		ctx.from = spec.PeriodFrom
		ctx.to = spec.PeriodTo
		ctx.entity = spec.Entity
		ctx.hasRealEntity = true
		return ctx.finalize(result)
	}

	var last Result
	for _, stage := range buildExecutionPlan(ctx) {
		result, ok := e.executeStage(stage, ctx)
		if ok {
			return ctx.finalize(result)
		}
		if result.Message != "" {
			last = result
		}
	}
	return e.finalizeExecutionResult(ctx, last)
}

func (e *Engine) shouldUseCounterpartyAuditFallback(ctx queryExecutionContext) bool {
	if !ctx.hasRealEntity {
		return false
	}
	return containsAny(ctx.q, counterpartyMetricKeywords(ctx.cfg))
}

func containsAmbiguityMessage(message string) bool {
	return containsAny(message, []string{"语义模糊"})
}
