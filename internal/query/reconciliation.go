package query

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"financeqa/internal/analysis"
)

func shouldUseReconciliation(q string) bool {
	if containsAny(q, []string{"为什么", "怎么回事", "差异", "原因", "拆开看", "看看具体", "具体差异", "实际利润"}) {
		return containsAny(q, []string{"利润", "营收", "收入", "销售额", "成本"})
	}
	if strings.Contains(q, "营收情况") {
		return true
	}
	if strings.Contains(q, "营收") && strings.Contains(q, "怎么样") {
		return true
	}
	return false
}

func (e *Engine) queryReconciliation(question, from, to string) Result {
	year, month := parsePeriod(to)
	e.calc.ResetTrace()

	book, bookSource, err := e.monthlyBookSummary(year, month)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	cash, err := e.calc.ComputeCashFlow(e.Company, from, to)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}

	interesting := e.topCounterpartiesByCashMovement(from, to, 8)
	snapshots := make([]counterpartySnapshot, 0, len(interesting))
	for _, name := range interesting {
		snap := e.buildCounterpartySnapshot(name, from, to)
		if snap.Role == "unknown" && snap.BankIn == 0 && snap.BankOut == 0 && snap.RevenueNet == 0 && snap.BookCost == 0 && snap.BookExpense == 0 {
			continue
		}
		snapshots = append(snapshots, snap)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		left := math.Max(snapshots[i].BankIn+snapshots[i].BankOut, snapshots[i].RevenueNet+snapshots[i].BookCost+snapshots[i].BookExpense)
		right := math.Max(snapshots[j].BankIn+snapshots[j].BankOut, snapshots[j].RevenueNet+snapshots[j].BookCost+snapshots[j].BookExpense)
		return left > right
	})

	highlights := make([]counterpartySnapshot, 0, 4)
	for _, snap := range snapshots {
		if snap.ComparisonBasis == "" {
			continue
		}
		highlights = append(highlights, snap)
		if len(highlights) == 4 {
			break
		}
	}

	logs := append([]string{}, e.calc.CalculationLogs...)
	logs = append(logs,
		fmt.Sprintf("[差异解释] %s 账上收入 %.2f, 账上成本及费用 %.2f, 净利润 %.2f, 利润 %.2f (营业外收入 %.2f, 营业外支出 %.2f)", to, book.Revenue, book.TotalCost, book.NetProfit, book.Profit, book.NonOperatingIncome, book.NonOperatingExpense),
		fmt.Sprintf("[差异解释] %s 银行卡上收款 %.2f, 付款 %.2f, 净流入 %.2f", to, cash.Income, cash.Expense, cash.Net),
	)
	var bridgeMap map[string]any
	if bridge, bridgeErr := analysis.AnalyzeProfitCashBridgeWithDB(context.Background(), e.db, e.Company, to); bridgeErr == nil {
		bridgeMap = bridgeToMap(&bridge)
		logs = append(logs, fmt.Sprintf("[利润调现金桥] period=%s estimated_operating_cash=%.2f bank_net_cash=%.2f non_operating_delta=%.2f", to, bridge.EstimatedOperatingCash, bridge.BankNetCash, bridge.NonOperatingCashDelta))
	}
	for _, snap := range highlights {
		logs = append(logs, fmt.Sprintf("[对手方归因] %s role=%s basis=%s in=%.2f out=%.2f revenue=%.2f cost=%.2f expense=%.2f vat_out=%.2f vat_in=%.2f reason=%s",
			snap.Name, snap.Role, snap.ComparisonBasis, snap.BankIn, snap.BankOut, snap.RevenueNet, snap.BookCost, snap.BookExpense, snap.OutputVAT, snap.InputVAT, snap.DifferenceReason))
	}

	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	sqls = append(sqls,
		"reconciliation(bank_statement): SELECT counterparty_name, SUM(credit_amount), SUM(debit_amount) FROM bank_statement WHERE ... GROUP BY counterparty_name ORDER BY ABS(net) DESC",
		"reconciliation(journal): SELECT account_code, direction, amount, summary, counterparty FROM journal WHERE ... AND (summary LIKE ? OR counterparty LIKE ?) ",
	)

	msg := e.composeBossReconciliationMessage(to, book, bookSource, cash, highlights)
	highlightMaps := make([]map[string]any, 0, len(highlights))
	for _, snap := range highlights {
		highlightMaps = append(highlightMaps, map[string]any{
			"name":                      snap.Name,
			"role":                      snap.Role,
			"bank_in":                   round2(snap.BankIn),
			"bank_out":                  round2(snap.BankOut),
			"ar_decrease":               round2(snap.ARDecrease),
			"ar_increase":               round2(snap.ARIncrease),
			"ap_decrease":               round2(snap.APDecrease),
			"ap_increase":               round2(snap.APIncrease),
			"prepayment_increase":       round2(snap.PrepaymentIncrease),
			"prepayment_cleared":        round2(snap.PrepaymentCleared),
			"revenue_net":               round2(snap.RevenueNet),
			"output_vat":                round2(snap.OutputVAT),
			"input_vat":                 round2(snap.InputVAT),
			"book_cost":                 round2(snap.BookCost),
			"book_expense":              round2(snap.BookExpense),
			"comparison_basis":          snap.ComparisonBasis,
			"difference_reason":         snap.DifferenceReason,
			"evidence_level":            string(snap.EvidenceLevel),
			"requires_month_disclosure": snap.RequiresMonthDisclosure,
			"support":                   append([]string{}, snap.Support...),
		})
	}
	data := map[string]any{
		"period":      to,
		"book_view":   book,
		"cash_view":   cash,
		"highlights":  highlightMaps,
		"book_source": bookSource,
		"dual_perspective": map[string]any{
			"cash": map[string]any{
				"说明":   "银行卡上看",
				"现金流入": cash.Income,
				"现金流出": cash.Expense,
				"净现金流": cash.Net,
			},
			"accrual": map[string]any{
				"说明":      "账上看",
				"营业收入":    book.Revenue,
				"营业成本及费用": book.TotalCost,
				"营业外收入":   book.NonOperatingIncome,
				"营业外支出":   book.NonOperatingExpense,
				"账面利润":    book.Profit,
			},
		},
		"difference_summary": map[string]any{
			"book_profit":        book.Profit,
			"cash_net_inflow":    cash.Net,
			"profit_cash_bridge": bridgeMap,
			"notices": []string{
				"银行卡收付和账上利润不是同一口径，差异需要拆成回款、税额、供应商付款和成本确认来看。",
				"若数据库没有结算月份字段，只能确认是历史应收回款，不能硬说对应哪一个结算月份。",
			},
		},
		"现金流入": cash.Income,
		"现金流出": cash.Expense,
		"净现金流": cash.Net,
		"账上看利润": map[string]any{
			"营业收入":    book.Revenue,
			"营业成本及费用": book.TotalCost,
			"营业外收入":   book.NonOperatingIncome,
			"营业外支出":   book.NonOperatingExpense,
			"账面利润":    book.Profit,
		},
	}

	return Result{
		Success:         true,
		Message:         msg,
		AnswerMethod:    "sql",
		Data:            data,
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}
