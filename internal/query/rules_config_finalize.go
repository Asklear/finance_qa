package query

import "strings"

func (cfg *RuleConfig) finalize() {
	if cfg.ReconciliationResidualGapEscalationAmount < 0 {
		cfg.ReconciliationResidualGapEscalationAmount = 0
	}
	cfg.GenericMetricStopwords = dedupeNonEmpty(cfg.GenericMetricStopwords)
	cfg.FallbackMonthlyExpenseKeywords = dedupeNonEmpty(cfg.FallbackMonthlyExpenseKeywords)
	cfg.HighPriorityPhrases = normalizeStringSliceMap(cfg.HighPriorityPhrases)
	cfg.IntentPriority = normalizeIntMap(cfg.IntentPriority)
	cfg.IntentConflicts = normalizeStringSliceMap(cfg.IntentConflicts)
	cfg.IntentMinConfidence = normalizeFloatMap(cfg.IntentMinConfidence)

	cfg.IntentKeywordLexicon = normalizeStringSliceMap(cfg.IntentKeywordLexicon)
	syncKnownIntentKeywordGroup(cfg, string(IntentARAPQuery), &cfg.IntentARAPKeywords)
	syncKnownIntentKeywordGroup(cfg, routerGroupHRCost, &cfg.IntentHRCostKeywords)
	syncKnownIntentKeywordGroup(cfg, string(IntentTaxQuery), &cfg.IntentTaxKeywords)
	syncKnownIntentKeywordGroup(cfg, routerGroupHealth, &cfg.IntentHealthKeywords)
	syncKnownIntentKeywordGroup(cfg, string(IntentFallback), &cfg.IntentFallbackKeywords)
	syncKnownIntentKeywordGroup(cfg, string(IntentAnalysis), &cfg.IntentAnalysisKeywords)
	syncKnownIntentKeywordGroup(cfg, string(IntentHostPayload), &cfg.IntentHostPayloadKeywords)
	syncKnownIntentKeywordGroup(cfg, string(IntentMonthlySummary), &cfg.IntentMonthlySummaryKeywords)

	cfg.MetricKeywordLexicon = normalizeStringSliceMap(cfg.MetricKeywordLexicon)
	cfg.HRBreakdownKeywordLexicon = dedupeNonEmpty(cfg.HRBreakdownKeywordLexicon)
	cfg.SupplierPaymentExcludeNameLexicon = dedupeNonEmpty(cfg.SupplierPaymentExcludeNameLexicon)
	cfg.CounterpartyClassificationQuestionLexicon = dedupeNonEmpty(cfg.CounterpartyClassificationQuestionLexicon)
	cfg.ProfitSingleViewBlockKeywordLexicon = dedupeNonEmpty(cfg.ProfitSingleViewBlockKeywordLexicon)
	cfg.ExpenseBreakdownTriggerKeywordLexicon = dedupeNonEmpty(cfg.ExpenseBreakdownTriggerKeywordLexicon)
	cfg.ExpenseBreakdownExpenseKeywordLexicon = dedupeNonEmpty(cfg.ExpenseBreakdownExpenseKeywordLexicon)
	cfg.ExpenseBreakdownMetricBlockKeywordLexicon = dedupeNonEmpty(cfg.ExpenseBreakdownMetricBlockKeywordLexicon)
	cfg.ExpenseBreakdownMetricAllowKeywordLexicon = dedupeNonEmpty(cfg.ExpenseBreakdownMetricAllowKeywordLexicon)
	cfg.ExpenseBreakdownCostKeywordLexicon = dedupeNonEmpty(cfg.ExpenseBreakdownCostKeywordLexicon)
	cfg.ExpenseBreakdownMetricLabel = strings.TrimSpace(cfg.ExpenseBreakdownMetricLabel)
	cfg.ExpenseBreakdownViewLexicon = normalizeExpenseBreakdownViews(cfg.ExpenseBreakdownViewLexicon)
	cfg.ExpenseBreakdownCashCategoryLexicon = normalizeExpenseBreakdownCategoryRules(cfg.ExpenseBreakdownCashCategoryLexicon)
	cfg.ExpenseBreakdownCashDefaultCategory = strings.TrimSpace(cfg.ExpenseBreakdownCashDefaultCategory)
	cfg.ExpenseBreakdownAccountCategoryLexicon = normalizeExpenseBreakdownCategoryRules(cfg.ExpenseBreakdownAccountCategoryLexicon)
	cfg.ExpenseBreakdownAccountDefaultCategory = strings.TrimSpace(cfg.ExpenseBreakdownAccountDefaultCategory)
	cfg.ContractPriorityKeywordLexicon = dedupeNonEmpty(cfg.ContractPriorityKeywordLexicon)
	cfg.ContractSourceTableLexicon = normalizeStringSliceMap(cfg.ContractSourceTableLexicon)
	cfg.ContractSummaryKeywordLexicon = dedupeNonEmpty(cfg.ContractSummaryKeywordLexicon)
	cfg.ContractCashFallbackLexicon = dedupeNonEmpty(cfg.ContractCashFallbackLexicon)
	cfg.IncomeStatementItemLexicon = normalizeStringSliceMap(cfg.IncomeStatementItemLexicon)
	cfg.HRBreakdownAccountCodeLexicon = normalizeStringSliceMap(cfg.HRBreakdownAccountCodeLexicon)
	cfg.HRCashBankAccountPrefixLexicon = dedupeNonEmpty(cfg.HRCashBankAccountPrefixLexicon)
	cfg.HRPayrollLiabilityPrefixLexicon = dedupeNonEmpty(cfg.HRPayrollLiabilityPrefixLexicon)
	cfg.HRCategoryKeywordLexicon = normalizeStringSliceMap(cfg.HRCategoryKeywordLexicon)
	cfg.CounterpartyRoleLexicon = normalizeStringSliceMap(cfg.CounterpartyRoleLexicon)
	cfg.CounterpartyTaxLexicon = normalizeStringSliceMap(cfg.CounterpartyTaxLexicon)
	cfg.InternalPartyOrgSuffixLexicon = dedupeNonEmpty(cfg.InternalPartyOrgSuffixLexicon)
	cfg.InternalPartyAccountContextKeywordLexicon = dedupeNonEmpty(cfg.InternalPartyAccountContextKeywordLexicon)
}

