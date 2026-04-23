package query

import "fmt"

func (e *Engine) collectSupplierContractSummary(summary contractDimensionSummary, like string) (contractDimensionSummary, error) {
	var contractCost, cashPaid float64
	e.db.QueryRow(`
SELECT COALESCE(SUM(cs.settlement_amount), 0)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND cs.year_month BETWEEN ? AND ?
`, like, like, summary.PeriodFrom, summary.PeriodTo).Scan(&contractCost)
	e.db.QueryRow(`
SELECT COALESCE(SUM(debit_amount), 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND counterparty_name LIKE ?
  AND transaction_date BETWEEN ? AND ?
`, e.Company, e.Company, like, summary.PeriodFrom+"-01", monthEndDay(summary.PeriodTo)).Scan(&cashPaid)

	contractCost = round2(contractCost)
	cashPaid = round2(cashPaid)
	summary.Data["book_view"] = map[string]any{
		"contract_cost": contractCost,
		"view":          "contract_ledger",
	}
	summary.Data["cash_view"] = map[string]any{
		"cash_paid_amount": cashPaid,
		"view":             "bank_cash_payment",
	}
	summary.Data["cash_paid_amount"] = cashPaid
	summary.CalculationLog = append(summary.CalculationLog, fmt.Sprintf("[合同维度-供应商] cost=%.2f paid=%.2f", contractCost, cashPaid))
	summary.ExecutedSQL = append(summary.ExecutedSQL,
		"supplier_contract_book: SELECT SUM(settlement_amount) FROM fin_cost_settlements JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?",
		"supplier_contract_cash: SELECT SUM(debit_amount) FROM bank_statement WHERE counterparty_name LIKE ? AND transaction_date BETWEEN ? AND ?",
	)
	return summary, nil
}

func (e *Engine) collectMixedContractSummary(summary contractDimensionSummary, like string) (contractDimensionSummary, error) {
	var revenueSettlement, costSettlement, cashReceived, cashPaid float64
	e.db.QueryRow(`
SELECT COALESCE(SUM(f.settlement_amount), 0)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND f.year_month BETWEEN ? AND ?
`, like, like, summary.PeriodFrom, summary.PeriodTo).Scan(&revenueSettlement)
	e.db.QueryRow(`
SELECT COALESCE(SUM(cs.settlement_amount), 0)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND cs.year_month BETWEEN ? AND ?
`, like, like, summary.PeriodFrom, summary.PeriodTo).Scan(&costSettlement)
	e.db.QueryRow(`
SELECT COALESCE(SUM(f.received_amount), 0)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND f.year_month BETWEEN ? AND ?
`, like, like, summary.PeriodFrom, summary.PeriodTo).Scan(&cashReceived)
	e.db.QueryRow(`
SELECT COALESCE(SUM(debit_amount), 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND counterparty_name LIKE ?
  AND transaction_date BETWEEN ? AND ?
`, e.Company, e.Company, like, summary.PeriodFrom+"-01", monthEndDay(summary.PeriodTo)).Scan(&cashPaid)

	revenueSettlement = round2(revenueSettlement)
	costSettlement = round2(costSettlement)
	cashReceived = round2(cashReceived)
	cashPaid = round2(cashPaid)
	summary.Data["book_view"] = map[string]any{
		"revenue_settlement": revenueSettlement,
		"cost_settlement":    costSettlement,
		"view":               "contract_ledger",
	}
	summary.Data["cash_view"] = map[string]any{
		"received_amount":  cashReceived,
		"cash_paid_amount": cashPaid,
		"view":             "bank_cash_flow",
	}
	summary.Data["cash_paid_amount"] = cashPaid
	summary.CalculationLog = append(summary.CalculationLog, fmt.Sprintf("[合同维度-混合] revenue=%.2f cost=%.2f received=%.2f paid=%.2f", revenueSettlement, costSettlement, cashReceived, cashPaid))
	summary.ExecutedSQL = append(summary.ExecutedSQL,
		"mixed_contract_book: SELECT SUM(settlement_amount) FROM fin_fund_income/fin_cost_settlements JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?",
		"mixed_contract_cash: SELECT SUM(received_amount) FROM fin_fund_income JOIN fin_contracts ...; SELECT SUM(debit_amount) FROM bank_statement ...",
	)
	return summary, nil
}
