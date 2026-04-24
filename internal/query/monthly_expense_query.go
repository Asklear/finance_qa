package query

import "fmt"

func (e *Engine) queryMonthlyExpenseFromBank(from, to string) Result {
	var total float64
	sqlTxt := `SELECT COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND transaction_date BETWEEN ? AND ?`
	e.db.QueryRow(sqlTxt, e.Company, e.Company, from+"-01", monthEndDay(to)).Scan(&total)
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 整体支出 %.2f 元（按银行卡实际支出统计）", displayPeriod(from, to), total),
		Data: map[string]any{
			"period": displayPeriod(from, to),
			"total":  total,
		},
		ExecutedSQL:     []string{fmt.Sprintf("queryMonthlyExpenseFromBank: %s [args: %s, %s, %s]", sqlTxt, e.Company, from+"-01", monthEndDay(to))},
		CalculationLogs: []string{fmt.Sprintf("[整体支出] period=%s~%s bank_out=%.2f", from, to, total)},
	}
}
