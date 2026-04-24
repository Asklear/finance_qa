package query

import "strings"

func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		base = append(base, v)
		seen[v] = true
	}
	return base
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == strings.TrimSpace(want) {
			return true
		}
	}
	return false
}

func firstMetricOrDefault(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	if strings.TrimSpace(items[0]) == "" {
		return fallback
	}
	return items[0]
}
