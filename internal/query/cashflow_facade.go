package query

import querycashflow "financeqa/internal/query/cashflow"

type CashDirection = querycashflow.CashDirection

const (
	CashDirectionUnknown = querycashflow.CashDirectionUnknown
	CashDirectionInflow  = querycashflow.CashDirectionInflow
	CashDirectionOutflow = querycashflow.CashDirectionOutflow
)

type CashFlowCounterpartyStat = querycashflow.CashFlowCounterpartyStat
type CashFlowDirectionSummary = querycashflow.CashFlowDirectionSummary

func IsBankCashAccount(accountCode string) bool {
	return querycashflow.IsBankCashAccount(accountCode)
}

func BankCashDirection(accountCode, accountingDirection string) CashDirection {
	return querycashflow.BankCashDirection(accountCode, accountingDirection)
}

func NewCashFlowCounterpartyStat(name string, outflow, inflow float64) CashFlowCounterpartyStat {
	return querycashflow.NewCashFlowCounterpartyStat(name, outflow, inflow)
}

func BuildCashFlowDirectionSummary(rows []CashFlowCounterpartyStat) CashFlowDirectionSummary {
	return querycashflow.BuildCashFlowDirectionSummary(rows)
}
