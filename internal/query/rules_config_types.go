package query

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

// RuleConfig 定义查询层可调规则（默认值 + 外部覆盖）。
type RuleConfig struct {
	GenericMetricStopwords                    []string            `json:"generic_metric_stopwords"`
	IntentARAPKeywords                        []string            `json:"intent_arap_keywords"`
	IntentHRCostKeywords                      []string            `json:"intent_hr_cost_keywords"`
	IntentTaxKeywords                         []string            `json:"intent_tax_keywords"`
	IntentHealthKeywords                      []string            `json:"intent_health_keywords"`
	IntentFallbackKeywords                    []string            `json:"intent_fallback_keywords"`
	IntentAnalysisKeywords                    []string            `json:"intent_analysis_keywords"`
	IntentHostPayloadKeywords                 []string            `json:"intent_host_payload_keywords"`
	IntentMonthlySummaryKeywords              []string            `json:"intent_monthly_summary_keywords"`
	FallbackMonthlyExpenseKeywords            []string            `json:"fallback_monthly_expense_keywords"`
	HighPriorityPhrases                       map[string][]string `json:"high_priority_phrases"`
	IntentPriority                            map[string]int      `json:"intent_priority"`
	IntentConflicts                           map[string][]string `json:"intent_conflicts"`
	IntentMinConfidence                       map[string]float64  `json:"intent_min_confidence"`
	RoleMixedMinRatio                         float64             `json:"role_mixed_min_ratio"`
	RoleMixedMinPositiveScore                 float64             `json:"role_mixed_min_positive_score"`
	RoleMixedMinPositiveRoles                 int                 `json:"role_mixed_min_positive_roles"`
	RoleMinPrimaryScore                       float64             `json:"role_min_primary_score"`
	RoleMinConfidence                         float64             `json:"role_min_confidence"`
	ReconciliationResidualGapEscalationAmount float64             `json:"reconciliation_residual_gap_escalation_amount"`
	ContractPriorityKeywordLexicon            []string            `json:"-"`
	ContractSourceTableLexicon                map[string][]string `json:"-"`
	ContractSummaryKeywordLexicon             []string            `json:"-"`
	ContractCashFallbackLexicon               []string            `json:"-"`
	IncomeStatementItemLexicon                map[string][]string `json:"-"`
	HRBreakdownAccountCodeLexicon             map[string][]string `json:"-"`
	HRCashBankAccountPrefixLexicon            []string            `json:"-"`
	HRPayrollLiabilityPrefixLexicon           []string            `json:"-"`
	HRPayrollLiabilityNameLexicon             []string            `json:"-"`
	HRCategoryKeywordLexicon                  map[string][]string `json:"-"`

	IntentKeywordLexicon                      map[string][]string                 `json:"-"`
	MetricKeywordLexicon                      map[string][]string                 `json:"-"`
	HRBreakdownKeywordLexicon                 []string                            `json:"-"`
	SupplierPaymentExcludeNameLexicon         []string                            `json:"-"`
	CounterpartyClassificationQuestionLexicon []string                            `json:"-"`
	ProfitSingleViewBlockKeywordLexicon       []string                            `json:"-"`
	ExpenseBreakdownTriggerKeywordLexicon     []string                            `json:"-"`
	ExpenseBreakdownExpenseKeywordLexicon     []string                            `json:"-"`
	ExpenseBreakdownMetricBlockKeywordLexicon []string                            `json:"-"`
	ExpenseBreakdownMetricAllowKeywordLexicon []string                            `json:"-"`
	ExpenseBreakdownCostKeywordLexicon        []string                            `json:"-"`
	ExpenseBreakdownMetricLabel               string                              `json:"-"`
	ExpenseBreakdownViewLexicon               map[string]ExpenseBreakdownViewRule `json:"-"`
	ExpenseBreakdownCashCategoryLexicon       []ExpenseBreakdownCategoryRule      `json:"-"`
	ExpenseBreakdownCashDefaultCategory       string                              `json:"-"`
	ExpenseBreakdownAccountCategoryLexicon    []ExpenseBreakdownCategoryRule      `json:"-"`
	ExpenseBreakdownAccountDefaultCategory    string                              `json:"-"`
	CounterpartyRoleLexicon                   map[string][]string                 `json:"-"`
	CounterpartyTaxLexicon                    map[string][]string                 `json:"-"`
	InternalPartyOrgSuffixLexicon             []string                            `json:"-"`
	InternalPartyAccountContextKeywordLexicon []string                            `json:"-"`
}

