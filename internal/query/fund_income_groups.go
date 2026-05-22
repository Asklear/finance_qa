package query

import "context"

type fundIncomeTotals struct {
	Settlement                   float64
	Received                     float64
	Invoice                      float64
	UnattributedInvoice          float64
	UnattributedInvoiceContracts []contractDimensionRow
	Receivable                   float64
	InvoiceOpen                  float64
	RowCount                     int
	MonthCount                   int
	ContractCount                int
}

func (e *Engine) collectFundIncomeTotals(ctx context.Context, periodFrom, periodTo, like string) (fundIncomeTotals, error) {
	totals, err := e.collectContractFinanceTotals(ctx, fundIncomeTotalsSpec(), periodFrom, periodTo, like)
	if err != nil {
		return fundIncomeTotals{}, err
	}
	return fundIncomeTotals{
		Settlement:                   totals.Settlement,
		Received:                     totals.Movement,
		Invoice:                      totals.Invoice,
		UnattributedInvoice:          totals.UnattributedInvoice,
		UnattributedInvoiceContracts: totals.UnattributedInvoiceContracts,
		Receivable:                   totals.SettlementOpen,
		InvoiceOpen:                  totals.InvoiceOpen,
		RowCount:                     totals.RowCount,
		MonthCount:                   totals.MonthCount,
		ContractCount:                totals.ContractCount,
	}, nil
}

func (e *Engine) hasFundIncomeGroupTables() bool {
	return e.hasContractFinanceGroupTables(fundIncomeTotalsSpec())
}
