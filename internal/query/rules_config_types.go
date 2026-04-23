package query

import "sync"

const (
	routerGroupHRCost      = "hr_cost"
	routerGroupHealth      = "health"
	routerGroupFallback    = "fallback"
	routerGroupAnalysis    = "analysis"
	routerGroupHostPayload = "host_payload"
	routerGroupMonthly     = "monthly_summary"
	metricKeyRevenue       = "revenue"
	metricKeyCost          = "cost"
	metricKeyProfit        = "profit"
	contractRoleDefault    = "default"
)

var (
	ruleConfigCacheMu   sync.RWMutex
	ruleConfigCacheKey  string
	ruleConfigCacheData RuleConfig
)

// RuleConfig 定义查询层可调规则（默认值 + 外部覆盖）。
type RuleConfig struct {
	GenericMetricStopwords         []string            `json:"generic_metric_stopwords"`
	IntentARAPKeywords             []string            `json:"intent_arap_keywords"`
	IntentHRCostKeywords           []string            `json:"intent_hr_cost_keywords"`
	IntentTaxKeywords              []string            `json:"intent_tax_keywords"`
	IntentHealthKeywords           []string            `json:"intent_health_keywords"`
	IntentFallbackKeywords         []string            `json:"intent_fallback_keywords"`
	IntentAnalysisKeywords         []string            `json:"intent_analysis_keywords"`
	IntentHostPayloadKeywords      []string            `json:"intent_host_payload_keywords"`
	IntentMonthlySummaryKeywords   []string            `json:"intent_monthly_summary_keywords"`
	FallbackMonthlyExpenseKeywords []string            `json:"fallback_monthly_expense_keywords"`
	HighPriorityPhrases            map[string][]string `json:"high_priority_phrases"`
	IntentPriority                 map[string]int      `json:"intent_priority"`
	IntentConflicts                map[string][]string `json:"intent_conflicts"`
	IntentMinConfidence            map[string]float64  `json:"intent_min_confidence"`
	RoleMixedMinRatio              float64             `json:"role_mixed_min_ratio"`
	RoleMixedMinPositiveScore      float64             `json:"role_mixed_min_positive_score"`
	RoleMixedMinPositiveRoles      int                 `json:"role_mixed_min_positive_roles"`
	RoleMinPrimaryScore            float64             `json:"role_min_primary_score"`
	RoleMinConfidence              float64             `json:"role_min_confidence"`
	ContractPriorityKeywordLexicon []string            `json:"-"`
	ContractSourceTableLexicon     map[string][]string `json:"-"`
	ContractSummaryKeywordLexicon  []string            `json:"-"`
	ContractCashFallbackLexicon    []string            `json:"-"`
	IncomeStatementItemLexicon     map[string][]string `json:"-"`

	IntentKeywordLexicon                      map[string][]string `json:"-"`
	MetricKeywordLexicon                      map[string][]string `json:"-"`
	HRBreakdownKeywordLexicon                 []string            `json:"-"`
	SupplierPaymentExcludeNameLexicon         []string            `json:"-"`
	CounterpartyClassificationQuestionLexicon []string            `json:"-"`
	ProfitSingleViewBlockKeywordLexicon       []string            `json:"-"`
	CounterpartyRoleLexicon                   map[string][]string `json:"-"`
	CounterpartyTaxLexicon                    map[string][]string `json:"-"`
	InternalPartyOrgSuffixLexicon             []string            `json:"-"`
	InternalPartyAccountContextKeywordLexicon []string            `json:"-"`
}

