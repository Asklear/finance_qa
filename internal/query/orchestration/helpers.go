package orchestration

import queryfact "financeqa/internal/query/fact"

func FindFactSetBySource(factSets []queryfact.FactSet, source string) (queryfact.FactSet, bool) {
	for _, factSet := range factSets {
		if factSet.Source == source {
			return factSet, true
		}
	}
	return queryfact.FactSet{}, false
}

func FindFactValue(factSet queryfact.FactSet, metricKey string) (float64, bool) {
	for _, fact := range factSet.Facts {
		if fact.MetricKey == metricKey {
			return fact.Value, true
		}
	}
	return 0, false
}

func AnyToString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func AnyToMapSlice(v any) []map[string]any {
	if rows, ok := v.([]map[string]any); ok {
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			out = append(out, CloneMap(row))
		}
		return out
	}
	if rows, ok := v.([]any); ok {
		out := make([]map[string]any, 0, len(rows))
		for _, raw := range rows {
			if row, ok := raw.(map[string]any); ok {
				out = append(out, CloneMap(row))
			}
		}
		return out
	}
	return nil
}

func CloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func ExtractFrameTraceStrings(factSets []queryfact.FactSet, key string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	for _, factSet := range factSets {
		for _, fact := range factSet.Facts {
			values := anyToStringSlice(fact.TracePayload[key])
			for _, item := range values {
				if _, ok := seen[item]; ok {
					continue
				}
				seen[item] = struct{}{}
				out = append(out, item)
			}
		}
	}
	return out
}

func anyToStringSlice(v any) []string {
	if values, ok := v.([]string); ok {
		return append([]string{}, values...)
	}
	if values, ok := v.([]any); ok {
		out := make([]string, 0, len(values))
		for _, raw := range values {
			if s, ok := raw.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func SourceCapabilitiesToStrings[T ~string](capabilities []T) []string {
	out := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, string(capability))
	}
	return out
}

func JoinWithComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += "，" + parts[i]
	}
	return out
}
