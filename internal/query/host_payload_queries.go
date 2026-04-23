package query

import (
	"fmt"
	"strings"
)

func (e *Engine) queryHostLLMPayload(question, from, to string) Result {
	payload := e.buildHostLLMPayload(from, to, question)
	logs := []string{
		fmt.Sprintf("[宿主LLM数据包] company=%s period=%s~%s", e.Company, from, to),
		"[宿主LLM数据包] 已输出全量财报原始数据（按期间过滤）",
	}
	sqls := []string{
		"host_payload(balance_sheet): SELECT * FROM balance_sheet WHERE ... AND period BETWEEN ? AND ?",
		"host_payload(income_statement): SELECT * FROM income_statement WHERE ... AND period BETWEEN ? AND ?",
		"host_payload(balance_detail): SELECT * FROM balance_detail WHERE ... AND period BETWEEN ? AND ?",
		"host_payload(journal): SELECT * FROM journal WHERE ... AND voucher_date BETWEEN ? AND ?",
		"host_payload(bank_statement): SELECT * FROM bank_statement WHERE ... AND transaction_date BETWEEN ? AND ?",
	}
	return Result{
		Success:      true,
		Message:      "已生成宿主LLM可消费的原始财报数据包",
		AnswerMethod: "llm_payload",
		Data: map[string]any{
			"llm_payload": payload,
			"usage":       "请宿主LLM基于 payload.financial_tables 和 payload.trace 进行最终语义判别与回答",
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func (e *Engine) HostLLMPayload(from, to, question string) Result {
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		anchor := e.getLatestPeriodAnchor().Format("2006-01")
		if strings.TrimSpace(from) == "" {
			from = anchor
		}
		if strings.TrimSpace(to) == "" {
			to = anchor
		}
	}
	return e.queryHostLLMPayload(question, from, to).withTraceData()
}

func (e *Engine) buildHostLLMPayload(from, to, question string) map[string]any {
	startDate := from + "-01"
	endDate := monthEndDay(to)

	financialTables := map[string]any{
		"balance_sheet": e.queryRowsAsMaps(`
SELECT company, period, account_code, account_name, opening_balance, closing_balance
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
ORDER BY period, account_code
`, e.Company, e.Company, from, to),
		"income_statement": e.queryRowsAsMaps(`
SELECT company, period, item_name, current_amount, cumulative_amount
FROM income_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
ORDER BY period, item_name
`, e.Company, e.Company, from, to),
		"balance_detail": e.queryRowsAsMaps(`
SELECT company, year, period, opening_period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit
FROM balance_detail
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
ORDER BY year, period, account_code
`, e.Company, e.Company, from, to),
		"journal": e.queryRowsAsMaps(`
SELECT company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
ORDER BY voucher_date, voucher_no
`, e.Company, e.Company, startDate, endDate),
		"bank_statement": e.queryRowsAsMaps(`
SELECT company, transaction_date, transaction_time, transaction_type, debit_amount, credit_amount, balance, summary, counterparty_name, counterparty_account
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
ORDER BY transaction_date, transaction_time
`, e.Company, e.Company, startDate, endDate),
		"fin_contracts": e.queryRowsAsMaps(`
SELECT DISTINCT c.contract_id, c.customer_name, c.contract_content
FROM fin_contracts c
LEFT JOIN fin_cost_settlements cs ON cs.contract_id = c.contract_id AND cs.year_month BETWEEN ? AND ?
LEFT JOIN fin_fund_income f ON f.contract_id = c.contract_id AND f.year_month BETWEEN ? AND ?
WHERE cs.contract_id IS NOT NULL OR f.contract_id IS NOT NULL
ORDER BY c.contract_id
`, from, to, from, to),
		"fin_cost_settlements": e.queryRowsAsMaps(`
SELECT contract_id, year_month, quantity, settlement_amount, is_invoiced, account_code
FROM fin_cost_settlements
WHERE year_month BETWEEN ? AND ?
ORDER BY year_month, contract_id
`, from, to),
		"fin_fund_income": e.queryRowsAsMaps(`
SELECT contract_id, year_month, settlement_amount, received_amount, is_invoiced, invoice_amount
FROM fin_fund_income
WHERE year_month BETWEEN ? AND ?
ORDER BY year_month, contract_id
`, from, to),
	}

	return map[string]any{
		"question": question,
		"company":  e.Company,
		"period": map[string]any{
			"from":       from,
			"to":         to,
			"start_date": startDate,
			"end_date":   endDate,
		},
		"financial_tables": financialTables,
		"trace": map[string]any{
			"intent":   "host_payload_or_fallback",
			"strategy": "sql_extract_then_host_llm_reasoning",
		},
	}
}

func (e *Engine) queryRowsAsMaps(sqlTxt string, args ...any) []map[string]any {
	rows, err := e.db.Query(sqlTxt, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil
	}

	out := make([]map[string]any, 0)
	for rows.Next() {
		raw := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if err := rows.Scan(dest...); err != nil {
			continue
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			v := raw[i]
			if b, ok := v.([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = v
			}
		}
		out = append(out, m)
	}
	return out
}

func (e *Engine) availableAccounts(period string) []string {
	rows, err := e.db.Query(`
SELECT DISTINCT account_name
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ?
ORDER BY account_name
LIMIT 30
`, e.Company, e.Company, period)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]string, 0, 30)
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		if n != "" {
			out = append(out, n)
		}
	}
	if len(out) > 0 {
		return out
	}

	rows2, err := e.db.Query(`
SELECT DISTINCT account_name
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
ORDER BY account_name
LIMIT 30
`, e.Company, e.Company)
	if err != nil {
		return out
	}
	defer rows2.Close()
	for rows2.Next() {
		var n string
		_ = rows2.Scan(&n)
		if n != "" {
			out = append(out, n)
		}
	}
	out = appendUniqueStrings(out, "货币资金", "银行存款", "应收账款", "应付账款", "管理费用", "研发支出", "人工成本", "支出", "费用")
	return out
}

func (e *Engine) counterpartySamples() []string {
	rows, err := e.db.Query(`
SELECT counterparty_name
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND IFNULL(TRIM(counterparty_name),'') <> ''
GROUP BY counterparty_name
ORDER BY SUM(ABS(COALESCE(credit_amount,0)-COALESCE(debit_amount,0))) DESC
LIMIT 10
`, e.Company, e.Company)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]string, 0, 10)
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}
