package query

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

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
	cfg.ContractPriorityKeywordLexicon = dedupeNonEmpty(cfg.ContractPriorityKeywordLexicon)
	cfg.ContractSourceTableLexicon = normalizeStringSliceMap(cfg.ContractSourceTableLexicon)
	cfg.ContractSummaryKeywordLexicon = dedupeNonEmpty(cfg.ContractSummaryKeywordLexicon)
	cfg.ContractCashFallbackLexicon = dedupeNonEmpty(cfg.ContractCashFallbackLexicon)
	cfg.IncomeStatementItemLexicon = normalizeStringSliceMap(cfg.IncomeStatementItemLexicon)
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
