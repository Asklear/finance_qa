package query

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

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

func defaultRuleConfig() RuleConfig {
	cfg := RuleConfig{
		GenericMetricStopwords: []string{
			"收入", "营收", "销售额",
			"成本", "总成本", "人力成本", "工资成本", "薪酬成本",
			"工资", "社保", "公积金",
			"利润", "毛利", "净利",
			"支出", "费用", "整体支出", "总支出", "全部支出",
			"销项税", "销项税额", "进项税", "进项税额", "税额",
			"应收", "应付", "应收账款", "应付账款",
			"现金流", "流水", "回款", "到账", "收款", "付款",
			"经营状况", "指标", "核心指标", "月度经营",
		},
		IntentARAPKeywords: []string{
			"应收", "应付", "账款", "往来款",
		},
		IntentHRCostKeywords: []string{
			"人力成本", "工资成本", "薪酬成本", "应付职工薪酬",
		},
		IntentTaxKeywords: []string{
			"税", "销项", "进项", "增值税",
		},
		IntentHealthKeywords: []string{
			"健康度", "健康", "怎么样",
		},
		IntentFallbackKeywords: []string{
			"健康度", "健康", "怎么样",
			"供应商多少", "多少供应商", "供应商有多少",
			"人力成本", "工资成本", "薪酬成本", "应付职工薪酬",
			"整体支出", "总支出", "全部支出",
		},
		IntentAnalysisKeywords: []string{
			"分析", "评分", "评价", "风险", "分析下",
		},
		IntentHostPayloadKeywords: []string{
			"宿主llm", "hostllm", "原始数据", "全量财报", "财报原始", "llm数据包",
		},
		IntentMonthlySummaryKeywords: []string{
			"概括", "总结", "利润", "指标", "经营状况", "收入", "支出", "支出汇总", "报销汇总", "成本", "总成本", "费用总额",
		},
		FallbackMonthlyExpenseKeywords: []string{
			"整体支出", "总支出", "全部支出", "支出汇总",
		},
		HighPriorityPhrases: map[string][]string{
			string(IntentARAPQuery): {"预收款", "预付款", "应收账款", "应付账款"},
		},
		IntentPriority: map[string]int{
			string(IntentHostPayload):           120,
			string(IntentLargeTransactionQuery): 110,
			string(IntentIdentityQuery):         105,
			string(IntentARAPQuery):             100,
			string(IntentTaxQuery):              90,
			string(IntentMonthlySummary):        70,
			string(IntentAnalysis):              50,
			string(IntentFallback):              40,
			string(IntentPrecise):               20,
			string(IntentGeneral):               10,
		},
		IntentConflicts: map[string][]string{
			string(IntentARAPQuery):      {string(IntentFallback), string(IntentGeneral)},
			string(IntentTaxQuery):       {string(IntentFallback), string(IntentGeneral)},
			string(IntentMonthlySummary): {string(IntentGeneral)},
		},
		IntentMinConfidence: map[string]float64{
			string(IntentARAPQuery):      0.6,
			string(IntentTaxQuery):       0.55,
			string(IntentMonthlySummary): 0.5,
			string(IntentAnalysis):       0.5,
			string(IntentFallback):       0.45,
		},
		RoleMixedMinRatio:         0.45,
		RoleMixedMinPositiveScore: 1.0,
		RoleMixedMinPositiveRoles: 2,
		RoleMinPrimaryScore:       0.5,
		RoleMinConfidence:         0.0,
		IntentKeywordLexicon: map[string][]string{
			string(IntentARAPQuery):             {"应收", "应付", "账款", "往来款"},
			routerGroupHRCost:                   {"人力成本", "工资成本", "薪酬成本", "应付职工薪酬"},
			string(IntentTaxQuery):              {"税", "销项", "进项", "增值税"},
			routerGroupHealth:                   {"健康度", "健康", "怎么样"},
			string(IntentFallback):              {"健康度", "健康", "怎么样", "供应商多少", "多少供应商", "供应商有多少", "人力成本", "工资成本", "薪酬成本", "应付职工薪酬", "整体支出", "总支出", "全部支出"},
			string(IntentAnalysis):              {"分析", "评分", "评价", "风险", "分析下"},
			string(IntentHostPayload):           {"宿主llm", "hostllm", "原始数据", "全量财报", "财报原始", "llm数据包"},
			string(IntentMonthlySummary):        {"概括", "总结", "利润", "指标", "经营状况", "收入", "支出", "支出汇总", "报销汇总", "成本", "总成本", "费用总额"},
			string(IntentLargeTransactionQuery): {"最大", "单笔", "流入对手方", "流出对手方"},
			string(IntentIdentityQuery):         {"是谁", "身份", "干嘛的", "哪里的", "谁是"},
			string(IntentPrecise):               {"期末", "余额", "是多少", "查询余额", "还有多少"},
		},
		MetricKeywordLexicon: map[string][]string{
			metricKeyRevenue: {"收入", "营收", "销售额"},
			metricKeyCost:    {"成本"},
			metricKeyProfit:  {"利润"},
		},
		HRBreakdownKeywordLexicon:                 []string{"工资", "社保", "公积金", "分别", "拆分", "拆开", "明细", "构成"},
		SupplierPaymentExcludeNameLexicon:         []string{"暂收款", "暂付款", "汇划收入", "中间业务收入", "手续费", "利息收入", "利息"},
		CounterpartyClassificationQuestionLexicon: []string{"成本还是收入", "是成本还是收入", "供应商付款还是预收款", "客户还是供应商"},
		ProfitSingleViewBlockKeywordLexicon:       []string{"现金流", "回款", "到账", "银行卡", "差异", "为什么"},
		CounterpartyRoleLexicon: map[string][]string{
			string(CounterpartyCustomer): {"应收", "回款", "收款", "结算款", "销售", "收入", "主营业务收入", "营业收入", "预收", "合同资产", "客户", "1122", "1121"},
			string(CounterpartySupplier): {"应付", "付款", "采购", "成本", "材料", "供应商", "外包", "2202", "预付账款", "1123", "112301"},
			string(CounterpartyEmployee): {"工资", "薪酬", "社保", "公积金", "报销", "差旅", "福利", "餐补", "伙食", "应付职工薪酬", "2211"},
		},
		CounterpartyTaxLexicon: map[string][]string{
			string(TaxSideOutput): {"销项税", "222101", "销项"},
			string(TaxSideInput):  {"进项税", "222102", "进项"},
		},
		InternalPartyOrgSuffixLexicon:             []string{"分公司", "子公司", "事业部", "办事处", "分部", "总部", "总公司"},
		InternalPartyAccountContextKeywordLexicon: []string{"应付职工薪酬", "其他应收款", "其他应付款", "内部往来"},
	}
	cfg.finalize()
	return cfg
}

