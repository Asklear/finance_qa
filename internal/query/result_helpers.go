package query

import "strings"

const journalTaxInclusionNote = "该结果来自序时账汇总，默认按凭证入账金额统计，不主动剔税；若税额未单独拆分，通常应视为含税口径，需结合进销项税/发票分录复核。"

func anyToFloat64(v any) float64 {
	switch typed := v.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	}
	return 0
}

func cloneResult(in Result) Result {
	out := in
	out.Data = cloneAnyMap(in.Data)
	out.ExecutedSQL = append([]string{}, in.ExecutedSQL...)
	out.CalculationLogs = append([]string{}, in.CalculationLogs...)
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneAny(v)
	}
	return out
}

func cloneAny(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []map[string]any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneAnyMap(item))
		}
		return out
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneAny(item))
		}
		return out
	case []FactSet:
		out := make([]FactSet, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneFactSet(item))
		}
		return out
	default:
		return v
	}
}

func cloneFactSet(in FactSet) FactSet {
	out := FactSet{
		Source: in.Source,
		Facts:  make([]Fact, 0, len(in.Facts)),
	}
	for _, fact := range in.Facts {
		factCopy := fact
		factCopy.TracePayload = cloneAnyMap(fact.TracePayload)
		out.Facts = append(out.Facts, factCopy)
	}
	return out
}

func resultUsesInferredOpenItemSettlement(result Result) bool {
	if !result.Success || result.Data == nil {
		return false
	}
	return resultDataUsesInferredOpenItemSettlement(result.Data)
}

func annotateJournalTaxDisclosure(result Result, enabled bool) Result {
	if !enabled {
		return result
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	result.Data["tax_inclusion"] = "journal_entry_gross_amount_default"
	result.Data["tax_inclusion_note"] = journalTaxInclusionNote
	if monthly, ok := result.Data["monthly"].(map[string]any); ok {
		monthly["tax_inclusion"] = "journal_entry_gross_amount_default"
		monthly["tax_inclusion_note"] = journalTaxInclusionNote
	}
	if book, ok := result.Data["财务做账口径(看利润)"].(map[string]any); ok {
		book["税额口径说明"] = journalTaxInclusionNote
	}
	if !strings.Contains(result.Message, "含税口径") {
		result.Message = strings.TrimSpace(result.Message) + " 补充：" + journalTaxInclusionNote
	}
	return result
}
