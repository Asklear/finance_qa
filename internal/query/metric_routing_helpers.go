package query

import "strings"

func shouldForceDualPerspective(q string) bool {
	return shouldForceDualPerspectiveWithConfig(q, getRuleConfig())
}

func shouldForceDualPerspectiveWithConfig(q string, cfg RuleConfig) bool {
	blockers := append([]string{"项目成本"}, cfg.intentKeywordGroup(routerGroupHRCost)...)
	blockers = append(blockers, cfg.intentKeywordGroup(string(IntentARAPQuery))...)
	blockers = append(blockers, cfg.intentKeywordGroup(string(IntentTaxQuery))...)
	if containsAny(q, blockers) {
		return false
	}
	return containsAny(q, metricQuestionKeywords(cfg))
}

func shouldPreferCoreMetricSummary(q, entity string, hasRealEntity bool, from, to string) bool {
	return shouldPreferCoreMetricSummaryWithConfig(q, entity, hasRealEntity, from, to, getRuleConfig())
}

func shouldPreferCoreMetricSummaryWithConfig(q, entity string, hasRealEntity bool, from, to string, cfg RuleConfig) bool {
	if shouldUseReconciliation(q) {
		return false
	}
	if !shouldForceDualPerspectiveWithConfig(q, cfg) {
		return false
	}
	if !hasRealEntity {
		return true
	}
	if from == to {
		return false
	}
	if isGenericMetricEntityWithConfig(entity, cfg) || looksLikeTemporalMetricEntity(entity) {
		return true
	}
	return false
}

func isIntervalCoreMetricQuestion(q, entity string, hasRealEntity bool, from, to string) bool {
	return isIntervalCoreMetricQuestionWithConfig(q, entity, hasRealEntity, from, to, getRuleConfig())
}

func isIntervalCoreMetricQuestionWithConfig(q, entity string, hasRealEntity bool, from, to string, cfg RuleConfig) bool {
	if shouldUseReconciliation(q) {
		return false
	}
	if from == to {
		return false
	}
	if hasRealEntity {
		return false
	}
	if strings.TrimSpace(entity) != "" && !isGenericMetricEntityWithConfig(entity, cfg) && !looksLikeTemporalMetricEntity(entity) {
		return false
	}
	if !containsAny(q, metricQuestionKeywords(cfg)) {
		return false
	}
	return containsAny(q, []string{"季度", "q1", "q2", "q3", "q4", "Q1", "Q2", "Q3", "Q4", "上半年", "下半年", "全年", "全年度", "整年", "年度", "累计", "年内"})
}

func shouldUseSingleAccrualCoreMetrics(q string) bool {
	return shouldUseSingleAccrualCoreMetricsWithConfig(q, getRuleConfig())
}

func shouldUseSingleAccrualCoreMetricsWithConfig(q string, cfg RuleConfig) bool {
	if shouldUseReconciliation(q) || containsAny(q, cfg.ProfitSingleViewBlockKeywords()) {
		return false
	}
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) {
		return false
	}
	return containsAny(q, metricQuestionKeywords(cfg))
}

func shouldUseSupplierPaymentStats(q string) bool {
	if !strings.Contains(q, "供应商") {
		return false
	}
	return containsAny(q, []string{
		"多少", "有哪些", "哪些", "哪几", "几个", "几家", "名单", "列表", "明细", "分别", "叫什么",
		"发生付款", "付款", "支付", "支出",
	})
}

func isCounterpartyClassificationQuestion(q string) bool {
	return isCounterpartyClassificationQuestionWithConfig(q, getRuleConfig())
}

func isCounterpartyClassificationQuestionWithConfig(q string, cfg RuleConfig) bool {
	return containsAny(q, cfg.CounterpartyClassificationQuestionKeywords())
}

func shouldBypassDualPerspective(q, entity string) bool {
	return strings.TrimSpace(entity) != "" && !isGenericMetricEntity(entity)
}

func shouldUseHRBreakdown(q string, cfg RuleConfig) bool {
	asksBreakdown := containsAny(q, cfg.HRBreakdownKeywords())
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) && (asksBreakdown || containsAny(q, []string{"多少", "是多少", "合计", "金额", "成本", "费用", "余额"})) {
		return true
	}
	return containsAny(q, []string{"工资", "社保", "公积金"}) && containsAny(q, []string{"多少", "明细", "分别", "合计", "成本"})
}

func metricQuestionKeywords(cfg RuleConfig) []string {
	keywords := make([]string, 0, 8)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyRevenue)...)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyCost)...)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyProfit)...)
	return dedupeStrings(keywords)
}

func counterpartyMetricKeywords(cfg RuleConfig) []string {
	keywords := append([]string{}, metricQuestionKeywords(cfg)...)
	keywords = append(keywords, "回款", "到账", "收款", "费用", "支出", "付款", "付了", "支付", "应收", "应付", "应收账款", "应付账款", "已开票未回款", "已收票未付款")
	return dedupeStrings(keywords)
}
