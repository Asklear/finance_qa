package query

import "strings"

func (cfg RuleConfig) IntentKeywords(intent Intent) []string {
	return copyStringSlice(cfg.intentKeywordGroup(string(intent)))
}

func (cfg RuleConfig) MetricKeywords(metric string) []string {
	return copyStringSlice(cfg.metricKeywordGroup(metric))
}

func (cfg RuleConfig) HRBreakdownKeywords() []string {
	return copyStringSlice(cfg.HRBreakdownKeywordLexicon)
}

func (cfg RuleConfig) CounterpartyClassificationQuestionKeywords() []string {
	return copyStringSlice(cfg.CounterpartyClassificationQuestionLexicon)
}

func (cfg RuleConfig) ProfitSingleViewBlockKeywords() []string {
	return copyStringSlice(cfg.ProfitSingleViewBlockKeywordLexicon)
}

func (cfg RuleConfig) CounterpartyRoleKeywords(role CounterpartyRole) []string {
	return copyStringSlice(cfg.CounterpartyRoleLexicon[string(role)])
}

func (cfg RuleConfig) TaxKeywords(side TaxSide) []string {
	return copyStringSlice(cfg.CounterpartyTaxLexicon[string(side)])
}

func (cfg RuleConfig) InternalPartyOrgSuffixes() []string {
	return copyStringSlice(cfg.InternalPartyOrgSuffixLexicon)
}

func (cfg RuleConfig) InternalPartyAccountContextKeywords() []string {
	return copyStringSlice(cfg.InternalPartyAccountContextKeywordLexicon)
}

func (cfg RuleConfig) intentKeywordGroup(group string) []string {
	if cfg.IntentKeywordLexicon == nil {
		return nil
	}
	return cfg.IntentKeywordLexicon[strings.TrimSpace(group)]
}

func (cfg RuleConfig) metricKeywordGroup(metric string) []string {
	if cfg.MetricKeywordLexicon == nil {
		return nil
	}
	return cfg.MetricKeywordLexicon[canonicalMetricKey(metric)]
}

func canonicalMetricKey(metric string) string {
	normalized := normalizeEntityText(metric)
	switch normalized {
	case normalizeEntityText("利润"), normalizeEntityText("净利"), normalizeEntityText("profit"):
		return metricKeyProfit
	case normalizeEntityText("成本"), normalizeEntityText("总成本"), normalizeEntityText("cost"):
		return metricKeyCost
	default:
		return metricKeyRevenue
	}
}

func metricDisplayName(metric string) string {
	switch canonicalMetricKey(metric) {
	case metricKeyProfit:
		return "利润"
	case metricKeyCost:
		return "成本"
	default:
		return "收入"
	}
}

func copyStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}
