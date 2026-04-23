package query

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
)

func getRuleConfig() RuleConfig {
	cacheKey := currentRuleConfigCacheKey()
	ruleConfigCacheMu.RLock()
	if cacheKey != "" && cacheKey == ruleConfigCacheKey {
		cfg := ruleConfigCacheData
		ruleConfigCacheMu.RUnlock()
		return cfg
	}
	ruleConfigCacheMu.RUnlock()

	cfg := defaultRuleConfig()
	mergeRuleConfigFromFile(&cfg)
	mergeRuleConfigFromEnv(&cfg)
	cfg.finalize()

	ruleConfigCacheMu.Lock()
	ruleConfigCacheKey = cacheKey
	ruleConfigCacheData = cfg
	ruleConfigCacheMu.Unlock()
	return cfg
}

// CurrentRuleConfig 返回当前生效规则（默认值 + 文件覆盖 + 环境变量覆盖）。
func CurrentRuleConfig() RuleConfig {
	return getRuleConfig()
}

func currentRuleConfigCacheKey() string {
	envs := os.Environ()
	filtered := make([]string, 0, len(envs))
	for _, entry := range envs {
		if strings.HasPrefix(entry, "FINANCEQA_") {
			filtered = append(filtered, entry)
		}
	}
	sort.Strings(filtered)

	var b strings.Builder
	for _, entry := range filtered {
		b.WriteString(entry)
		b.WriteByte('\n')
	}

	path := strings.TrimSpace(os.Getenv("FINANCEQA_RULES_PATH"))
	if path != "" {
		if stat, err := os.Stat(path); err == nil {
			b.WriteString("rules_path_stat=")
			b.WriteString(path)
			b.WriteByte('|')
			b.WriteString(strconv.FormatInt(stat.Size(), 10))
			b.WriteByte('|')
			b.WriteString(strconv.FormatInt(stat.ModTime().UnixNano(), 10))
		} else {
			b.WriteString("rules_path_missing=")
			b.WriteString(path)
		}
	}

	return b.String()
}

func mergeRuleConfigFromFile(cfg *RuleConfig) {
	path := strings.TrimSpace(os.Getenv("FINANCEQA_RULES_PATH"))
	if path == "" {
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var raw ruleConfigFile
	if err := json.Unmarshal(content, &raw); err != nil {
		return
	}
	applyLegacyRuleConfig(cfg, raw)
	if raw.SchemaVersion >= 2 {
		applyNestedRuleConfig(cfg, raw)
	}
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
}

func mergeRuleConfigFromEnv(cfg *RuleConfig) {
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_METRIC_STOPWORDS")); raw != "" {
		cfg.GenericMetricStopwords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_ARAP_KEYWORDS")); raw != "" {
		setIntentKeywordGroup(cfg, string(IntentARAPQuery), strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_HR_COST_KEYWORDS")); raw != "" {
		setIntentKeywordGroup(cfg, routerGroupHRCost, strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_TAX_KEYWORDS")); raw != "" {
		setIntentKeywordGroup(cfg, string(IntentTaxQuery), strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_HEALTH_KEYWORDS")); raw != "" {
		setIntentKeywordGroup(cfg, routerGroupHealth, strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_FALLBACK_KEYWORDS")); raw != "" {
		setIntentKeywordGroup(cfg, string(IntentFallback), strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_ANALYSIS_KEYWORDS")); raw != "" {
		setIntentKeywordGroup(cfg, string(IntentAnalysis), strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_HOST_PAYLOAD_KEYWORDS")); raw != "" {
		setIntentKeywordGroup(cfg, string(IntentHostPayload), strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_MONTHLY_SUMMARY_KEYWORDS")); raw != "" {
		setIntentKeywordGroup(cfg, string(IntentMonthlySummary), strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_FALLBACK_MONTHLY_EXPENSE_KEYWORDS")); raw != "" {
		cfg.FallbackMonthlyExpenseKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_SUPPLIER_PAYMENT_EXCLUDE_NAME_KEYWORDS")); raw != "" {
		cfg.SupplierPaymentExcludeNameLexicon = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_CONTRACT_PRIORITY_KEYWORDS")); raw != "" {
		cfg.ContractPriorityKeywordLexicon = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_CONTRACT_SUMMARY_KEYWORDS")); raw != "" {
		cfg.ContractSummaryKeywordLexicon = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_CONTRACT_CASH_FALLBACK_KEYWORDS")); raw != "" {
		cfg.ContractCashFallbackLexicon = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if v, ok := parseEnvStringSliceMap("FINANCEQA_HIGH_PRIORITY_PHRASES"); ok {
		cfg.HighPriorityPhrases = normalizeStringSliceMap(v)
	}
	if v, ok := parseEnvStringSliceMap("FINANCEQA_CONTRACT_SOURCE_TABLES"); ok {
		cfg.ContractSourceTableLexicon = normalizeStringSliceMap(v)
	}
	if v, ok := parseEnvStringSliceMap("FINANCEQA_INCOME_STATEMENT_ITEM_PATTERNS"); ok {
		cfg.IncomeStatementItemLexicon = normalizeStringSliceMap(v)
	}
	if v, ok := parseEnvIntMap("FINANCEQA_INTENT_PRIORITY"); ok {
		cfg.IntentPriority = normalizeIntMap(v)
	}
	if v, ok := parseEnvStringSliceMap("FINANCEQA_INTENT_CONFLICTS"); ok {
		cfg.IntentConflicts = normalizeStringSliceMap(v)
	}
	if v, ok := parseEnvFloatMap("FINANCEQA_INTENT_MIN_CONFIDENCE"); ok {
		cfg.IntentMinConfidence = normalizeFloatMap(v)
	}
	if v, ok := parseEnvFloat("FINANCEQA_ROLE_MIXED_MIN_RATIO"); ok {
		cfg.RoleMixedMinRatio = v
	}
	if v, ok := parseEnvFloat("FINANCEQA_ROLE_MIXED_MIN_POSITIVE_SCORE"); ok {
		cfg.RoleMixedMinPositiveScore = v
	}
	if v, ok := parseEnvInt("FINANCEQA_ROLE_MIXED_MIN_POSITIVE_ROLES"); ok {
		cfg.RoleMixedMinPositiveRoles = v
	}
	if v, ok := parseEnvFloat("FINANCEQA_ROLE_MIN_PRIMARY_SCORE"); ok {
		cfg.RoleMinPrimaryScore = v
	}
	if v, ok := parseEnvFloat("FINANCEQA_ROLE_MIN_CONFIDENCE"); ok {
		cfg.RoleMinConfidence = v
	}
}
