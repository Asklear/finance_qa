package query

import (
	"encoding/json"
	"os"
	"strings"
)

func buildRuleConfig(rulesPath string, getenv func(string) string) RuleConfig {
	cfg := defaultRuleConfig()
	mergeRuleConfigFromPath(&cfg, rulesPath)
	mergeRuleConfigFromLookup(&cfg, getenv)
	cfg.finalize()
	return cfg
}

func mergeRuleConfigFromFile(cfg *RuleConfig) {
	mergeRuleConfigFromPath(cfg, os.Getenv("FINANCEQA_RULES_PATH"))
}

func mergeRuleConfigFromPath(cfg *RuleConfig, rulesPath string) {
	raw, ok := readRuleConfigFile(strings.TrimSpace(rulesPath))
	if !ok {
		return
	}
	applyLegacyRuleConfig(cfg, raw)
	if raw.SchemaVersion >= 2 {
		applyNestedRuleConfig(cfg, raw)
	}
}

func readRuleConfigFile(path string) (ruleConfigFile, bool) {
	if strings.TrimSpace(path) == "" {
		return ruleConfigFile{}, false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ruleConfigFile{}, false
	}
	var raw ruleConfigFile
	if err := json.Unmarshal(content, &raw); err != nil {
		return ruleConfigFile{}, false
	}
	return raw, true
}

func applyLegacyRuleConfig(cfg *RuleConfig, raw ruleConfigFile) {
	if len(raw.GenericMetricStopwords) > 0 {
		cfg.GenericMetricStopwords = dedupeNonEmpty(raw.GenericMetricStopwords)
	}
	if len(raw.IntentARAPKeywords) > 0 {
		setIntentKeywordGroup(cfg, string(IntentARAPQuery), raw.IntentARAPKeywords)
	}
	if len(raw.IntentHRCostKeywords) > 0 {
		setIntentKeywordGroup(cfg, routerGroupHRCost, raw.IntentHRCostKeywords)
	}
	if len(raw.IntentTaxKeywords) > 0 {
		setIntentKeywordGroup(cfg, string(IntentTaxQuery), raw.IntentTaxKeywords)
	}
	if len(raw.IntentHealthKeywords) > 0 {
		setIntentKeywordGroup(cfg, routerGroupHealth, raw.IntentHealthKeywords)
	}
	if len(raw.IntentFallbackKeywords) > 0 {
		setIntentKeywordGroup(cfg, string(IntentFallback), raw.IntentFallbackKeywords)
	}
	if len(raw.IntentAnalysisKeywords) > 0 {
		setIntentKeywordGroup(cfg, string(IntentAnalysis), raw.IntentAnalysisKeywords)
	}
	if len(raw.IntentHostPayloadKeywords) > 0 {
		setIntentKeywordGroup(cfg, string(IntentHostPayload), raw.IntentHostPayloadKeywords)
	}
	if len(raw.IntentMonthlySummaryKeywords) > 0 {
		setIntentKeywordGroup(cfg, string(IntentMonthlySummary), raw.IntentMonthlySummaryKeywords)
	}
	if len(raw.FallbackMonthlyExpenseKeywords) > 0 {
		cfg.FallbackMonthlyExpenseKeywords = dedupeNonEmpty(raw.FallbackMonthlyExpenseKeywords)
	}
	if len(raw.HighPriorityPhrases) > 0 {
		cfg.HighPriorityPhrases = normalizeStringSliceMap(raw.HighPriorityPhrases)
	}
	if len(raw.IntentPriority) > 0 {
		cfg.IntentPriority = normalizeIntMap(raw.IntentPriority)
	}
	if len(raw.IntentConflicts) > 0 {
		cfg.IntentConflicts = normalizeStringSliceMap(raw.IntentConflicts)
	}
	if len(raw.IntentMinConfidence) > 0 {
		cfg.IntentMinConfidence = normalizeFloatMap(raw.IntentMinConfidence)
	}
	if raw.RoleMixedMinRatio != nil {
		cfg.RoleMixedMinRatio = *raw.RoleMixedMinRatio
	}
	if raw.RoleMixedMinPositiveScore != nil {
		cfg.RoleMixedMinPositiveScore = *raw.RoleMixedMinPositiveScore
	}
	if raw.RoleMixedMinPositiveRoles != nil {
		cfg.RoleMixedMinPositiveRoles = *raw.RoleMixedMinPositiveRoles
	}
	if raw.RoleMinPrimaryScore != nil {
		cfg.RoleMinPrimaryScore = *raw.RoleMinPrimaryScore
	}
	if raw.RoleMinConfidence != nil {
		cfg.RoleMinConfidence = *raw.RoleMinConfidence
	}
	if raw.ReconciliationResidualGapEscalationAmount != nil {
		cfg.ReconciliationResidualGapEscalationAmount = *raw.ReconciliationResidualGapEscalationAmount
	}
}

