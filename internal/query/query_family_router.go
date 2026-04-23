package query

import "strings"

func detectQueryFamily(q string, intent Intent, entity, from, to string, cfg RuleConfig, needsContractDimension bool) QueryFamily {
	if needsContractDimension {
		return QueryFamilyContractDimension
	}
	if strings.Contains(q, "数据出来") {
		return QueryFamilyReadiness
	}
	if shouldUseSupplierPaymentStats(q) {
		return QueryFamilySupplierPayments
	}
	if shouldUseHRBreakdown(q, cfg) {
		return QueryFamilyHRCost
	}
	if intent == IntentARAPQuery || isOpeningPeriodQuestion(q) {
		return QueryFamilyARAP
	}
	if shouldUseReconciliation(q) {
		return QueryFamilyReconciliation
	}
	hasRealishEntity := isRealishQueryEntity(entity)
	if isBossAggregateSummaryQuestion(q, entity, from, to, cfg) {
		return QueryFamilyCoreMetric
	}
	if isIntervalCoreMetricQuestion(q, entity, hasRealishEntity, from, to) || shouldPreferCoreMetricSummary(q, entity, hasRealishEntity, from, to) {
		return QueryFamilyCoreMetric
	}
	if hasRealishEntity && containsAny(q, append(metricQuestionKeywords(cfg), "回款", "到账", "收款", "费用", "支出", "付款", "支付")) {
		return QueryFamilyCounterparty
	}
	switch intent {
	case IntentMonthlySummary:
		return QueryFamilyCoreMetric
	case IntentFallback:
		if hasRealishEntity {
			return QueryFamilyCounterparty
		}
	}
	return QueryFamilyGeneral
}

func isBossAggregateSummaryQuestion(q, entity, from, to string, cfg RuleConfig) bool {
	if !containsAny(q, metricQuestionKeywords(cfg)) {
		return false
	}
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return false
	}
	// 公司汇总型提问不能被时间片段/问句碎片误判成实体后抢走路由。
	if isRealishQueryEntity(entity) {
		return false
	}
	return true
}
