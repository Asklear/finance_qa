package query

type counterpartyAnswerHandler func(*Engine, counterpartyAuditContext) (Result, bool)

func defaultCounterpartyAnswerHandlers() []counterpartyAnswerHandler {
	return []counterpartyAnswerHandler{
		func(e *Engine, ctx counterpartyAuditContext) (Result, bool) {
			cfg := getRuleConfig()
			if e != nil {
				cfg = e.currentRuleConfig()
			}
			if !containsAny(ctx.q, cfg.CounterpartyClassificationQuestionKeywords()) {
				return Result{}, false
			}
			return tryCounterpartyClassificationAnswer(ctx)
		},
		func(e *Engine, ctx counterpartyAuditContext) (Result, bool) {
			if e == nil {
				return Result{}, false
			}
			return e.tryCounterpartyReceiptsAnswer(ctx)
		},
		func(_ *Engine, ctx counterpartyAuditContext) (Result, bool) {
			return tryCounterpartyRevenueAnswer(ctx)
		},
		func(_ *Engine, ctx counterpartyAuditContext) (Result, bool) {
			return tryCounterpartyEmployeeExpenseAnswer(ctx)
		},
		func(_ *Engine, ctx counterpartyAuditContext) (Result, bool) {
			return tryCounterpartyCostAnswer(ctx)
		},
	}
}

func executeCounterpartyAnswerPipeline(engine *Engine, ctx counterpartyAuditContext, handlers []counterpartyAnswerHandler, fallback func(counterpartyAuditContext) Result) Result {
	for _, handler := range handlers {
		if result, ok := handler(engine, ctx); ok {
			return result
		}
	}
	return fallback(ctx)
}
