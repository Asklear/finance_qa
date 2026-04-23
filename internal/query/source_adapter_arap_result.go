package query

import "strings"

func buildARAPFactSetFromQueryResult(spec QuerySpec, result Result) (FactSet, bool) {
	if result.Data == nil {
		return FactSet{}, false
	}

	facts := make([]Fact, 0, 8)
	if receivable, ok := mapFromAny(result.Data["receivable"]); ok {
		scope := arapScope{
			typ:         "receivable",
			accountName: "应收账款",
			codePrefix:  "1122",
			metricLabel: "arap",
		}
		facts = append(facts, buildARAPFactsFromData(spec, scope, receivable, anyToString(receivable["source"]) == "balance_sheet")...)
		if openData, ok := mapFromAny(receivable["open_item_analysis"]); ok {
			facts = append(facts, buildARAPOpenItemFacts(spec, scope, openData)...)
		}
	}
	if payable, ok := mapFromAny(result.Data["payable"]); ok {
		scope := arapScope{
			typ:         "payable",
			accountName: "应付账款",
			codePrefix:  "2202",
			metricLabel: "arap",
		}
		facts = append(facts, buildARAPFactsFromData(spec, scope, payable, anyToString(payable["source"]) == "balance_sheet")...)
		if openData, ok := mapFromAny(payable["open_item_analysis"]); ok {
			facts = append(facts, buildARAPOpenItemFacts(spec, scope, openData)...)
		}
	}
	if len(facts) > 0 {
		return FactSet{Source: "arap", Facts: facts}, true
	}

	scopes := detectARAPScopes(spec)
	if len(scopes) == 0 {
		return FactSet{}, false
	}
	scope := scopes[0]
	if len(scopes) > 1 {
		if inferred, ok := inferARAPScopeFromResult(result.Data); ok {
			scope = inferred
		} else {
			return FactSet{}, false
		}
	}
	facts = append(facts, buildARAPFactsFromData(spec, scope, result.Data, anyToString(result.Data["source"]) == "balance_sheet")...)
	if openData, ok := mapFromAny(result.Data["open_item_analysis"]); ok {
		facts = append(facts, buildARAPOpenItemFacts(spec, scope, openData)...)
	}
	if len(facts) == 0 {
		return FactSet{}, false
	}
	return FactSet{Source: "arap", Facts: facts}, true
}

func inferARAPScopeFromResult(data map[string]any) (arapScope, bool) {
	accountName := anyToString(data["account"])
	switch {
	case strings.Contains(accountName, "应收"):
		return arapScope{
			typ:         "receivable",
			accountName: "应收账款",
			codePrefix:  "1122",
			metricLabel: "arap",
		}, true
	case strings.Contains(accountName, "应付"):
		return arapScope{
			typ:         "payable",
			accountName: "应付账款",
			codePrefix:  "2202",
			metricLabel: "arap",
		}, true
	}
	if strings.Contains(anyToString(data["type"]), "receivable") {
		return arapScope{
			typ:         "receivable",
			accountName: "应收账款",
			codePrefix:  "1122",
			metricLabel: "arap",
		}, true
	}
	if strings.Contains(anyToString(data["type"]), "payable") {
		return arapScope{
			typ:         "payable",
			accountName: "应付账款",
			codePrefix:  "2202",
			metricLabel: "arap",
		}, true
	}
	return arapScope{}, false
}

func mapFromAny(v any) (map[string]any, bool) {
	data, ok := v.(map[string]any)
	return data, ok
}
