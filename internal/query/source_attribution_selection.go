package query

import "strings"

func choosePrimaryAndSupportingTables(spec QuerySpec, data map[string]any, tables []string) ([]string, []string) {
	if len(tables) == 0 {
		return nil, nil
	}

	switch spec.QueryFamily {
	case QueryFamilyContractDimension:
		role := strings.TrimSpace(anyToString(data["role"]))
		askedTopic := strings.TrimSpace(anyToString(data["asked_topic"]))
		switch askedTopic {
		case "content":
			return filterSourceTables(tables, "fin_contracts"), remainingSourceTables(tables, "fin_contracts")
		case "revenue", "receipts":
			return filterSourceTables(tables, "fin_fund_income"), remainingSourceTables(tables, "fin_fund_income")
		case "cost", "payments":
			return filterSourceTables(tables, "fin_cost_settlements"), remainingSourceTables(tables, "fin_cost_settlements")
		case "profit":
			if role == "mixed_contract" {
				return filterSourceTables(tables, "fin_fund_income", "fin_cost_settlements"), remainingSourceTables(tables, "fin_fund_income", "fin_cost_settlements")
			}
			if role == "customer_contract" {
				return filterSourceTables(tables, "fin_fund_income"), remainingSourceTables(tables, "fin_fund_income")
			}
			return filterSourceTables(tables, "fin_cost_settlements"), remainingSourceTables(tables, "fin_cost_settlements")
		default:
			return tables, nil
		}
	case QueryFamilyCoreMetric:
		if strings.TrimSpace(anyToString(data["source_priority"])) == "contract_first" {
			metric := detectSourceMetric(spec, data)
			primary := contractAggregateTablesForMetric(metric)
			return filterSourceTables(tables, primary...), remainingSourceTables(tables, primary...)
		}
		accrualSource := detectAccrualSource(data)
		if strings.Contains(accrualSource, "journal") {
			return filterSourceTables(tables, "fin_journal"), remainingSourceTables(tables, "fin_journal")
		}
		return filterSourceTables(tables, "fin_income_statement"), remainingSourceTables(tables, "fin_income_statement")
	case QueryFamilyARAP:
		if strings.Contains(strings.TrimSpace(anyToString(data["source"])), "journal") {
			return filterSourceTables(tables, "fin_journal"), remainingSourceTables(tables, "fin_journal")
		}
		return filterSourceTables(tables, "fin_balance_detail"), remainingSourceTables(tables, "fin_balance_detail")
	case QueryFamilySupplierPayments:
		return filterSourceTables(tables, "fin_bank_statement"), remainingSourceTables(tables, "fin_bank_statement")
	case QueryFamilyHRCost:
		return filterSourceTables(tables, "fin_journal"), remainingSourceTables(tables, "fin_journal")
	default:
		return tables, nil
	}
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