type ruleConfigFile struct {
	SchemaVersion  int                          `json:"schema_version"`
	Router         routerRuleConfigFile         `json:"router"`
	Counterparty   counterpartyRuleConfigFile   `json:"counterparty"`
	InternalParty  internalPartyRuleConfigFile  `json:"internal_party"`
	Contract       contractRuleConfigFile       `json:"contract"`
	Accounting     accountingRuleConfigFile     `json:"accounting"`
	Reconciliation reconciliationRuleConfigFile `json:"reconciliation"`

	GenericMetricStopwords                    []string            `json:"generic_metric_stopwords"`
	IntentARAPKeywords                        []string            `json:"intent_arap_keywords"`
	IntentHRCostKeywords                      []string            `json:"intent_hr_cost_keywords"`
	IntentTaxKeywords                         []string            `json:"intent_tax_keywords"`
	IntentHealthKeywords                      []string            `json:"intent_health_keywords"`
	IntentFallbackKeywords                    []string            `json:"intent_fallback_keywords"`
	IntentAnalysisKeywords                    []string            `json:"intent_analysis_keywords"`
	IntentHostPayloadKeywords                 []string            `json:"intent_host_payload_keywords"`
	IntentMonthlySummaryKeywords              []string            `json:"intent_monthly_summary_keywords"`
	FallbackMonthlyExpenseKeywords            []string            `json:"fallback_monthly_expense_keywords"`
	HighPriorityPhrases                       map[string][]string `json:"high_priority_phrases"`
	IntentPriority                            map[string]int      `json:"intent_priority"`
	IntentConflicts                           map[string][]string `json:"intent_conflicts"`
	IntentMinConfidence                       map[string]float64  `json:"intent_min_confidence"`
	RoleMixedMinRatio                         *float64            `json:"role_mixed_min_ratio"`
	RoleMixedMinPositiveScore                 *float64            `json:"role_mixed_min_positive_score"`
	RoleMixedMinPositiveRoles                 *int                `json:"role_mixed_min_positive_roles"`
	RoleMinPrimaryScore                       *float64            `json:"role_min_primary_score"`
	RoleMinConfidence                         *float64            `json:"role_min_confidence"`
	ReconciliationResidualGapEscalationAmount *float64            `json:"reconciliation_residual_gap_escalation_amount"`
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
	ExpenseBreakdown                           expenseBreakdownRuleConfigFile        `json:"expense_breakdown"`
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

type expenseBreakdownRuleConfigFile struct {
	TriggerKeywords        []string                            `json:"trigger_keywords"`
	ExpenseKeywords        []string                            `json:"expense_keywords"`
	MetricBlockKeywords    []string                            `json:"metric_block_keywords"`
	MetricAllowKeywords    []string                            `json:"metric_allow_keywords"`
	CostKeywords           []string                            `json:"cost_keywords"`
	MetricLabel            string                              `json:"metric_label"`
	Views                  map[string]ExpenseBreakdownViewRule `json:"views"`
	CashCategories         []ExpenseBreakdownCategoryRule      `json:"cash_categories"`
	CashDefaultCategory    string                              `json:"cash_default_category"`
	AccountCategories      []ExpenseBreakdownCategoryRule      `json:"account_categories"`
	AccountDefaultCategory string                              `json:"account_default_category"`
}

type ExpenseBreakdownViewRule struct {
	Label        string `json:"label"`
	Description  string `json:"description"`
	SummaryLimit int    `json:"summary_limit"`
}

type ExpenseBreakdownCategoryRule struct {
	Category             string   `json:"category"`
	Keywords             []string `json:"keywords"`
	CounterpartyRole     string   `json:"counterparty_role"`
	InternalParty        bool     `json:"internal_party"`
	ExternalOrganization bool     `json:"external_organization"`
	AccountCodePrefixes  []string `json:"account_code_prefixes"`
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
	IncomeStatementItems       map[string][]string `json:"income_statement_items"`
	HRBreakdownAccountCodes    map[string][]string `json:"hr_breakdown_account_codes"`
	HRCashBankAccountPrefixes  []string            `json:"hr_cash_bank_account_prefixes"`
	HRPayrollLiabilityPrefixes []string            `json:"hr_payroll_liability_account_prefixes"`
	HRPayrollLiabilityNames    []string            `json:"hr_payroll_liability_name_keywords"`
	HRCategoryKeywords         map[string][]string `json:"hr_category_keywords"`
}

type reconciliationRuleConfigFile struct {
	ResidualGapEscalationAmount *float64 `json:"residual_gap_escalation_amount"`
}
