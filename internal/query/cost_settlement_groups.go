package query

import "context"

type costSettlementTotals struct {
	Settlement    float64
	Paid          float64
	Invoice       float64
	Payable       float64
	InvoiceOpen   float64
	RowCount      int
	MonthCount    int
	ContractCount int
}

func (e *Engine) collectCostSettlementTotals(ctx context.Context, periodFrom, periodTo, like string) (costSettlementTotals, error) {
	totals, err := e.collectContractFinanceTotals(ctx, costSettlementTotalsSpec(), periodFrom, periodTo, like)
	if err != nil {
		return costSettlementTotals{}, err
	}
	return costSettlementTotals{
		Settlement:    totals.Settlement,
		Paid:          totals.Movement,
		Invoice:       totals.Invoice,
		Payable:       totals.SettlementOpen,
		InvoiceOpen:   totals.InvoiceOpen,
		RowCount:      totals.RowCount,
		MonthCount:    totals.MonthCount,
		ContractCount: totals.ContractCount,
	}, nil
}

func (e *Engine) hasCostSettlementGroupTables() bool {
	return e.hasContractFinanceGroupTables(costSettlementTotalsSpec())
}
