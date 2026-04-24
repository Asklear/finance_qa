package query

import (
	"fmt"
	"strings"
)

func (e *Engine) queryTax(question, from, to string) Result {
	startDate, endDate := from+"-01", monthEndDay(to)
	var output, input float64
	companyClause, companyArgs := e.scopedCompanyClause("company")
	sqlTxt := fmt.Sprintf(`
SELECT
  COALESCE(SUM(CASE
    WHEN account_name LIKE '%%销项%%' OR account_code LIKE '22210106%%'
    THEN COALESCE(credit_amount, 0) ELSE 0 END), 0) AS output_tax,
  COALESCE(SUM(CASE
    WHEN account_name LIKE '%%进项%%' OR account_code LIKE '22210101%%' OR account_code LIKE '222102%%'
    THEN COALESCE(debit_amount, 0) ELSE 0 END), 0) AS input_tax
FROM journal
WHERE %s
  AND voucher_date BETWEEN ? AND ?`, companyClause)
	args := append(companyArgs, startDate, endDate)
	e.db.QueryRow(sqlTxt, args...).Scan(&output, &input)

	logs := []string{
		fmt.Sprintf("[税务审计] 销项税额: %.2f (贷方发生)", output),
		fmt.Sprintf("[税务审计] 进项税额: %.2f (借方发生)", input),
		fmt.Sprintf("[计算结果] 当月净应交: %.2f", output-input),
	}

	msg := fmt.Sprintf("%s 税额查询完成：销项 %.2f 元，进项 %.2f 元", to, output, input)
	if strings.Contains(question, "净税额") {
		msg = fmt.Sprintf("%s 净税额 %.2f 元（销项 %.2f 元 - 进项 %.2f 元）", to, output-input, output, input)
	} else if strings.Contains(question, "销项") && strings.Contains(question, "进项") {
		msg = fmt.Sprintf("%s 销项税额 %.2f 元，进项税额 %.2f 元，净税额 %.2f 元", to, output, input, output-input)
	} else if strings.Contains(question, "进项") {
		msg = fmt.Sprintf("%s 进项税额查询完成：应计 %.2f 元", to, input)
	} else if strings.Contains(question, "销项") {
		msg = fmt.Sprintf("%s 销项税额查询完成：应计 %.2f 元", to, output)
	}

	return Result{
		Success: true,
		Message: msg,
		Data: map[string]any{
			"output":        output,
			"input":         input,
			"total_output":  output,
			"total_input":   input,
			"net_vat":       output - input,
			"source_tables": sourceTablesForTaxQuery(),
		},
		ExecutedSQL: []string{
			fmt.Sprintf("queryTax(aggregated): %s [args: %v]", sqlTxt, args),
		},
		CalculationLogs: logs,
	}
}
