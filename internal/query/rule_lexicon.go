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

func (cfg RuleConfig) SupplierPaymentExcludeNames() []string {
	return copyStringSlice(cfg.SupplierPaymentExcludeNameLexicon)
}

func (cfg RuleConfig) CounterpartyClassificationQuestionKeywords() []string {
	return copyStringSlice(cfg.CounterpartyClassificationQuestionLexicon)
}

func (cfg RuleConfig) ProfitSingleViewBlockKeywords() []string {
	return copyStringSlice(cfg.ProfitSingleViewBlockKeywordLexicon)
}

func (cfg RuleConfig) ExpenseBreakdownTriggerKeywords() []string {
	return copyStringSlice(cfg.ExpenseBreakdownTriggerKeywordLexicon)
}

func (cfg RuleConfig) ExpenseBreakdownExpenseKeywords() []string {
	return copyStringSlice(cfg.ExpenseBreakdownExpenseKeywordLexicon)
}

func (cfg RuleConfig) ExpenseBreakdownMetricBlockKeywords() []string {
	return copyStringSlice(cfg.ExpenseBreakdownMetricBlockKeywordLexicon)
}

func (cfg RuleConfig) ExpenseBreakdownMetricAllowKeywords() []string {
	return copyStringSlice(cfg.ExpenseBreakdownMetricAllowKeywordLexicon)
}

func (cfg RuleConfig) ExpenseBreakdownCostKeywords() []string {
	return copyStringSlice(cfg.ExpenseBreakdownCostKeywordLexicon)
}

func (cfg RuleConfig) ExpenseBreakdownMetricName() string {
	return strings.TrimSpace(cfg.ExpenseBreakdownMetricLabel)
}

func (cfg RuleConfig) ExpenseBreakdownView(key string) ExpenseBreakdownViewRule {
	view := cfg.ExpenseBreakdownViewLexicon[strings.TrimSpace(key)]
	return view
}

func (cfg RuleConfig) ExpenseBreakdownCashCategoryRules() []ExpenseBreakdownCategoryRule {
	return copyExpenseBreakdownCategoryRules(cfg.ExpenseBreakdownCashCategoryLexicon)
}

func (cfg RuleConfig) ExpenseBreakdownCashDefaultCategoryName() string {
	return strings.TrimSpace(cfg.ExpenseBreakdownCashDefaultCategory)
}

func (cfg RuleConfig) ExpenseBreakdownAccountCategoryRules() []ExpenseBreakdownCategoryRule {
	return copyExpenseBreakdownCategoryRules(cfg.ExpenseBreakdownAccountCategoryLexicon)
}

func (cfg RuleConfig) ExpenseBreakdownAccountDefaultCategoryName() string {
	return strings.TrimSpace(cfg.ExpenseBreakdownAccountDefaultCategory)
}

func (cfg RuleConfig) ContractPriorityKeywords() []string {
	return copyStringSlice(cfg.ContractPriorityKeywordLexicon)
}

func (cfg RuleConfig) ContractSummaryKeywords() []string {
	return copyStringSlice(cfg.ContractSummaryKeywordLexicon)
}

func (cfg RuleConfig) ContractCashFallbackKeywords() []string {
	return copyStringSlice(cfg.ContractCashFallbackLexicon)
}

func (cfg RuleConfig) ContractSourceTables(role string) []string {
	tables := cfg.ContractSourceTableLexicon[strings.TrimSpace(role)]
	if len(tables) == 0 {
		tables = cfg.ContractSourceTableLexicon[contractRoleDefault]
	}
	return copyStringSlice(tables)
}

func (cfg RuleConfig) IncomeStatementPatterns(key string) []string {
	return copyStringSlice(cfg.IncomeStatementItemLexicon[strings.TrimSpace(key)])
}

func (cfg RuleConfig) HRBreakdownAccountCodes(category string) []string {
	return copyStringSlice(cfg.HRBreakdownAccountCodeLexicon[strings.TrimSpace(category)])
}

func (cfg RuleConfig) HRCashBankAccountPrefixes() []string {
	return copyStringSlice(cfg.HRCashBankAccountPrefixLexicon)
}

func (cfg RuleConfig) HRPayrollLiabilityPrefixes() []string {
	return copyStringSlice(cfg.HRPayrollLiabilityPrefixLexicon)
}

func (cfg RuleConfig) HRPayrollLiabilityNameKeywords() []string {
	return copyStringSlice(cfg.HRPayrollLiabilityNameLexicon)
}

func (cfg RuleConfig) HRCategoryKeywords(category string) []string {
	return copyStringSlice(cfg.HRCategoryKeywordLexicon[strings.TrimSpace(category)])
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

func (cfg RuleConfig) ReconciliationResidualGapEscalationThreshold() float64 {
	if cfg.ReconciliationResidualGapEscalationAmount < 0 {
		return 0
	}
	return cfg.ReconciliationResidualGapEscalationAmount
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

func copyExpenseBreakdownCategoryRules(items []ExpenseBreakdownCategoryRule) []ExpenseBreakdownCategoryRule {
	if len(items) == 0 {
		return nil
	}
	out := make([]ExpenseBreakdownCategoryRule, len(items))
	for i, item := range items {
		out[i] = item
		out[i].Keywords = copyStringSlice(item.Keywords)
		out[i].AccountCodePrefixes = copyStringSlice(item.AccountCodePrefixes)
	}
	return out
}
