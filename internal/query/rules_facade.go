package query

import queryrules "financeqa/internal/query/rules"

func parseRuleConfigCSV(raw string) []string {
	return queryrules.ParseCSV(raw)
}

func parseRuleConfigFloat(raw string) (float64, bool) {
	return queryrules.ParseFloat(raw)
}

func parseRuleConfigInt(raw string) (int, bool) {
	return queryrules.ParseInt(raw)
}

func parseRuleConfigStringSliceMap(raw string) (map[string][]string, bool) {
	return queryrules.ParseStringSliceMap(raw)
}

func parseRuleConfigIntMap(raw string) (map[string]int, bool) {
	return queryrules.ParseIntMap(raw)
}

func parseRuleConfigFloatMap(raw string) (map[string]float64, bool) {
	return queryrules.ParseFloatMap(raw)
}

func ensureStringSliceMap(input map[string][]string) map[string][]string {
	return queryrules.EnsureStringSliceMap(input)
}

func ensureIntMap(input map[string]int) map[string]int {
	return queryrules.EnsureIntMap(input)
}

func ensureFloatMap(input map[string]float64) map[string]float64 {
	return queryrules.EnsureFloatMap(input)
}

func normalizeStringSliceMap(input map[string][]string) map[string][]string {
	return queryrules.NormalizeStringSliceMap(input)
}

func normalizeIntMap(input map[string]int) map[string]int {
	return queryrules.NormalizeIntMap(input)
}

func normalizeFloatMap(input map[string]float64) map[string]float64 {
	return queryrules.NormalizeFloatMap(input)
}

func dedupeNonEmpty(items []string) []string {
	return queryrules.DedupeNonEmpty(items)
}
