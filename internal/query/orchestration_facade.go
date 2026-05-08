package query

import queryorchestration "financeqa/internal/query/orchestration"

func findFactSetBySource(factSets []FactSet, source string) (FactSet, bool) {
	return queryorchestration.FindFactSetBySource(factSets, source)
}

func findFactValue(factSet FactSet, metricKey string) (float64, bool) {
	return queryorchestration.FindFactValue(factSet, metricKey)
}

func anyToString(v any) string {
	return queryorchestration.AnyToString(v)
}

func anyToMapSlice(v any) []map[string]any {
	return queryorchestration.AnyToMapSlice(v)
}

func cloneMap(in map[string]any) map[string]any {
	return queryorchestration.CloneMap(in)
}

func extractFrameTraceStrings(factSets []FactSet, key string) []string {
	return queryorchestration.ExtractFrameTraceStrings(factSets, key)
}

func sourceCapabilitiesToStrings(capabilities []SourceCapability) []string {
	return queryorchestration.SourceCapabilitiesToStrings(capabilities)
}

func joinWithComma(parts []string) string {
	return queryorchestration.JoinWithComma(parts)
}
