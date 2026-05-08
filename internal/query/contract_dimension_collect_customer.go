package query

import (
	"context"
	"fmt"
)

func (e *Engine) collectCustomerContractSummary(summary contractDimensionSummary, like string, hasSubPeriod bool, subPeriod string) (contractDimensionSummary, error) {
	totals, err := e.collectFundIncomeTotals(context.Background(), summary.PeriodFrom, summary.PeriodTo, like)
	if err != nil {
		return contractDimensionSummary{}, err
	}

	settlementAmount := round2(totals.Settlement)
	invoiceAmount := round2(totals.Invoice)
	cashReceived := round2(totals.Received)
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
		subTotals, err := e.collectFundIncomeTotals(context.Background(), subPeriod, subPeriod, like)
		if err != nil {
			return contractDimensionSummary{}, err
		}
		subReceipts := round2(subTotals.Received)
		summary.SubPeriod = subPeriod
		summary.Data["sub_period"] = subPeriod
		summary.Data["sub_period_receipts"] = subReceipts
		summary.CalculationLog = append(summary.CalculationLog, fmt.Sprintf("[合同维度-客户] sub_period=%s receipts=%.2f", subPeriod, subReceipts))
		summary.ExecutedSQL = append(summary.ExecutedSQL, "customer_contract_subperiod_cash: SELECT SUM(received_amount) FROM fin_fund_income + fin_fund_income_groups ... WHERE year_month = ?")
	}
	summary.CalculationLog = append(summary.CalculationLog, fmt.Sprintf("[合同维度-客户] settlement=%.2f invoice=%.2f received=%.2f", settlementAmount, invoiceAmount, cashReceived))
	summary.ExecutedSQL = append(summary.ExecutedSQL,
		"customer_contract_book: SELECT SUM(settlement_amount), SUM(invoice_amount) FROM fin_fund_income + fin_fund_income_groups ... WHERE year_month BETWEEN ? AND ?",
		"customer_contract_cash: SELECT SUM(received_amount) FROM fin_fund_income + fin_fund_income_groups ... WHERE year_month BETWEEN ? AND ?",
	)
	return summary, nil
}
