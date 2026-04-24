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
