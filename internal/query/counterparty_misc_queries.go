package query

import "fmt"

func (e *Engine) queryLargeBankTransactions(question, from, to string) Result {
	var name string
	var amount float64
	directionLabel := "流入"
	amountColumn := "credit_amount"
	if containsAny(question, []string{"流出", "支出", "付款", "付出"}) {
		directionLabel = "流出"
		amountColumn = "debit_amount"
	}
	companyClause, companyArgs := e.scopedCompanyClause("company")
	sqlTxt := fmt.Sprintf(`SELECT counterparty_name, %s
FROM bank_statement
WHERE %s
  AND transaction_date BETWEEN ? AND ?
  AND COALESCE(%s, 0) > 0
ORDER BY COALESCE(%s, 0) DESC, transaction_date DESC, counterparty_name
LIMIT 1`, amountColumn, companyClause, amountColumn, amountColumn)
	args := append(companyArgs, from+"-01", monthEndDay(to))
	e.db.QueryRow(sqlTxt, args...).Scan(&name, &amount)
	if name == "" {
		return Result{
			Success: false,
			Message: "未发现大额记录",
			ExecutedSQL: []string{
				fmt.Sprintf("queryLargeBankTransactions: %s [args: %v]", sqlTxt, args),
			},
		}
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 最大%s对手方为 [%s]，流水 %.2f 元", from, directionLabel, name, amount),
		Data: map[string]any{
			"counterparty":  name,
			"amount":        amount,
			"direction":     directionLabel,
			"source_tables": sourceTablesForLargeBankTransaction(),
		},
		ExecutedSQL: []string{
			fmt.Sprintf("queryLargeBankTransactions: %s [args: %v]", sqlTxt, args),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[大额流水] direction=%s top_counterparty=%s amount=%.2f", directionLabel, name, amount),
		},
	}
}

func (e *Engine) detectEntityRole(name string) (role string, log string) {
	endDate := monthEndDay(e.getLatestPeriodAnchor().Format("2006-01"))
	startDate := "2000-01-01"
	evidence := e.collectCounterpartyEvidence(name, startDate[:7], endDate[:7])
	classification := ClassifyCounterparty(name, evidence)
	if classification.Role == CounterpartyUnknown {
		return "unknown", "unknown"
	}
	return string(classification.Role), fmt.Sprintf("role=%s confidence=%.3f signals=%v", classification.Role, classification.Confidence, classification.Signals)
}
