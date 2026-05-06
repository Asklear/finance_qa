package query

import (
	"fmt"
	"strings"
)

func (e *Engine) queryPrecise(question, period string) Result {
	if shouldUsePreciseBankDepositBalanceQuestion(question) {
		return e.queryBankDepositPreciseBalance(question, period)
	}
	accountNames, err := e.findMatchingAccounts(question, period)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	if len(accountNames) == 0 {
		return Result{Success: false, Message: "account not found"}
	}
	if len(accountNames) == 1 {
		return e.querySinglePreciseBalance(accountNames[0], period)
	}
	return e.queryMultiplePreciseBalances(accountNames, period)
}

func (e *Engine) queryBankDepositPreciseBalance(question, period string) Result {
	balance, ok := e.queryBalanceDetailClosing(period, "1002")
	if !ok {
		return e.querySinglePreciseBalance("货币资金", period)
	}
	data := map[string]any{
		"period":                period,
		"account":               "银行存款",
		"closing":               balance,
		"closing_balance":       balance,
		"source_tables":         []string{"fin_balance_detail"},
		"source_primary_tables": []string{"fin_balance_detail"},
	}
	if strings.Contains(question, "货币资金") {
		data["货币资金_closing_balance"] = balance
	}
	if strings.Contains(question, "银行存款") {
		data["银行存款_closing_balance"] = balance
	}
	accountLabel := "银行存款"
	if strings.Contains(question, "货币资金") && strings.Contains(question, "银行存款") {
		accountLabel = "货币资金/银行存款"
	}
	return Result{
		Success:      true,
		Message:      fmt.Sprintf("%s %s期末余额 %.2f 元", period, accountLabel, balance),
		AnswerMethod: "sql",
		Data:         data,
		ExecutedSQL: []string{
			"SELECT closing_debit, closing_credit FROM balance_detail WHERE ... AND account_code = '1002'",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[余额对账] 科目:银行存款(1002), 期间:%s", period),
		},
	}
}

func (e *Engine) queryBalanceDetailClosing(period, accountCode string) (float64, bool) {
	var debit, credit float64
	err := e.db.QueryRow(`
SELECT COALESCE(SUM(closing_debit), 0), COALESCE(SUM(closing_credit), 0)
FROM balance_detail
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND account_code = ?`, e.Company, e.Company, period, accountCode).Scan(&debit, &credit)
	if err != nil {
		return 0, false
	}
	if debit == 0 && credit == 0 {
		return 0, false
	}
	return round2(debit - credit), true
}

func shouldUsePreciseBankDepositBalanceQuestion(question string) bool {
	q := NormalizeQuestion(question)
	if !shouldUsePreciseBalanceQuestion(q) {
		return false
	}
	return strings.Contains(q, "银行存款") || (strings.Contains(q, "货币资金") && strings.Contains(q, "银行"))
}

func (e *Engine) querySinglePreciseBalance(accountName, period string) Result {
	startDate, endDate := period+"-01", monthEndDay(period)
	var opening, closing, debit, credit float64
	e.db.QueryRow(`SELECT opening_balance, closing_balance FROM balance_sheet WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ? AND account_name = ?`, e.Company, e.Company, period, accountName).Scan(&opening, &closing)
	e.db.QueryRow(`SELECT SUM(debit_amount), SUM(credit_amount) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND voucher_date BETWEEN ? AND ? AND account_name = ?`, e.Company, e.Company, startDate, endDate, accountName).Scan(&debit, &credit)

	logs := []string{
		fmt.Sprintf("[余额对账] 科目:%s, 期间:%s", accountName, period),
		fmt.Sprintf("[轧账公式] 期初余:%.2f + 借方发生:%.2f - 贷方发生:%.2f = 期末余:%.2f", opening, debit, credit, closing),
	}

	bsSQL := fmt.Sprintf(`SELECT opening_balance, closing_balance FROM balance_sheet WHERE ... AND account_name = '%s'`, accountName)
	jrSQL := fmt.Sprintf(`SELECT SUM(debit_amount), SUM(credit_amount) FROM journal WHERE ... AND account_name = '%s'`, accountName)

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 综合账务余额为 %.2f 元", period, accountName, closing),
		Data: map[string]any{
			"period":          period,
			"account":         accountName,
			"opening":         opening,
			"closing":         closing,
			"debit":           debit,
			"credit":          credit,
			"opening_balance": opening,
			"closing_balance": closing,
			"debit_amount":    debit,
			"credit_amount":   credit,
			"source_tables":   sourceTablesForPreciseBalance(),
		},
		ExecutedSQL:     []string{bsSQL, jrSQL},
		CalculationLogs: logs,
	}
}

func (e *Engine) queryMultiplePreciseBalances(accountNames []string, period string) Result {
	details := make([]map[string]any, 0, len(accountNames))
	parts := make([]string, 0, len(accountNames))
	sourceTables := sourceTablesForPreciseBalance()
	sqls := []string{}
	logs := []string{}
	data := map[string]any{
		"period":        period,
		"details":       details,
		"source_tables": sourceTables,
	}
	for _, accountName := range accountNames {
		item := e.querySinglePreciseBalance(accountName, period)
		if !item.Success {
			continue
		}
		opening := anyToFloat64(item.Data["opening_balance"])
		closing := anyToFloat64(item.Data["closing_balance"])
		debit := anyToFloat64(item.Data["debit_amount"])
		credit := anyToFloat64(item.Data["credit_amount"])
		details = append(details, map[string]any{
			"account":         accountName,
			"opening_balance": opening,
			"closing_balance": closing,
			"debit_amount":    debit,
			"credit_amount":   credit,
		})
		data[accountName+"_opening_balance"] = opening
		data[accountName+"_closing_balance"] = closing
		parts = append(parts, fmt.Sprintf("%s %.2f 元", accountName, closing))
		sqls = append(sqls, item.ExecutedSQL...)
		logs = append(logs, item.CalculationLogs...)
	}
	if len(details) == 0 {
		return Result{Success: false, Message: "account not found"}
	}
	data["details"] = details
	return Result{
		Success:         true,
		Message:         fmt.Sprintf("%s %s。", period, strings.Join(parts, "，")),
		Data:            data,
		ExecutedSQL:     dedupeStrings(sqls),
		CalculationLogs: dedupeStrings(logs),
	}
}

func (e *Engine) findMatchingAccount(question, period string) (string, error) {
	accounts, err := e.findMatchingAccounts(question, period)
	if err != nil {
		return "", err
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("account not found")
	}
	return accounts[0], nil
}

func (e *Engine) findMatchingAccounts(question, period string) ([]string, error) {
	wants := preciseBalanceAccountCandidates(question)
	rows, _ := e.db.Query(`SELECT DISTINCT account_name FROM balance_sheet WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ?`, e.Company, e.Company, period)
	matches := make([]string, 0, len(wants))
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var n string
			rows.Scan(&n)
			for _, want := range wants {
				if n == want || strings.Contains(question, n) {
					matches = append(matches, n)
					break
				}
			}
		}
	}
	matches = dedupeStrings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("account not found")
	}
	return matches, nil
}

func preciseBalanceAccountCandidates(question string) []string {
	candidates := make([]string, 0, 2)
	if strings.Contains(question, "货币资金") {
		candidates = append(candidates, "货币资金")
	}
	if strings.Contains(question, "银行存款") {
		candidates = append(candidates, "银行存款")
	}
	if len(candidates) == 0 {
		candidates = append(candidates, strings.TrimSpace(question))
	}
	return dedupeStrings(candidates)
}
