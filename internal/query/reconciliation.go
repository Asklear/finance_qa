package query

import (
	"context"
	"fmt"
	"strings"

	"financeqa/internal/analysis"
)

func shouldUseReconciliation(q string) bool {
	if containsAny(q, []string{"为什么", "怎么回事", "差异", "原因", "拆开看", "看看具体", "具体差异", "实际利润"}) {
		return containsAny(q, []string{"利润", "营收", "收入", "销售额", "成本"})
	}
	if strings.Contains(q, "营收情况") {
		return containsAny(q, []string{"为什么", "怎么回事", "差异", "拆开", "账上", "银行卡", "现金", "利润", "成本", "费用", "到账", "回款", "收款"})
	}
	if strings.Contains(q, "营收") && strings.Contains(q, "怎么样") {
		return true
	}
	return false
}

func (e *Engine) queryReconciliation(question, from, to string) Result {
	periodLabel := displayPeriod(from, to)
	e.calc.ResetTrace()

	book, bookSource, _, bookSQLs, bookLogs, err := e.bookSummaryForRange(from, to)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	cash, err := e.calc.ComputeCashFlow(e.Company, from, to)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}

	highlights := e.collectReconciliationHighlights(from, to, 8, 4)

	logs := append([]string{}, e.calc.CalculationLogs...)
	logs = append(logs, bookLogs...)
	logs = append(logs,
		fmt.Sprintf("[差异解释] %s 账上收入 %.2f, 账上成本及费用 %.2f, 净利润 %.2f, 利润 %.2f (营业外收入 %.2f, 营业外支出 %.2f)", periodLabel, book.Revenue, book.TotalCost, book.NetProfit, book.Profit, book.NonOperatingIncome, book.NonOperatingExpense),
		fmt.Sprintf("[差异解释] %s 银行卡上收款 %.2f, 付款 %.2f, 净流入 %.2f", periodLabel, cash.Income, cash.Expense, cash.Net),
	)
	var bridge *analysis.ProfitCashBridge
	var bridgeMap map[string]any
	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	sqls = append(sqls, bookSQLs...)
	if resolvedBridge, bridgeSQLs, bridgeLogs := e.rangeProfitCashBridge(context.Background(), from, to); len(bridgeLogs) > 0 || len(bridgeSQLs) > 0 || resolvedBridge != nil {
		logs = append(logs, bridgeLogs...)
		sqls = append(sqls, bridgeSQLs...)
		if resolvedBridge != nil {
			bridge = resolvedBridge
			bridgeMap = bridgeToMap(bridge)
		}
	}
	for _, snap := range highlights {
		logs = append(logs, fmt.Sprintf("[对手方归因] %s role=%s basis=%s in=%.2f out=%.2f revenue=%.2f cost=%.2f expense=%.2f vat_out=%.2f vat_in=%.2f reason=%s",
			snap.Name, snap.Role, snap.ComparisonBasis, snap.BankIn, snap.BankOut, snap.RevenueNet, snap.BookCost, snap.BookExpense, snap.OutputVAT, snap.InputVAT, snap.DifferenceReason))
	}

	sqls = append(sqls,
		"reconciliation(bank_statement): SELECT counterparty_name, SUM(credit_amount), SUM(debit_amount) FROM bank_statement WHERE ... GROUP BY counterparty_name ORDER BY ABS(net) DESC",
		"reconciliation(journal): SELECT account_code, direction, amount, summary, counterparty FROM journal WHERE ... AND (summary LIKE ? OR counterparty LIKE ?) ",
	)

	msg := e.composeBossReconciliationMessage(periodLabel, book, bookSource, cash, bridge, highlights)
	data := buildReconciliationResultData(periodLabel, book, bookSource, cash, highlights, bridgeMap)

	return Result{
		Success:         true,
		Message:         msg,
		AnswerMethod:    "sql",
		Data:            data,
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}
