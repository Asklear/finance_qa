package query

import (
	"strings"

	"financeqa/internal/accounting"
)

func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		base = append(base, v)
		seen[v] = true
	}
	return base
}

func shouldForceDualPerspective(q string) bool {
	cfg := getRuleConfig()
	blockers := append([]string{"项目成本"}, cfg.intentKeywordGroup(routerGroupHRCost)...)
	blockers = append(blockers, cfg.intentKeywordGroup(string(IntentARAPQuery))...)
	blockers = append(blockers, cfg.intentKeywordGroup(string(IntentTaxQuery))...)
	if containsAny(q, blockers) {
		return false
	}
	return containsAny(q, metricQuestionKeywords(cfg))
}

func shouldPreferCoreMetricSummary(q, entity string, hasRealEntity bool, from, to string) bool {
	if !shouldForceDualPerspective(q) {
		return false
	}
	if !hasRealEntity {
		return true
	}
	if from == to {
		return false
	}
	if isGenericMetricEntity(entity) || looksLikeTemporalMetricEntity(entity) {
		return true
	}
	return false
}

func isIntervalCoreMetricQuestion(q, entity string, hasRealEntity bool, from, to string) bool {
	if from == to {
		return false
	}
	if hasRealEntity {
		return false
	}
	if strings.TrimSpace(entity) != "" && !isGenericMetricEntity(entity) && !looksLikeTemporalMetricEntity(entity) {
		return false
	}
	if !containsAny(q, metricQuestionKeywords(getRuleConfig())) {
		return false
	}
	return containsAny(q, []string{"季度", "q1", "q2", "q3", "q4", "Q1", "Q2", "Q3", "Q4", "上半年", "下半年", "全年", "全年度", "整年", "年度", "累计", "年内"})
}

func shouldUseSingleAccrualCoreMetrics(q string) bool {
	cfg := getRuleConfig()
	if shouldUseReconciliation(q) || containsAny(q, cfg.ProfitSingleViewBlockKeywords()) {
		return false
	}
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) {
		return false
	}
	return containsAny(q, metricQuestionKeywords(cfg))
}

func shouldUseSupplierPaymentStats(q string) bool {
	return strings.Contains(q, "供应商") && strings.Contains(q, "多少")
}

func isCounterpartyClassificationQuestion(q string) bool {
	return containsAny(q, getRuleConfig().CounterpartyClassificationQuestionKeywords())
}

func shouldBypassDualPerspective(q, entity string) bool {
	return strings.TrimSpace(entity) != "" && !isGenericMetricEntity(entity)
}

func shouldUseHRBreakdown(q string, cfg RuleConfig) bool {
	asksBreakdown := containsAny(q, cfg.HRBreakdownKeywords())
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) && asksBreakdown {
		return true
	}
	return containsAny(q, []string{"工资", "社保", "公积金"}) && containsAny(q, []string{"多少", "明细", "分别", "合计", "成本"})
}

func isGenericMetricEntity(entity string) bool {
	key := normalizeEntityText(entity)
	if key == "" {
		return true
	}
	cfg := getRuleConfig()
	for _, s := range cfg.GenericMetricStopwords {
		if normalizeEntityText(s) == key {
			return true
		}
	}
	return false
}

func detectRequestedMetrics(q string) []string {
	metrics := make([]string, 0, 3)
	cfg := getRuleConfig()
	if containsAny(q, cfg.MetricKeywords(metricKeyRevenue)) {
		metrics = append(metrics, "收入")
	}
	if containsAny(q, cfg.MetricKeywords(metricKeyCost)) {
		metrics = append(metrics, "成本")
	}
	if containsAny(q, cfg.MetricKeywords(metricKeyProfit)) {
		metrics = append(metrics, "利润")
	}
	if len(metrics) == 0 {
		metrics = append(metrics, detectCoreMetric(q))
	}
	return metrics
}

func detectCoreMetric(q string) string {
	switch {
	case containsAny(q, getRuleConfig().MetricKeywords(metricKeyProfit)):
		return "利润"
	case containsAny(q, getRuleConfig().MetricKeywords(metricKeyCost)):
		return "成本"
	default:
		return "收入"
	}
}

func metricQuestionKeywords(cfg RuleConfig) []string {
	keywords := make([]string, 0, 8)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyRevenue)...)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyCost)...)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyProfit)...)
	return dedupeStrings(keywords)
}

func pickMetricValue(metric string, dual *accounting.DualPerspective) (float64, float64) {
	switch metric {
	case "利润":
		return dual.Cash.Net, dual.Accrual.Profit
	case "成本":
		return dual.Cash.Expense, dual.Accrual.TotalCost
	case "销售额", "收入":
		return dual.Cash.Income, dual.Accrual.Revenue
	default:
		return dual.Cash.Income, dual.Accrual.Revenue
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == strings.TrimSpace(want) {
			return true
		}
	}
	return false
}

func firstMetricOrDefault(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	if strings.TrimSpace(items[0]) == "" {
		return fallback
	}
	return items[0]
}