type ruleConfigFile struct {
	SchemaVersion int                         `json:"schema_version"`
	Router        routerRuleConfigFile        `json:"router"`
	Counterparty  counterpartyRuleConfigFile  `json:"counterparty"`
	InternalParty internalPartyRuleConfigFile `json:"internal_party"`
	Contract      contractRuleConfigFile      `json:"contract"`
	Accounting    accountingRuleConfigFile    `json:"accounting"`

	GenericMetricStopwords         []string            `json:"generic_metric_stopwords"`
	IntentARAPKeywords             []string            `json:"intent_arap_keywords"`
	IntentHRCostKeywords           []string            `json:"intent_hr_cost_keywords"`
	IntentTaxKeywords              []string            `json:"intent_tax_keywords"`
	IntentHealthKeywords           []string            `json:"intent_health_keywords"`
	IntentFallbackKeywords         []string            `json:"intent_fallback_keywords"`
	IntentAnalysisKeywords         []string            `json:"intent_analysis_keywords"`
	IntentHostPayloadKeywords      []string            `json:"intent_host_payload_keywords"`
	IntentMonthlySummaryKeywords   []string            `json:"intent_monthly_summary_keywords"`
	FallbackMonthlyExpenseKeywords []string            `json:"fallback_monthly_expense_keywords"`
	HighPriorityPhrases            map[string][]string `json:"high_priority_phrases"`
	IntentPriority                 map[string]int      `json:"intent_priority"`
	IntentConflicts                map[string][]string `json:"intent_conflicts"`
	IntentMinConfidence            map[string]float64  `json:"intent_min_confidence"`
	RoleMixedMinRatio              *float64            `json:"role_mixed_min_ratio"`
	RoleMixedMinPositiveScore      *float64            `json:"role_mixed_min_positive_score"`
	RoleMixedMinPositiveRoles      *int                `json:"role_mixed_min_positive_roles"`
	RoleMinPrimaryScore            *float64            `json:"role_min_primary_score"`
	RoleMinConfidence              *float64            `json:"role_min_confidence"`
}

type routerRuleConfigFile struct {
	Stopwords                                  routerStopwordsRuleConfigFile         `json:"stopwords"`
	Intents                                    map[string]routerIntentRuleConfigFile `json:"intents"`
	MetricKeywords                             map[string][]string                   `json:"metric_keywords"`
	HRBreakdownKeywords                        []string                              `json:"hr_breakdown_keywords"`
	SupplierPaymentExcludeNameKeywords         []string                              `json:"supplier_payment_exclude_name_keywords"`
	CounterpartyClassificationQuestionKeywords []string                              `json:"counterparty_classification_question_keywords"`
	ProfitSingleViewBlockKeywords              []string                              `json:"profit_single_view_block_keywords"`
	FallbackMonthlyExpenseKeywords             []string                              `json:"fallback_monthly_expense_keywords"`
}

type routerStopwordsRuleConfigFile struct {
	GenericMetric []string `json:"generic_metric"`
}

type routerIntentRuleConfigFile struct {
	Keywords            []string `json:"keywords"`
	Priority            *int     `json:"priority"`
	MinConfidence       *float64 `json:"min_confidence"`
	Conflicts           []string `json:"conflicts"`
	HighPriorityPhrases []string `json:"high_priority_phrases"`
}

type counterpartyRuleConfigFile struct {
	Roles      map[string][]string                 `json:"roles"`
	Tax        map[string][]string                 `json:"tax"`
	Thresholds counterpartyThresholdRuleConfigFile `json:"thresholds"`
}

type counterpartyThresholdRuleConfigFile struct {
	MixedMinRatio         *float64 `json:"mixed_min_ratio"`
	MixedMinPositiveScore *float64 `json:"mixed_min_positive_score"`
	MixedMinPositiveRoles *int     `json:"mixed_min_positive_roles"`
	MinPrimaryScore       *float64 `json:"min_primary_score"`
	MinConfidence         *float64 `json:"min_confidence"`
}

type internalPartyRuleConfigFile struct {
	OrgSuffixes            []string `json:"org_suffixes"`
	AccountContextKeywords []string `json:"account_context_keywords"`
}

type contractRuleConfigFile struct {
	PriorityKeywords     []string            `json:"priority_keywords"`
	SourceTables         map[string][]string `json:"source_tables"`
	SummaryKeywords      []string            `json:"summary_keywords"`
	CashFallbackKeywords []string            `json:"cash_fallback_keywords"`
}

type accountingRuleConfigFile struct {
	IncomeStatementItems map[string][]string `json:"income_statement_items"`
}