func applyNestedRuleConfig(cfg *RuleConfig, raw ruleConfigFile) {
	if len(raw.Router.Stopwords.GenericMetric) > 0 {
		cfg.GenericMetricStopwords = dedupeNonEmpty(raw.Router.Stopwords.GenericMetric)
	}
	for intentKey, intentCfg := range raw.Router.Intents {
		if len(intentCfg.Keywords) > 0 {
			setIntentKeywordGroup(cfg, intentKey, intentCfg.Keywords)
		}
		if intentCfg.Priority != nil {
			cfg.IntentPriority = ensureIntMap(cfg.IntentPriority)
			cfg.IntentPriority[strings.TrimSpace(intentKey)] = *intentCfg.Priority
		}
		if len(intentCfg.Conflicts) > 0 {
			cfg.IntentConflicts = ensureStringSliceMap(cfg.IntentConflicts)
			cfg.IntentConflicts[strings.TrimSpace(intentKey)] = dedupeNonEmpty(intentCfg.Conflicts)
		}
		if intentCfg.MinConfidence != nil {
			cfg.IntentMinConfidence = ensureFloatMap(cfg.IntentMinConfidence)
			cfg.IntentMinConfidence[strings.TrimSpace(intentKey)] = *intentCfg.MinConfidence
		}
		if len(intentCfg.HighPriorityPhrases) > 0 {
			cfg.HighPriorityPhrases = ensureStringSliceMap(cfg.HighPriorityPhrases)
			cfg.HighPriorityPhrases[strings.TrimSpace(intentKey)] = dedupeNonEmpty(intentCfg.HighPriorityPhrases)
		}
	}
	if len(raw.Router.MetricKeywords) > 0 {
		cfg.MetricKeywordLexicon = normalizeStringSliceMap(raw.Router.MetricKeywords)
	}
	if len(raw.Router.HRBreakdownKeywords) > 0 {
		cfg.HRBreakdownKeywordLexicon = dedupeNonEmpty(raw.Router.HRBreakdownKeywords)
	}
	if len(raw.Router.SupplierPaymentExcludeNameKeywords) > 0 {
		cfg.SupplierPaymentExcludeNameLexicon = dedupeNonEmpty(raw.Router.SupplierPaymentExcludeNameKeywords)
	}
	if len(raw.Router.CounterpartyClassificationQuestionKeywords) > 0 {
		cfg.CounterpartyClassificationQuestionLexicon = dedupeNonEmpty(raw.Router.CounterpartyClassificationQuestionKeywords)
	}
	if len(raw.Router.ProfitSingleViewBlockKeywords) > 0 {
		cfg.ProfitSingleViewBlockKeywordLexicon = dedupeNonEmpty(raw.Router.ProfitSingleViewBlockKeywords)
	}
	if len(raw.Router.FallbackMonthlyExpenseKeywords) > 0 {
		cfg.FallbackMonthlyExpenseKeywords = dedupeNonEmpty(raw.Router.FallbackMonthlyExpenseKeywords)
	}
	if len(raw.Counterparty.Roles) > 0 {
		cfg.CounterpartyRoleLexicon = normalizeStringSliceMap(raw.Counterparty.Roles)
	}
	if len(raw.Counterparty.Tax) > 0 {
		cfg.CounterpartyTaxLexicon = normalizeStringSliceMap(raw.Counterparty.Tax)
	}
	if raw.Counterparty.Thresholds.MixedMinRatio != nil {
		cfg.RoleMixedMinRatio = *raw.Counterparty.Thresholds.MixedMinRatio
	}
	if raw.Counterparty.Thresholds.MixedMinPositiveScore != nil {
		cfg.RoleMixedMinPositiveScore = *raw.Counterparty.Thresholds.MixedMinPositiveScore
	}
	if raw.Counterparty.Thresholds.MixedMinPositiveRoles != nil {
		cfg.RoleMixedMinPositiveRoles = *raw.Counterparty.Thresholds.MixedMinPositiveRoles
	}
	if raw.Counterparty.Thresholds.MinPrimaryScore != nil {
		cfg.RoleMinPrimaryScore = *raw.Counterparty.Thresholds.MinPrimaryScore
	}
	if raw.Counterparty.Thresholds.MinConfidence != nil {
		cfg.RoleMinConfidence = *raw.Counterparty.Thresholds.MinConfidence
	}
	if raw.Reconciliation.ResidualGapEscalationAmount != nil {
		cfg.ReconciliationResidualGapEscalationAmount = *raw.Reconciliation.ResidualGapEscalationAmount
	}
	if len(raw.InternalParty.OrgSuffixes) > 0 {
		cfg.InternalPartyOrgSuffixLexicon = dedupeNonEmpty(raw.InternalParty.OrgSuffixes)
	}
	if len(raw.InternalParty.AccountContextKeywords) > 0 {
		cfg.InternalPartyAccountContextKeywordLexicon = dedupeNonEmpty(raw.InternalParty.AccountContextKeywords)
	}
	if len(raw.Contract.PriorityKeywords) > 0 {
		cfg.ContractPriorityKeywordLexicon = dedupeNonEmpty(raw.Contract.PriorityKeywords)
	}
	if len(raw.Contract.SourceTables) > 0 {
		cfg.ContractSourceTableLexicon = normalizeStringSliceMap(raw.Contract.SourceTables)
	}
	if len(raw.Contract.SummaryKeywords) > 0 {
		cfg.ContractSummaryKeywordLexicon = dedupeNonEmpty(raw.Contract.SummaryKeywords)
	}
	if len(raw.Contract.CashFallbackKeywords) > 0 {
		cfg.ContractCashFallbackLexicon = dedupeNonEmpty(raw.Contract.CashFallbackKeywords)
	}
	if len(raw.Accounting.IncomeStatementItems) > 0 {
		cfg.IncomeStatementItemLexicon = normalizeStringSliceMap(raw.Accounting.IncomeStatementItems)
	}
	if len(raw.Accounting.HRBreakdownAccountCodes) > 0 {
		cfg.HRBreakdownAccountCodeLexicon = normalizeStringSliceMap(raw.Accounting.HRBreakdownAccountCodes)
	}
	if len(raw.Accounting.HRCashBankAccountPrefixes) > 0 {
		cfg.HRCashBankAccountPrefixLexicon = dedupeNonEmpty(raw.Accounting.HRCashBankAccountPrefixes)
	}
	if len(raw.Accounting.HRPayrollLiabilityPrefixes) > 0 {
		cfg.HRPayrollLiabilityPrefixLexicon = dedupeNonEmpty(raw.Accounting.HRPayrollLiabilityPrefixes)
	}
	if len(raw.Accounting.HRPayrollLiabilityNames) > 0 {
		cfg.HRPayrollLiabilityNameLexicon = dedupeNonEmpty(raw.Accounting.HRPayrollLiabilityNames)
	}
	if len(raw.Accounting.HRCategoryKeywords) > 0 {
		cfg.HRCategoryKeywordLexicon = normalizeStringSliceMap(raw.Accounting.HRCategoryKeywords)
	}
}
