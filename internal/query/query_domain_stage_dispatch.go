package query

func (e *Engine) executeDomainStage(stage executionStage, ctx queryExecutionContext) (Result, bool) {
	if handler, ok := executionDomainStageHandlers()[stage]; ok {
		return handler(e, ctx)
	}
	return Result{}, false
}
