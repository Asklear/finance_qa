package query

func (e *Engine) queryCounterpartyAmountFallback(question, entity, from, to string) Result {
	if entity == "" {
		return Result{Success: false, Message: "no named counterparty found"}
	}
	snap := e.buildCounterpartySnapshot(entity, from, to)
	evidence := e.collectCounterpartyEvidence(entity, from, to)
	cfg := e.currentRuleConfig()
	classification := ClassifyCounterpartyWithConfig(entity, evidence, cfg)
	taxReport := NormalizeTaxWithConfig(entity, evidence, cfg)
	usedRetro := false
	if snap.BankIn == 0 && snap.BankOut == 0 && snap.RevenueNet == 0 && snap.BookCost == 0 && snap.BookExpense == 0 {
		retroFrom := from[:4] + "-01"
		snap = e.buildCounterpartySnapshot(entity, retroFrom, to)
		evidence = e.collectCounterpartyEvidence(entity, retroFrom, to)
		classification = ClassifyCounterpartyWithConfig(entity, evidence, cfg)
		taxReport = NormalizeTaxWithConfig(entity, evidence, cfg)
		usedRetro = true
	}

	ctx := buildCounterpartyAuditContext(question, entity, from, to, snap, classification, taxReport, evidence, usedRetro)
	return executeCounterpartyAnswerPipeline(e, ctx, defaultCounterpartyAnswerHandlers(), buildCounterpartyFallbackAnswer)
}
