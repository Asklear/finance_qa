package query

import "strings"

func (e *Engine) enrichStrictContractMissingWithContinuityCandidates(spec QuerySpec, result Result) Result {
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	entity := strings.TrimSpace(spec.Entity)
	if entity == "" || strings.TrimSpace(spec.PeriodFrom) == "" || strings.TrimSpace(spec.PeriodTo) == "" {
		return result
	}
	candidates := e.collectContractContinuityCandidates(entity, spec.PeriodFrom, spec.PeriodTo)
	if len(candidates) == 0 {
		return result
	}
	result.Data["contract_continuity_candidates"] = candidates
	result.Data["contract_continuity_note"] = "原主体当前期间无合同台账记录；以下为历史同名项目在当前期间挂到其他主体名下的候选，供宿主 LLM 按项目连续性判断，不等同于确定主体映射。"
	result.CalculationLogs = append(result.CalculationLogs, "[合同连续性候选] matched same contract_content across historical and requested periods")
	return result
}

func (e *Engine) collectContractContinuityCandidates(entity, from, to string) []map[string]any {
	historical := e.collectHistoricalContractContents(entity, from)
	if len(historical) == 0 {
		return nil
	}

	candidates := make([]map[string]any, 0, len(historical))
	seen := map[string]struct{}{}
	for _, content := range historical {
		rows := e.collectCurrentContractContentRows(entity, content, from, to)
		for _, row := range rows {
			key := normalizeEntityText(anyToString(row["candidate_entity"])) + "|" + normalizeEntityText(anyToString(row["contract_content"]))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, row)
		}
	}
	return candidates
}

func (e *Engine) collectHistoricalContractContents(entity, beforePeriod string) []string {
	rows, err := e.db.Query(`
SELECT DISTINCT c.contract_content
FROM fin_contracts c
JOIN fin_fund_income f ON f.contract_id = c.contract_id
WHERE c.customer_name LIKE ?
  AND COALESCE(TRIM(c.contract_content), '') <> ''
  AND f.year_month < ?
ORDER BY c.contract_content
`, "%"+entity+"%", beforePeriod)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			continue
		}
		content = strings.TrimSpace(content)
		if content != "" {
			out = append(out, content)
		}
	}
	return out
}

func (e *Engine) collectCurrentContractContentRows(originalEntity, content, from, to string) []map[string]any {
	rows, err := e.db.Query(`
SELECT c.customer_name,
       c.contract_content,
       MIN(f.year_month),
       MAX(f.year_month),
       ROUND(COALESCE(SUM(f.settlement_amount), 0), 2),
       ROUND(COALESCE(SUM(f.received_amount), 0), 2),
       ROUND(COALESCE(SUM(f.invoice_amount), 0), 2),
       COUNT(*)
FROM fin_contracts c
JOIN fin_fund_income f ON f.contract_id = c.contract_id
WHERE c.contract_content = ?
  AND c.customer_name NOT LIKE ?
  AND f.year_month BETWEEN ? AND ?
GROUP BY c.customer_name, c.contract_content
ORDER BY 6 DESC, c.customer_name
`, content, "%"+originalEntity+"%", from, to)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var candidateEntity, contractContent, candidateFrom, candidateTo string
		var settlement, received, invoice float64
		var rowCount int
		if err := rows.Scan(&candidateEntity, &contractContent, &candidateFrom, &candidateTo, &settlement, &received, &invoice, &rowCount); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"original_entity":             originalEntity,
			"candidate_entity":            candidateEntity,
			"contract_content":            contractContent,
			"candidate_period_from":       candidateFrom,
			"candidate_period_to":         candidateTo,
			"candidate_settlement_amount": round2(settlement),
			"candidate_received_amount":   round2(received),
			"candidate_invoice_amount":    round2(invoice),
			"candidate_row_count":         rowCount,
			"basis":                       "same_contract_content_across_periods",
			"confidence":                  0.75,
		})
	}
	return out
}
