package query

import (
	"strings"
	"testing"

	"financeqa/internal/accounting"
)

func TestBossDualPerspectiveMessageNamesIncomeStatementTerms(t *testing.T) {
	msg := buildBossDualPerspectiveMessage(
		"2026-01~2026-03",
		accounting.CashPerspective{Income: 120, Expense: 80, Net: 40},
		monthlyBookView{Revenue: 100, TotalCost: 70, Profit: 30},
		nil,
	)

	for _, want := range []string{"营业收入", "营业成本"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message = %q, want include %q", msg, want)
		}
	}
}
