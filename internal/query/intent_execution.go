package query

type intentExecutionHandler func(*Engine, queryExecutionContext) Result

func intentExecutionHandlers() map[Intent]intentExecutionHandler {
	return map[Intent]intentExecutionHandler{
		IntentHostPayload: func(e *Engine, ctx queryExecutionContext) Result {
			return e.queryHostLLMPayload(ctx.q, ctx.from, ctx.to)
		},
		IntentIdentityQuery: func(e *Engine, ctx queryExecutionContext) Result {
			return e.queryIdentityResult(ctx.entity)
		},
		IntentARAPQuery: func(e *Engine, ctx queryExecutionContext) Result {
			return e.queryARAP(ctx.q, ctx.entity, ctx.from, ctx.to)
		},
		IntentLargeTransactionQuery: func(e *Engine, ctx queryExecutionContext) Result {
			return e.queryLargeBankTransactions(ctx.q, ctx.from, ctx.to)
		},
		IntentTaxQuery: func(e *Engine, ctx queryExecutionContext) Result {
			return e.queryTax(ctx.q, ctx.from, ctx.to)
		},
		IntentMonthlySummary: func(e *Engine, ctx queryExecutionContext) Result {
			return e.queryMonthlySummary(ctx.q, ctx.from, ctx.to)
		},
		IntentAnalysis: func(e *Engine, ctx queryExecutionContext) Result {
			return e.queryAnalysis(ctx.to)
		},
		IntentFallback: func(e *Engine, ctx queryExecutionContext) Result {
			return e.queryFallback(ctx.q, ctx.from, ctx.to, "")
		},
	}
}

func (e *Engine) executeIntentRoute(ctx queryExecutionContext) Result {
	if handler, ok := intentExecutionHandlers()[ctx.intent]; ok {
		return handler(e, ctx)
	}
	return e.queryPrecise(ctx.q, ctx.to)
}
