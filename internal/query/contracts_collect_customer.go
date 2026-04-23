package query

import "fmt"

func (e *Engine) collectCustomerContractSummary(summary contractDimensionSummary, like string, hasSubPeriod bool, subPeriod string) (contractDimensionSummary, error) {
	var settlementAmount, invoiceAmount, cashReceived float64
	e.db.QueryRow(`
SELECT COALESCE(SUM(f.settlement_amount), 0), COALESCE(SUM(f.invoice_amount), 0)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND f.year_month BETWEEN ? AND ?
`, like, like, summary.PeriodFrom, summary.PeriodTo).Scan(&settlementAmount, &invoiceAmount)
	e.db.QueryRow(`
SELECT COALESCE(SUM(f.received_amount), 0)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND f.year_month BETWEEN ? AND ?
`, like, like, summary.PeriodFrom, summary.PeriodTo).Scan(&cashReceived)

	settlementAmount = round2(settlementAmount)
	invoiceAmount = round2(invoiceAmount)
	cashReceived = round2(cashReceived)
	summary.Data["book_view"] = map[string]any{
		"settlement_amount": settlementAmount,
		"invoice_amount":    invoiceAmount,
		"view":              "contract_ledger",
	}
	summary.Data["cash_view"] = map[string]any{
		"received_amount": cashReceived,
		"view":            "bank_cash_collection",
	}
	if hasSubPeriod {
		var subReceipts float64
		e.db.QueryRow(`
SELECT COALESCE(SUM(f.received_amount), 0)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND f.year_month = ?
`, like, like, subPeriod).Scan(&subReceipts)
		subReceipts = round2(subReceipts)
		summary.SubPeriod = subPeriod
		summary.Data["sub_period"] = subPeriod
		summary.Data["sub_period_receipts"] = subReceipts
		summary.CalculationLog = append(summary.CalculationLog, fmt.Sprintf("[合同维度-客户] sub_period=%s receipts=%.2f", subPeriod, subReceipts))
		summary.ExecutedSQL = append(summary.ExecutedSQL, "customer_contract_subperiod_cash: SELECT SUM(received_amount) FROM fin_fund_income JOIN fin_contracts ... WHERE year_month = ?")
	}
	summary.CalculationLog = append(summary.CalculationLog, fmt.Sprintf("[合同维度-客户] settlement=%.2f invoice=%.2f received=%.2f", settlementAmount, invoiceAmount, cashReceived))
	summary.ExecutedSQL = append(summary.ExecutedSQL,
		"customer_contract_book: SELECT SUM(settlement_amount), SUM(invoice_amount) FROM fin_fund_income JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?",
		"customer_contract_cash: SELECT SUM(received_amount) FROM fin_fund_income JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?",
	)
	return summary, nil
}