func getRuleConfig() RuleConfig {
	cfg := defaultRuleConfig()
	mergeRuleConfigFromFile(&cfg)
	mergeRuleConfigFromEnv(&cfg)
	cfg.finalize()
	return cfg
}

// CurrentRuleConfig 返回当前生效规则（默认值 + 文件覆盖 + 环境变量覆盖）。
func CurrentRuleConfig() RuleConfig {
	return getRuleConfig()
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
	if v, ok := parseEnvStringSliceMap("FINANCEQA_HIGH_PRIORITY_PHRASES"); ok {
		cfg.HighPriorityPhrases = normalizeStringSliceMap(v)
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

func (cfg *RuleConfig) finalize() {
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

func ensureStringSliceMap(input map[string][]string) map[string][]string {
	if input == nil {
		return map[string][]string{}
	}
	return input
}

func ensureIntMap(input map[string]int) map[string]int {
	if input == nil {
		return map[string]int{}
	}
	return input
}

func ensureFloatMap(input map[string]float64) map[string]float64 {
	if input == nil {
		return map[string]float64{}
	}
	return input
}

func parseEnvFloat(key string) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseEnvInt(key string) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseEnvStringSliceMap(key string) (map[string][]string, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, false
	}
	var v map[string][]string
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false
	}
	return v, true
}

func parseEnvIntMap(key string) (map[string]int, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, false
	}
	var v map[string]int
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false
	}
	return v, true
}

func parseEnvFloatMap(key string) (map[string]float64, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, false
	}
	var v map[string]float64
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false
	}
	return v, true
}

func normalizeStringSliceMap(input map[string][]string) map[string][]string {
	out := make(map[string][]string, len(input))
	for key, values := range input {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		normalizedValues := dedupeNonEmpty(values)
		if len(normalizedValues) == 0 {
			continue
		}
		out[trimmedKey] = normalizedValues
	}
	return out
}

func normalizeIntMap(input map[string]int) map[string]int {
	out := make(map[string]int, len(input))
	for key, value := range input {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = value
	}
	return out
}

func normalizeFloatMap(input map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(input))
	for key, value := range input {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = value
	}
	return out
}

func dedupeNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := normalizeEntityText(trimmed)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
