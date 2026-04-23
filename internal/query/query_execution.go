package query

import "strings"

func (e *Engine) executeQuery(ctx queryExecutionContext) Result {
	var result Result

	if ctx.spec.QueryFamily == QueryFamilyHRCost || shouldUseHRBreakdown(ctx.q, ctx.cfg) {
		result = e.queryHRBreakdown(ctx.from, ctx.to)
		if result.Success {
			return ctx.finalize(result)
		}
	}

	if result, ok := e.tryOrchestratedQuery(ctx.spec); ok {
		return ctx.finalize(result)
	}

	if ctx.spec.NeedsContractDimension || shouldUseContractDimension(ctx.q) {
		result = e.queryContractDimension(ctx.q, ctx.entity, ctx.anchor)
		if result.Success {
			return ctx.finalize(result)
		}
	}
	if isIntervalCoreMetricQuestion(ctx.q, ctx.entity, ctx.hasRealEntity, ctx.from, ctx.to) || shouldPreferCoreMetricSummary(ctx.q, ctx.entity, ctx.hasRealEntity, ctx.from, ctx.to) {
		result = e.queryDualPerspectiveForCoreMetric(ctx.q, ctx.from, ctx.to)
		if result.Success {
			return ctx.finalize(result)
		}
	}
	if shouldUseSupplierPaymentStats(ctx.q) {
		result = e.querySupplierPayments(ctx.from, ctx.to)
		if result.Success {
			return ctx.finalize(result)
		}
	}
	if ctx.hasRealEntity && isCounterpartyClassificationQuestion(ctx.q) {
		result = e.queryCounterpartyAmountFallback(ctx.q, ctx.entity, ctx.from, ctx.to)
		if result.Success {
			return ctx.finalize(result)
		}
	}
	if shouldUseReconciliation(ctx.q) {
		result = e.queryReconciliation(ctx.q, ctx.from, ctx.to)
		if result.Success {
			return ctx.finalize(result)
		}
	}
	if e.shouldUseCounterpartyAuditFallback(ctx) {
		result = e.queryCounterpartyAmountFallback(ctx.q, ctx.entity, ctx.from, ctx.to)
		if result.Success {
			return ctx.finalize(result)
		}
	}

	result = e.executeIntentRoute(ctx)
	if result.Success {
		return ctx.finalize(result)
	}

	if ctx.entity != "" && result.Message == "account not found" {
		return ctx.finalize(e.queryFallback(ctx.q, ctx.from, ctx.to, result.Message))
	}
	if result.Message == "account not found" || strings.Contains(result.Message, "语义模糊") {
		return ctx.finalize(e.queryFallback(ctx.q, ctx.from, ctx.to, result.Message))
	}
	if result.Message != "" {
		return ctx.finalize(result)
	}
	return ctx.finalize(e.queryFallback(ctx.q, ctx.from, ctx.to, result.Message))
}

func (e *Engine) executeIntentRoute(ctx queryExecutionContext) Result {
	switch ctx.intent {
	case IntentHostPayload:
		return e.queryHostLLMPayload(ctx.q, ctx.from, ctx.to)
	case IntentIdentityQuery:
		return e.queryIdentityResult(ctx.entity)
	case IntentARAPQuery:
		return e.queryARAP(ctx.q, ctx.entity, ctx.from, ctx.to)
	case IntentLargeTransactionQuery:
		return e.queryLargeBankTransactions(ctx.q, ctx.from, ctx.to)
	case IntentTaxQuery:
		return e.queryTax(ctx.q, ctx.from, ctx.to)
	case IntentMonthlySummary:
		return e.queryMonthlySummary(ctx.q, ctx.from, ctx.to)
	case IntentAnalysis:
		return e.queryAnalysis(ctx.to)
	case IntentFallback:
		return e.queryFallback(ctx.q, ctx.from, ctx.to, "")
	default:
		return e.queryPrecise(ctx.q, ctx.to)
	}
}

func (e *Engine) shouldUseCounterpartyAuditFallback(ctx queryExecutionContext) bool {
	if !ctx.hasRealEntity {
		return false
	}
	return containsAny(ctx.q, append(metricQuestionKeywords(ctx.cfg), "回款", "到账", "收款", "费用", "支出", "付款", "付了", "支付"))
}
