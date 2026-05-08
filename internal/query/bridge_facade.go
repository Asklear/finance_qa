package query

import querybridge "financeqa/internal/query/bridge"

// BuildHostSummaryContract exposes the host-summary compatibility envelope for
// the Go MCP server.
func BuildHostSummaryContract(data map[string]any, question string) map[string]any {
	return querybridge.BuildHostSummaryContract(data, question)
}

func buildHostSummaryContract(data map[string]any, question string) map[string]any {
	return querybridge.BuildHostSummaryContract(data, question)
}

func buildFinalAnswer(r Result) string {
	return querybridge.BuildFinalAnswer(r.Data, r.Message)
}
