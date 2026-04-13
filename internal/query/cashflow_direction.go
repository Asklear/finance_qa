package query

import (
	"math"
	"strings"
)

// CashDirection describes the direction of cash movement for a transaction.
type CashDirection string

const (
	CashDirectionUnknown CashDirection = "unknown"
	CashDirectionInflow  CashDirection = "inflow"
	CashDirectionOutflow CashDirection = "outflow"
)

// IsBankCashAccount returns true for bank/cash accounts that should use the
// cash-flow direction rule. We currently cover 1001* and 1002*.
func IsBankCashAccount(accountCode string) bool {
	accountCode = strings.TrimSpace(accountCode)
	return strings.HasPrefix(accountCode, "1001") || strings.HasPrefix(accountCode, "1002")
}

// BankCashDirection maps accounting direction to cash-flow direction for bank
// and cash accounts. For 1001*/1002*, debit means inflow and credit means outflow.
func BankCashDirection(accountCode, accountingDirection string) CashDirection {
	if !IsBankCashAccount(accountCode) {
		return CashDirectionUnknown
	}

	switch strings.TrimSpace(accountingDirection) {
	case "借", "debit", "DEBIT":
		return CashDirectionInflow
	case "贷", "credit", "CREDIT":
		return CashDirectionOutflow
	default:
		return CashDirectionUnknown
	}
}

// CashFlowCounterpartyStat is a reusable direction-aware summary for one counterparty.
type CashFlowCounterpartyStat struct {
	Name      string        `json:"name"`
	Outflow   float64       `json:"outflow"`
	Inflow    float64       `json:"inflow"`
	Net       float64       `json:"net"`
	Direction CashDirection `json:"direction"`
}

// NewCashFlowCounterpartyStat creates a reusable counterparty summary from
// outflow/inflow totals.
func NewCashFlowCounterpartyStat(name string, outflow, inflow float64) CashFlowCounterpartyStat {
	stat := CashFlowCounterpartyStat{
		Name:    name,
		Outflow: roundCashFloat(outflow),
		Inflow:  roundCashFloat(inflow),
		Net:     roundCashFloat(inflow - outflow),
	}

	switch {
	case stat.Outflow > stat.Inflow:
		stat.Direction = CashDirectionOutflow
	case stat.Inflow > stat.Outflow:
		stat.Direction = CashDirectionInflow
	default:
		stat.Direction = CashDirectionUnknown
	}

	return stat
}

// CashFlowDirectionSummary aggregates counterparty-level cash direction data
// into a monthly or query-level response shape.
type CashFlowDirectionSummary struct {
	Period         string                     `json:"period,omitempty"`
	TotalOutflow   float64                    `json:"total_outflow"`
	TotalInflow    float64                    `json:"total_inflow"`
	Net            float64                    `json:"net"`
	Counterparties []CashFlowCounterpartyStat `json:"counterparties"`
}

// BuildCashFlowDirectionSummary aggregates reusable counterparty rows into a
// summary structure for overall expense/supplier statistics.
func BuildCashFlowDirectionSummary(rows []CashFlowCounterpartyStat) CashFlowDirectionSummary {
	summary := CashFlowDirectionSummary{
		Counterparties: append([]CashFlowCounterpartyStat(nil), rows...),
	}

	for i := range summary.Counterparties {
		summary.Counterparties[i].Outflow = roundCashFloat(summary.Counterparties[i].Outflow)
		summary.Counterparties[i].Inflow = roundCashFloat(summary.Counterparties[i].Inflow)
		summary.Counterparties[i].Net = roundCashFloat(summary.Counterparties[i].Inflow - summary.Counterparties[i].Outflow)
		if summary.Counterparties[i].Direction == "" {
			switch {
			case summary.Counterparties[i].Outflow > summary.Counterparties[i].Inflow:
				summary.Counterparties[i].Direction = CashDirectionOutflow
			case summary.Counterparties[i].Inflow > summary.Counterparties[i].Outflow:
				summary.Counterparties[i].Direction = CashDirectionInflow
			default:
				summary.Counterparties[i].Direction = CashDirectionUnknown
			}
		}

		summary.TotalOutflow += summary.Counterparties[i].Outflow
		summary.TotalInflow += summary.Counterparties[i].Inflow
	}

	summary.TotalOutflow = roundCashFloat(summary.TotalOutflow)
	summary.TotalInflow = roundCashFloat(summary.TotalInflow)
	summary.Net = roundCashFloat(summary.TotalInflow - summary.TotalOutflow)
	return summary
}

func roundCashFloat(v float64) float64 {
	return math.Round(v*100) / 100
}
