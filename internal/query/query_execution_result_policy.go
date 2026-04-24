package query

import "strings"

func shouldFallbackExecutionResult(result Result) bool {
	if result.Success {
		return false
	}
	if result.Message == "account not found" || containsAmbiguityMessage(result.Message) {
		return true
	}
	return strings.TrimSpace(result.Message) == ""
}
