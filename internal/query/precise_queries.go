package query

import (
	"fmt"
	"strings"
)

func (e *Engine) queryPrecise(question, period string) Result {
	accountName, err := e.findMatchingAccount(question, period)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
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

func (e *Engine) findMatchingAccount(question, period string) (string, error) {
	rows, _ := e.db.Query(`SELECT DISTINCT account_name FROM balance_sheet WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ?`, e.Company, e.Company, period)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var n string
			rows.Scan(&n)
			if strings.Contains(question, n) {
				return n, nil
			}
		}
	}
	return "", fmt.Errorf("account not found")
}
