package query

import "fmt"

func (e *Engine) buildAmbiguousFallbackResult(q, from, to string) Result {
	accounts := e.availableAccounts(to)
	samples := e.counterpartySamples()
	entity := e.extractNamedEntity(q)
	logs := []string{fmt.Sprintf("[识别] fallback实体识别结果: %s", entity)}
	payload := e.buildHostLLMPayload(from, to, q)
	return Result{
		Success:      false,
		Message:      "指令语义模糊",
		AnswerMethod: "llm_payload",
		Data: map[string]any{
			"fallback_attempted":  true,
			"hint":                "请给出更具体的问题，例如“某年某月应收账款多少”或“某客户某月回款多少”",
			"available_accounts":  accounts,
			"counterparty_sample": samples,
			"llm_payload":         payload,
		},
		CalculationLogs: logs,
	}
}
