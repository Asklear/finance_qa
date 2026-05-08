package rules

import "strings"

func EnsureStringSliceMap(input map[string][]string) map[string][]string {
	if input == nil {
		return map[string][]string{}
	}
	return input
}

func EnsureIntMap(input map[string]int) map[string]int {
	if input == nil {
		return map[string]int{}
	}
	return input
}

func EnsureFloatMap(input map[string]float64) map[string]float64 {
	if input == nil {
		return map[string]float64{}
	}
	return input
}

func NormalizeStringSliceMap(input map[string][]string) map[string][]string {
	out := make(map[string][]string, len(input))
	for key, values := range input {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		normalizedValues := DedupeNonEmpty(values)
		if len(normalizedValues) == 0 {
			continue
		}
		out[trimmedKey] = normalizedValues
	}
	return out
}

func NormalizeIntMap(input map[string]int) map[string]int {
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

func NormalizeFloatMap(input map[string]float64) map[string]float64 {
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

func DedupeNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := normalizeRuleText(trimmed)
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

func normalizeRuleText(s string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "（", "", "）", "", "(", "", ")", "", "-", "", "_", "", ",", "", "，", "", ".", "", "。", "")
	return replacer.Replace(strings.TrimSpace(s))
}
