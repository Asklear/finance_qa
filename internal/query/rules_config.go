package query

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
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
}

type ruleConfigFile struct {
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

func defaultRuleConfig() RuleConfig {
	return RuleConfig{
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
			string(IntentHostPayload):    120,
			string(IntentARAPQuery):      100,
			string(IntentTaxQuery):       90,
			string(IntentMonthlySummary): 70,
			string(IntentAnalysis):       50,
			string(IntentFallback):       40,
			string(IntentPrecise):        20,
			string(IntentGeneral):        10,
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
	}
}

func getRuleConfig() RuleConfig {
	cfg := defaultRuleConfig()
	mergeRuleConfigFromFile(&cfg)
	mergeRuleConfigFromEnv(&cfg)
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
	if len(raw.GenericMetricStopwords) > 0 {
		cfg.GenericMetricStopwords = dedupeNonEmpty(raw.GenericMetricStopwords)
	}
	if len(raw.IntentARAPKeywords) > 0 {
		cfg.IntentARAPKeywords = dedupeNonEmpty(raw.IntentARAPKeywords)
	}
	if len(raw.IntentHRCostKeywords) > 0 {
		cfg.IntentHRCostKeywords = dedupeNonEmpty(raw.IntentHRCostKeywords)
	}
	if len(raw.IntentTaxKeywords) > 0 {
		cfg.IntentTaxKeywords = dedupeNonEmpty(raw.IntentTaxKeywords)
	}
	if len(raw.IntentHealthKeywords) > 0 {
		cfg.IntentHealthKeywords = dedupeNonEmpty(raw.IntentHealthKeywords)
	}
	if len(raw.IntentFallbackKeywords) > 0 {
		cfg.IntentFallbackKeywords = dedupeNonEmpty(raw.IntentFallbackKeywords)
	}
	if len(raw.IntentAnalysisKeywords) > 0 {
		cfg.IntentAnalysisKeywords = dedupeNonEmpty(raw.IntentAnalysisKeywords)
	}
	if len(raw.IntentHostPayloadKeywords) > 0 {
		cfg.IntentHostPayloadKeywords = dedupeNonEmpty(raw.IntentHostPayloadKeywords)
	}
	if len(raw.IntentMonthlySummaryKeywords) > 0 {
		cfg.IntentMonthlySummaryKeywords = dedupeNonEmpty(raw.IntentMonthlySummaryKeywords)
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

func mergeRuleConfigFromEnv(cfg *RuleConfig) {
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_METRIC_STOPWORDS")); raw != "" {
		cfg.GenericMetricStopwords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_ARAP_KEYWORDS")); raw != "" {
		cfg.IntentARAPKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_HR_COST_KEYWORDS")); raw != "" {
		cfg.IntentHRCostKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_TAX_KEYWORDS")); raw != "" {
		cfg.IntentTaxKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_HEALTH_KEYWORDS")); raw != "" {
		cfg.IntentHealthKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_FALLBACK_KEYWORDS")); raw != "" {
		cfg.IntentFallbackKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_ANALYSIS_KEYWORDS")); raw != "" {
		cfg.IntentAnalysisKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_HOST_PAYLOAD_KEYWORDS")); raw != "" {
		cfg.IntentHostPayloadKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_INTENT_MONTHLY_SUMMARY_KEYWORDS")); raw != "" {
		cfg.IntentMonthlySummaryKeywords = dedupeNonEmpty(strings.Split(raw, ","))
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCEQA_FALLBACK_MONTHLY_EXPENSE_KEYWORDS")); raw != "" {
		cfg.FallbackMonthlyExpenseKeywords = dedupeNonEmpty(strings.Split(raw, ","))
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
