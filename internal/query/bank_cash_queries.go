package query

import "fmt"

func (e *Engine) queryBankCashFlow(question, from, to string) Result {
	e.calc.ResetTrace()
	cash, err := e.calc.ComputeCashFlow(e.Company, from, to)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}

	q := NormalizeQuestion(question)
	periodLabel := displayPeriod(from, to)
	message := fmt.Sprintf("%s 银行卡实际到账 %.2f 元，实际支出 %.2f 元，净增加 %.2f 元。", periodLabel, cash.Income, cash.Expense, cash.Net)
	switch {
	case containsAny(q, []string{"实际支出", "支出", "付款", "支付"}):
		message = fmt.Sprintf("%s 银行卡实际支出 %.2f 元。", periodLabel, cash.Expense)
	case containsAny(q, []string{"净增加", "净流入", "净流出", "净现金流"}):
		message = fmt.Sprintf("%s 银行卡净增加 %.2f 元（实际到账 %.2f 元，实际支出 %.2f 元）。", periodLabel, cash.Net, cash.Income, cash.Expense)
	case containsAny(q, []string{"实际到账", "到账", "回款", "收款"}):
		message = fmt.Sprintf("%s 银行卡实际到账 %.2f 元。", periodLabel, cash.Income)
	}

	return Result{
		Success:      true,
		Message:      message,
		AnswerMethod: "sql",
		Data: map[string]any{
			"period":                   periodLabel,
			"period_from":              from,
			"period_to":                to,
			"cash_flow":                buildCoreMetricCashFlowSummary(cash),
			"cash_view":                buildCoreMetricCashFlowSummary(cash),
			"source_primary_tables":    []string{"fin_bank_statement"},
			"source_supporting_tables": []string{},
		},
		ExecutedSQL:     append([]string{}, e.calc.ExecutedSQLs...),
		CalculationLogs: append([]string{}, e.calc.CalculationLogs...),
	}
}
