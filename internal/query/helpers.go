package query

import "strings"

// Intent 枚举定义所有审计行为意图
type Intent string

const (
	IntentGeneral               Intent = "general"
	IntentAmount                Intent = "amount"
	IntentIdentityQuery         Intent = "identity"
	IntentMonthlySummary        Intent = "monthly_summary"
	IntentHostPayload           Intent = "host_payload"
	IntentPrecise               Intent = "precise"
	IntentTaxQuery              Intent = "tax"
	IntentARAPQuery             Intent = "arap"
	IntentLargeTransactionQuery Intent = "large_transaction"
	IntentAnalysis              Intent = "analysis"
	IntentFallback              Intent = "fallback"
)

// ClassifyIntent 精准意图识别引擎 V6 (加权版)
func ClassifyIntent(question string) Intent {
	q := strings.ReplaceAll(question, " ", "")
	cfg := getRuleConfig()

	if isLargeTransactionIntentQuestion(q, cfg) {
		return IntentLargeTransactionQuery
	}

	if containsAny(q, cfg.IntentKeywords(IntentIdentityQuery)) {
		return IntentIdentityQuery
	}

	// 这些问题虽然可能包含“应付”，但业务语义是人力成本，不应被 AR/AP 分流截走。
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) {
		return IntentFallback
	}

	if containsAny(q, cfg.intentKeywordGroup(string(IntentARAPQuery))) {
		return IntentARAPQuery
	}

	if containsAny(q, cfg.intentKeywordGroup(string(IntentTaxQuery))) {
		return IntentTaxQuery
	}

	// 这类问法需要 fallback 结构化提示，而不是分析模块直接接管
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHealth)) {
		return IntentFallback
	}

	if containsAny(q, cfg.intentKeywordGroup(string(IntentAnalysis))) {
		return IntentAnalysis
	}

	if containsAny(q, cfg.intentKeywordGroup(string(IntentFallback))) {
		return IntentFallback
	}

	if containsAny(q, cfg.intentKeywordGroup(string(IntentHostPayload))) {
		return IntentHostPayload
	}

	if strings.Contains(q, "项目") && containsAny(q, []string{"收入", "成本", "支出", "应收", "应付", "数据出来"}) {
		return IntentFallback
	}

	if containsAny(q, cfg.intentKeywordGroup(string(IntentMonthlySummary))) {
		return IntentMonthlySummary
	}

	if containsAny(q, cfg.IntentKeywords(IntentPrecise)) {
		return IntentPrecise
	}

	return IntentGeneral
}

func NormalizeQuestion(q string) string {
	q = strings.ReplaceAll(q, "？", "?")
	q = strings.ReplaceAll(q, "，", ",")
	// 激进清理：移除所有空格，确保实体提取器不会因为空格干扰而失效
	q = strings.ReplaceAll(q, " ", "")
	return strings.TrimSpace(q)
}

func containsAny(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
