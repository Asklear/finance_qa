package query_test

import (
	"testing"

	"financeqa/internal/query"
)

func TestBankCashDirectionForBankAccounts(t *testing.T) {
	t.Run("1001 debit is inflow", func(t *testing.T) {
		got := query.BankCashDirection("1001", "借")
		if got != query.CashDirectionInflow {
			t.Fatalf("BankCashDirection(1001, 借) = %q, want %q", got, query.CashDirectionInflow)
		}
	})

	t.Run("1002 credit is outflow", func(t *testing.T) {
		got := query.BankCashDirection("1002", "贷")
		if got != query.CashDirectionOutflow {
			t.Fatalf("BankCashDirection(1002, 贷) = %q, want %q", got, query.CashDirectionOutflow)
		}
	})
}

func TestBankCashDirectionSkipsNonCashAccounts(t *testing.T) {
	got := query.BankCashDirection("1122", "借")
	if got != query.CashDirectionUnknown {
		t.Fatalf("BankCashDirection(1122, 借) = %q, want %q", got, query.CashDirectionUnknown)
	}
}

func TestBuildCashFlowDirectionSummary(t *testing.T) {
	rows := []query.CashFlowCounterpartyStat{
		query.NewCashFlowCounterpartyStat("供应商A", 300, 20),
		query.NewCashFlowCounterpartyStat("客户B", 10, 100),
	}

	summary := query.BuildCashFlowDirectionSummary(rows)

	if summary.TotalOutflow != 310 {
		t.Fatalf("summary.TotalOutflow = %.2f, want 310", summary.TotalOutflow)
	}
	if summary.TotalInflow != 120 {
		t.Fatalf("summary.TotalInflow = %.2f, want 120", summary.TotalInflow)
	}
	if summary.Net != -190 {
		t.Fatalf("summary.Net = %.2f, want -190", summary.Net)
	}
	if len(summary.Counterparties) != 2 {
		t.Fatalf("summary.Counterparties len = %d, want 2", len(summary.Counterparties))
	}
	if summary.Counterparties[0].Direction != query.CashDirectionOutflow {
		t.Fatalf("counterparty[0].Direction = %q, want %q", summary.Counterparties[0].Direction, query.CashDirectionOutflow)
	}
	if summary.Counterparties[1].Direction != query.CashDirectionInflow {
		t.Fatalf("counterparty[1].Direction = %q, want %q", summary.Counterparties[1].Direction, query.CashDirectionInflow)
	}
}
