package query

import "strings"

func shouldPreferContractAggregate(q string, intent Intent, family QueryFamily, metricKind MetricKind, cfg RuleConfig) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	if family != QueryFamilyCoreMetric {
		return false
	}
	if shouldUseContractFirstARAP(q) {
		return true
	}
	if intent == IntentARAPQuery || intent == IntentTaxQuery || intent == IntentAnalysis || intent == IntentHostPayload {
		return false
	}
	if shouldUseExplicitFinancialAccountQuestion(q) {
		return false
	}
	if metricKind == MetricKindUnknown || metricKind == MetricKindReceipts {
		return false
	}
	if asksExplicitNetProfit(q) {
		return false
	}
	if containsAny(q, cfg.ContractCashFallbackKeywords()) {
		return false
	}
	if shouldUseSupplierPaymentStats(q) || shouldUseHRBreakdown(q, cfg) || shouldUseReconciliation(q) {
		return false
	}
	return containsAny(q, cfg.ContractSummaryKeywords())
}
