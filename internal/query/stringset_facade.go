package query

import querystringset "financeqa/internal/query/stringset"

func appendUniqueStrings(base []string, values ...string) []string {
	return querystringset.AppendUnique(base, values...)
}

func containsString(items []string, want string) bool {
	return querystringset.Contains(items, want)
}

func firstMetricOrDefault(items []string, fallback string) string {
	return querystringset.FirstOrDefault(items, fallback)
}
