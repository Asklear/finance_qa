package query

import "strings"

func (e *Engine) collectSourceTables(spec QuerySpec, data map[string]any) []string {
	return resolveSourceAttributionPlan(spec, data).tables
}

func contractSourceTablesFromData(data map[string]any) []string {
	role := strings.TrimSpace(anyToString(data["role"]))
	askedTopic := strings.TrimSpace(anyToString(data["asked_topic"]))
	switch askedTopic {
	case "content":
		return []string{"fin_contracts"}
	case "revenue", "receipts":
		return []string{"fin_contracts", "fin_fund_income"}
	case "cost", "payments":
		if role == "supplier_contract" || role == "mixed_contract" {
			return []string{"fin_contracts", "fin_cost_settlements", "fin_bank_statement"}
		}
		return []string{"fin_contracts", "fin_cost_settlements"}
	case "profit":
		if role == "mixed_contract" {
			return []string{"fin_contracts", "fin_fund_income", "fin_cost_settlements", "fin_bank_statement"}
		}
		if role == "supplier_contract" {
			return []string{"fin_contracts", "fin_cost_settlements", "fin_bank_statement"}
		}
		return []string{"fin_contracts", "fin_fund_income"}
	default:
		if role == "supplier_contract" {
			return []string{"fin_contracts", "fin_cost_settlements", "fin_bank_statement"}
		}
		if role == "mixed_contract" {
			return []string{"fin_contracts", "fin_fund_income", "fin_cost_settlements", "fin_bank_statement"}
		}
		return []string{"fin_contracts", "fin_fund_income"}
	}
}

func contractAggregateTablesForMetric(metric string) []string {
	switch strings.TrimSpace(metric) {
	case "成本":
		return []string{"fin_cost_settlements"}
	case "利润":
		return []string{"fin_fund_income", "fin_cost_settlements"}
	default:
		return []string{"fin_fund_income"}
	}
}

func contractAggregateTablesForRequestedMetrics(spec QuerySpec, data map[string]any) []string {
	requested := anySourceStringSlice(data["requested_metrics"])
	if len(requested) == 0 {
		return contractAggregateTablesForMetric(detectSourceMetric(spec, data))
	}
	tables := make([]string, 0, 2)
	if contractAggregateNeedsRevenueData(requested) {
		tables = append(tables, "fin_fund_income")
	}
	if contractAggregateNeedsCostData(requested) {
		tables = append(tables, "fin_cost_settlements")
	}
	if len(tables) == 0 {
		return contractAggregateTablesForMetric(detectSourceMetric(spec, data))
	}
	return dedupeSourceTables(tables...)
}

func detectSourceMetric(spec QuerySpec, data map[string]any) string {
	if metric := strings.TrimSpace(anyToString(data["metric"])); metric != "" && metric != "核心指标" {
		return metric
	}
	switch spec.MetricKind {
	case MetricKindCost:
		return "成本"
	case MetricKindProfit:
		return "利润"
	default:
		return "收入"
	}
}

func detectAccrualSource(data map[string]any) string {
	if monthly, ok := data["monthly"].(map[string]any); ok {
		if source := strings.TrimSpace(anyToString(monthly["source"])); source != "" {
			return source
		}
	}
	if summary, ok := data["range_summary"].(map[string]any); ok {
		if source := strings.TrimSpace(anyToString(summary["source"])); source != "" {
			return source
		}
	}
	return strings.TrimSpace(anyToString(data["source"]))
}

func hasCashPerspective(data map[string]any) bool {
	if _, ok := data["money_view"]; ok {
		return true
	}
	if _, ok := data["cash_view"]; ok {
		return true
	}
	if _, ok := data["cash_flow"]; ok {
		return true
	}
	return false
}

func anySourceStringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return dedupeSourceTables(typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(anyToString(item))
			if text == "" {
				continue
			}
			out = append(out, text)
		}
		return dedupeSourceTables(out...)
	default:
		return nil
	}
}
