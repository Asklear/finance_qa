package query

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// RuleConfig 定义查询层可调规则（默认值 + 外部覆盖）。
type RuleConfig struct {
	GenericMetricStopwords      []string `json:"generic_metric_stopwords"`
	RoleMixedMinRatio           float64  `json:"role_mixed_min_ratio"`
	RoleMixedMinPositiveScore   float64  `json:"role_mixed_min_positive_score"`
	RoleMixedMinPositiveRoles   int      `json:"role_mixed_min_positive_roles"`
	RoleMinPrimaryScore         float64  `json:"role_min_primary_score"`
	RoleMinConfidence           float64  `json:"role_min_confidence"`
}

type ruleConfigFile struct {
	GenericMetricStopwords    []string  `json:"generic_metric_stopwords"`
	RoleMixedMinRatio         *float64  `json:"role_mixed_min_ratio"`
	RoleMixedMinPositiveScore *float64  `json:"role_mixed_min_positive_score"`
	RoleMixedMinPositiveRoles *int      `json:"role_mixed_min_positive_roles"`
	RoleMinPrimaryScore       *float64  `json:"role_min_primary_score"`
	RoleMinConfidence         *float64  `json:"role_min_confidence"`
}

func defaultRuleConfig() RuleConfig {
	return RuleConfig{
		GenericMetricStopwords: []string{
			"收入", "营收", "销售额",
			"成本", "总成本", "人力成本", "工资成本", "薪酬成本",
			"利润", "毛利", "净利",
			"支出", "费用", "整体支出", "总支出", "全部支出",
			"销项税", "销项税额", "进项税", "进项税额", "税额",
			"应收", "应付", "应收账款", "应付账款",
			"现金流", "流水", "回款", "到账", "收款", "付款",
			"经营状况", "指标", "核心指标", "月度经营",
		},
		RoleMixedMinRatio:         0.45,
		RoleMixedMinPositiveScore: 1.0,
		RoleMixedMinPositiveRoles: 2,
		RoleMinPrimaryScore:       0.0,
		RoleMinConfidence:         0.0,
	}
}

func getRuleConfig() RuleConfig {
	cfg := defaultRuleConfig()
	mergeRuleConfigFromFile(&cfg)
	mergeRuleConfigFromEnv(&cfg)
	return cfg
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

