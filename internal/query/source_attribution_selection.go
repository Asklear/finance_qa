package query

import "strings"

func choosePrimaryAndSupportingTables(spec QuerySpec, data map[string]any, tables []string) ([]string, []string) {
	if len(tables) == 0 {
		return nil, nil
	}
	plan := resolveSourceAttributionPlan(spec, data)
	if len(plan.primaryBaseTables) == 0 {
		return tables, nil
	}
	primary := filterSourceTables(tables, plan.primaryBaseTables...)
	supporting := filterSourceTables(tables, plan.supportingBaseTables...)
	if len(primary) == 0 {
		return tables, nil
	}
	return primary, supporting
}

func filterSourceTables(tables []string, wanted ...string) []string {
	wantSet := map[string]struct{}{}
	for _, item := range wanted {
		wantSet[strings.TrimSpace(item)] = struct{}{}
	}
	out := make([]string, 0, len(tables))
	for _, tableName := range tables {
		base := strings.TrimSpace(baseSourceTableName(tableName))
		if _, ok := wantSet[base]; ok {
			out = append(out, tableName)
		}
	}
	return dedupeSourceTables(out...)
}

func remainingSourceTables(tables []string, used ...string) []string {
	usedSet := map[string]struct{}{}
	for _, item := range used {
		usedSet[strings.TrimSpace(item)] = struct{}{}
	}
	out := make([]string, 0, len(tables))
	for _, tableName := range tables {
		if _, ok := usedSet[strings.TrimSpace(baseSourceTableName(tableName))]; ok {
			continue
		}
		out = append(out, tableName)
	}
	return dedupeSourceTables(out...)
}

func dedupeSourceTables(items ...string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func baseSourceTableName(tableName string) string {
	tableName = strings.TrimSpace(tableName)
	if idx := strings.LastIndex(tableName, "."); idx >= 0 {
		return strings.TrimSpace(tableName[idx+1:])
	}
	return tableName
}
