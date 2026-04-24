package query

func (e *Engine) executeQuery(ctx queryExecutionContext) Result {
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
