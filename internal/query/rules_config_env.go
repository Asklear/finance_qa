package query

import (
	"os"
	"strings"
)

func mergeRuleConfigFromEnv(cfg *RuleConfig) {
	mergeRuleConfigFromLookup(cfg, os.Getenv)
}

func mergeRuleConfigFromLookup(cfg *RuleConfig, getenv func(string) string) {
	applyIntentKeywordEnvOverrides(cfg, getenv)
	applyStringSliceEnvOverrides(cfg, getenv)
	applyStructuredEnvOverrides(cfg, getenv)
	applyThresholdEnvOverrides(cfg, getenv)
}

func applyIntentKeywordEnvOverrides(cfg *RuleConfig, getenv func(string) string) {
	overrides := []struct {
		key   string
		group string
	}{
		{key: "FINANCEQA_INTENT_ARAP_KEYWORDS", group: string(IntentARAPQuery)},
		{key: "FINANCEQA_INTENT_HR_COST_KEYWORDS", group: routerGroupHRCost},
		{key: "FINANCEQA_INTENT_TAX_KEYWORDS", group: string(IntentTaxQuery)},
		{key: "FINANCEQA_INTENT_HEALTH_KEYWORDS", group: routerGroupHealth},
		{key: "FINANCEQA_INTENT_FALLBACK_KEYWORDS", group: string(IntentFallback)},
		{key: "FINANCEQA_INTENT_ANALYSIS_KEYWORDS", group: string(IntentAnalysis)},
		{key: "FINANCEQA_INTENT_HOST_PAYLOAD_KEYWORDS", group: string(IntentHostPayload)},
		{key: "FINANCEQA_INTENT_MONTHLY_SUMMARY_KEYWORDS", group: string(IntentMonthlySummary)},
	}
	for _, override := range overrides {
		if values := parseRuleConfigCSV(getenv(override.key)); len(values) > 0 {
			setIntentKeywordGroup(cfg, override.group, values)
		}
	}
}

func applyStringSliceEnvOverrides(cfg *RuleConfig, getenv func(string) string) {
	overrides := []struct {
		key   string
		apply func([]string)
	}{
		{
			key: "FINANCEQA_METRIC_STOPWORDS",
			apply: func(values []string) {
				cfg.GenericMetricStopwords = dedupeNonEmpty(values)
			},
		},
		{
			key: "FINANCEQA_FALLBACK_MONTHLY_EXPENSE_KEYWORDS",
			apply: func(values []string) {
				cfg.FallbackMonthlyExpenseKeywords = dedupeNonEmpty(values)
			},
		},
		{
			key: "FINANCEQA_SUPPLIER_PAYMENT_EXCLUDE_NAME_KEYWORDS",
			apply: func(values []string) {
				cfg.SupplierPaymentExcludeNameLexicon = dedupeNonEmpty(values)
			},
		},
		{
			key: "FINANCEQA_CONTRACT_PRIORITY_KEYWORDS",
			apply: func(values []string) {
				cfg.ContractPriorityKeywordLexicon = dedupeNonEmpty(values)
			},
		},
		{
			key: "FINANCEQA_CONTRACT_SUMMARY_KEYWORDS",
			apply: func(values []string) {
				cfg.ContractSummaryKeywordLexicon = dedupeNonEmpty(values)
			},
		},
		{
			key: "FINANCEQA_CONTRACT_CASH_FALLBACK_KEYWORDS",
			apply: func(values []string) {
				cfg.ContractCashFallbackLexicon = dedupeNonEmpty(values)
			},
		},
	}
	for _, override := range overrides {
		if values := parseRuleConfigCSV(getenv(override.key)); len(values) > 0 {
			override.apply(values)
		}
	}
}

func applyStructuredEnvOverrides(cfg *RuleConfig, getenv func(string) string) {
	stringSliceMapOverrides := []struct {
		key   string
		apply func(map[string][]string)
	}{
		{
			key: "FINANCEQA_HIGH_PRIORITY_PHRASES",
			apply: func(values map[string][]string) {
				cfg.HighPriorityPhrases = normalizeStringSliceMap(values)
			},
		},
		{
			key: "FINANCEQA_CONTRACT_SOURCE_TABLES",
			apply: func(values map[string][]string) {
				cfg.ContractSourceTableLexicon = normalizeStringSliceMap(values)
			},
		},
		{
			key: "FINANCEQA_INCOME_STATEMENT_ITEM_PATTERNS",
			apply: func(values map[string][]string) {
				cfg.IncomeStatementItemLexicon = normalizeStringSliceMap(values)
			},
		},
		{
			key: "FINANCEQA_INTENT_CONFLICTS",
			apply: func(values map[string][]string) {
				cfg.IntentConflicts = normalizeStringSliceMap(values)
			},
		},
	}
	for _, override := range stringSliceMapOverrides {
		if values, ok := parseRuleConfigStringSliceMap(getenv(override.key)); ok {
			override.apply(values)
		}
	}

	if values, ok := parseRuleConfigIntMap(getenv("FINANCEQA_INTENT_PRIORITY")); ok {
		cfg.IntentPriority = normalizeIntMap(values)
	}
	if values, ok := parseRuleConfigFloatMap(getenv("FINANCEQA_INTENT_MIN_CONFIDENCE")); ok {
		cfg.IntentMinConfidence = normalizeFloatMap(values)
	}
}

func applyThresholdEnvOverrides(cfg *RuleConfig, getenv func(string) string) {
	floatOverrides := []struct {
		key   string
		apply func(float64)
	}{
		{
			key: "FINANCEQA_ROLE_MIXED_MIN_RATIO",
			apply: func(v float64) {
				cfg.RoleMixedMinRatio = v
			},
		},
		{
			key: "FINANCEQA_ROLE_MIXED_MIN_POSITIVE_SCORE",
			apply: func(v float64) {
				cfg.RoleMixedMinPositiveScore = v
			},
		},
		{
			key: "FINANCEQA_ROLE_MIN_PRIMARY_SCORE",
			apply: func(v float64) {
				cfg.RoleMinPrimaryScore = v
			},
		},
		{
			key: "FINANCEQA_ROLE_MIN_CONFIDENCE",
			apply: func(v float64) {
				cfg.RoleMinConfidence = v
			},
		},
		{
			key: "FINANCEQA_RECONCILIATION_RESIDUAL_GAP_ESCALATION_AMOUNT",
			apply: func(v float64) {
				cfg.ReconciliationResidualGapEscalationAmount = v
			},
		},
	}
	for _, override := range floatOverrides {
		if value, ok := parseRuleConfigFloat(strings.TrimSpace(getenv(override.key))); ok {
			override.apply(value)
		}
	}
	if value, ok := parseRuleConfigInt(strings.TrimSpace(getenv("FINANCEQA_ROLE_MIXED_MIN_POSITIVE_ROLES"))); ok {
		cfg.RoleMixedMinPositiveRoles = value
	}
}
