package query

import (
	"fmt"
	"strings"
)

func (e *Engine) queryProjectIncomeCost(entity, from, to, question string) Result {
	snap := e.buildCounterpartySnapshot(entity, from, to)
	sqlTxt := `SELECT counterparty_name, credit_amount, debit_amount FROM bank_statement WHERE ... AND counterparty_name LIKE ?`
	if strings.Contains(question, "收入") {
		amount := round2(snap.RevenueNet)
		if amount == 0 {
			amount = round2(snap.BankIn)
		}
		return annotateJournalTaxDisclosure(Result{
			Success: true,
			Message: fmt.Sprintf("%s %s 项目收入 %.2f 元", to, entity, amount),
			Data:    map[string]any{"entity": entity, "period": to, "income": amount, "bank_in": round2(snap.BankIn), "revenue_net": round2(snap.RevenueNet)},
			ExecutedSQL: []string{
				fmt.Sprintf("queryProjectIncomeCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
			},
			CalculationLogs: []string{
				fmt.Sprintf("[项目收支] bank_in=%.2f revenue_net=%.2f", snap.BankIn, snap.RevenueNet),
			},
		}, snap.RevenueNet > 0)
	}
	amount := round2(snap.BookCost + snap.BookExpense)
	if amount == 0 {
		amount = round2(snap.BankOut)
	}
	return annotateJournalTaxDisclosure(Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 项目成本 %.2f 元", to, entity, amount),
		Data:    map[string]any{"entity": entity, "period": to, "cost": amount, "bank_out": round2(snap.BankOut), "book_cost": round2(snap.BookCost), "book_expense": round2(snap.BookExpense)},
		ExecutedSQL: []string{
			fmt.Sprintf("queryProjectIncomeCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[项目收支] bank_out=%.2f book_cost=%.2f book_expense=%.2f", snap.BankOut, snap.BookCost, snap.BookExpense),
		},
	}, snap.BookCost+snap.BookExpense > 0)
}