func setIntentKeywordGroup(cfg *RuleConfig, group string, values []string) {
	normalized := dedupeNonEmpty(values)
	if len(normalized) == 0 {
		return
	}
	cfg.IntentKeywordLexicon = ensureStringSliceMap(cfg.IntentKeywordLexicon)
	cfg.IntentKeywordLexicon[strings.TrimSpace(group)] = normalized
	switch strings.TrimSpace(group) {
	case string(IntentARAPQuery):
		cfg.IntentARAPKeywords = normalized
	case routerGroupHRCost:
		cfg.IntentHRCostKeywords = normalized
	case string(IntentTaxQuery):
		cfg.IntentTaxKeywords = normalized
	case routerGroupHealth:
		cfg.IntentHealthKeywords = normalized
	case string(IntentFallback):
		cfg.IntentFallbackKeywords = normalized
	case string(IntentAnalysis):
		cfg.IntentAnalysisKeywords = normalized
	case string(IntentHostPayload):
		cfg.IntentHostPayloadKeywords = normalized
	case string(IntentMonthlySummary):
		cfg.IntentMonthlySummaryKeywords = normalized
	}
}

func mergeExpenseBreakdownViews(base, overrides map[string]ExpenseBreakdownViewRule) map[string]ExpenseBreakdownViewRule {
	out := make(map[string]ExpenseBreakdownViewRule, len(base)+len(overrides))
	for key, view := range normalizeExpenseBreakdownViews(base) {
		out[key] = view
	}
	for key, override := range normalizeExpenseBreakdownViews(overrides) {
		existing := out[key]
		if override.Label != "" {
			existing.Label = override.Label
		}
		if override.Description != "" {
			existing.Description = override.Description
		}
		if override.SummaryLimit > 0 {
			existing.SummaryLimit = override.SummaryLimit
		}
		out[key] = existing
	}
	return out
}

func normalizeExpenseBreakdownViews(input map[string]ExpenseBreakdownViewRule) map[string]ExpenseBreakdownViewRule {
	out := make(map[string]ExpenseBreakdownViewRule, len(input))
	for key, view := range input {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		view.Label = strings.TrimSpace(view.Label)
		view.Description = strings.TrimSpace(view.Description)
		if view.Label == "" && view.Description == "" && view.SummaryLimit <= 0 {
			continue
		}
		out[trimmedKey] = view
	}
	return out
}

func normalizeExpenseBreakdownCategoryRules(input []ExpenseBreakdownCategoryRule) []ExpenseBreakdownCategoryRule {
	out := make([]ExpenseBreakdownCategoryRule, 0, len(input))
	for _, rule := range input {
		rule.Category = strings.TrimSpace(rule.Category)
		if rule.Category == "" {
			continue
		}
		rule.Keywords = dedupeNonEmpty(rule.Keywords)
		rule.CounterpartyRole = strings.TrimSpace(rule.CounterpartyRole)
		rule.AccountCodePrefixes = dedupeNonEmpty(rule.AccountCodePrefixes)
		out = append(out, rule)
	}
	return out
}

func syncKnownIntentKeywordGroup(cfg *RuleConfig, group string, target *[]string) {
	if values, ok := cfg.IntentKeywordLexicon[group]; ok && len(values) > 0 {
		normalized := dedupeNonEmpty(values)
		*target = normalized
		cfg.IntentKeywordLexicon[group] = normalized
		return
	}
	normalized := dedupeNonEmpty(*target)
	*target = normalized
	if len(normalized) == 0 {
		return
	}
	cfg.IntentKeywordLexicon = ensureStringSliceMap(cfg.IntentKeywordLexicon)
	cfg.IntentKeywordLexicon[group] = normalized
}
