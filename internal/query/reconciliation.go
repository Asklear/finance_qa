package query

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"financeqa/internal/accounting"
)

type evidenceLevel string

const (
	evidenceDirect  evidenceLevel = "direct"
	evidenceDerived evidenceLevel = "derived"
	evidenceUnknown evidenceLevel = "unknown"
)

type counterpartySnapshot struct {
	Name                    string        `json:"name"`
	Role                    string        `json:"role"`
	BankIn                  float64       `json:"bank_in"`
	BankOut                 float64       `json:"bank_out"`
	ARDecrease              float64       `json:"ar_decrease"`
	ARIncrease              float64       `json:"ar_increase"`
	APDecrease              float64       `json:"ap_decrease"`
	APIncrease              float64       `json:"ap_increase"`
	PrepaymentIncrease      float64       `json:"prepayment_increase"`
	PrepaymentCleared       float64       `json:"prepayment_cleared"`
	RevenueNet              float64       `json:"revenue_net"`
	OutputVAT               float64       `json:"output_vat"`
	InputVAT                float64       `json:"input_vat"`
	BookCost                float64       `json:"book_cost"`
	BookExpense             float64       `json:"book_expense"`
	ComparisonBasis         string        `json:"comparison_basis"`
	DifferenceReason        string        `json:"difference_reason"`
	EvidenceLevel           evidenceLevel `json:"evidence_level"`
	RequiresMonthDisclosure bool          `json:"requires_month_disclosure"`
	Support                 []string      `json:"support"`
}

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
		fmt.Sprintf("[差异解释] %s 账上收入 %.2f, 账上成本及费用 %.2f, 账上利润 %.2f", to, book.Revenue, book.TotalCost, book.Profit),
		fmt.Sprintf("[差异解释] %s 银行卡上收款 %.2f, 付款 %.2f, 净流入 %.2f", to, cash.Income, cash.Expense, cash.Net),
	)
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
				"账面利润":    book.Profit,
			},
		},
		"difference_summary": map[string]any{
			"book_profit":     book.Profit,
			"cash_net_inflow": cash.Net,
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

type monthlyBookView struct {
	Revenue        float64 `json:"revenue"`
	Cost           float64 `json:"cost"`
	TaxSurcharge   float64 `json:"tax_surcharge"`
	SellingExpense float64 `json:"selling_expense"`
	AdminExpense   float64 `json:"admin_expense"`
	FinanceExpense float64 `json:"finance_expense"`
	TotalCost      float64 `json:"total_cost"`
	Profit         float64 `json:"profit"`
}

func (e *Engine) monthlyBookSummary(year, month int) (monthlyBookView, string, error) {
	var revenue, cost, adminExpense, sellingExpense, financeExpense, taxSurcharge, profit float64
	hasRevenue := false
	hasCost := false
	hasProfit := false
	rows, err := e.db.Query(`
SELECT item_name, current_amount
FROM income_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
`, e.Company, e.Company, fmt.Sprintf("%04d-%02d", year, month))
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item string
			var amount float64
			if scanErr := rows.Scan(&item, &amount); scanErr != nil {
				continue
			}
			switch {
			case strings.Contains(item, "营业收入"), strings.Contains(item, "主营业务收入"), strings.Contains(item, "营业总收入"):
				revenue = amount
				hasRevenue = true
			case strings.Contains(item, "营业成本"), strings.Contains(item, "主营业务成本"):
				cost = amount
				hasCost = true
			case strings.Contains(item, "管理费用"):
				adminExpense = amount
			case strings.Contains(item, "销售费用"):
				sellingExpense = amount
			case strings.Contains(item, "财务费用"):
				financeExpense = amount
			case strings.Contains(item, "税金及附加"), strings.Contains(item, "营业税金及附加"):
				taxSurcharge = amount
			case strings.Contains(item, "净利润"), strings.Contains(item, "利润总额"):
				profit = amount
				hasProfit = true
			}
		}
	}

	if hasRevenue && hasProfit {
		totalCost := round2(cost + sellingExpense + adminExpense + financeExpense + taxSurcharge)
		if !hasCost && totalCost == 0 {
			// 利润表若只给了收入和利润，仍可稳态还原成本费用总额。
			totalCost = round2(revenue - profit)
		}
		if !hasCost {
			cost = totalCost
		}
		return monthlyBookView{
			Revenue:        round2(revenue),
			Cost:           round2(cost),
			TaxSurcharge:   round2(taxSurcharge),
			SellingExpense: round2(sellingExpense),
			AdminExpense:   round2(adminExpense),
			FinanceExpense: round2(financeExpense),
			TotalCost:      totalCost,
			Profit:         round2(profit),
		}, "income_statement", nil
	}

	monthly, err := e.calc.ComputeMonthlyFromJournal(e.Company, year, month)
	if err != nil {
		return monthlyBookView{}, "", err
	}
	is, err := e.calc.ComputeIncomeStatement(e.Company, year, month)
	if err != nil {
		return monthlyBookView{}, "", err
	}
	return monthlyBookView{
		Revenue:        monthly.Revenue,
		Cost:           is.Cost,
		TaxSurcharge:   is.TaxSurcharge,
		SellingExpense: is.SellingExpense,
		AdminExpense:   is.AdminExpense,
		FinanceExpense: is.FinanceExpense,
		TotalCost:      round2(is.Cost + is.TaxSurcharge + is.SellingExpense + is.AdminExpense + is.FinanceExpense),
		Profit:         monthly.Profit,
	}, "journal_fallback", nil
}

func (e *Engine) topCounterpartiesByCashMovement(from, to string, limit int) []string {
	rows, err := e.db.Query(`
SELECT counterparty_name,
       COALESCE(SUM(credit_amount), 0) AS bank_in,
       COALESCE(SUM(debit_amount), 0) AS bank_out
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
  AND IFNULL(TRIM(counterparty_name), '') <> ''
GROUP BY counterparty_name
ORDER BY ABS(COALESCE(SUM(credit_amount), 0) - COALESCE(SUM(debit_amount), 0)) DESC,
         (COALESCE(SUM(credit_amount), 0) + COALESCE(SUM(debit_amount), 0)) DESC
LIMIT ?
`, e.Company, e.Company, from+"-01", monthEndDay(to), limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]string, 0, limit)
	for rows.Next() {
		var name string
		var inAmt, outAmt float64
		if scanErr := rows.Scan(&name, &inAmt, &outAmt); scanErr != nil {
			continue
		}
		if strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func (e *Engine) collectCounterpartyEvidence(name, from, to string) []LedgerEvidence {
	like := "%" + name + "%"
	evidence := make([]LedgerEvidence, 0, 32)
	startDate := from + "-01"
	endDate := monthEndDay(to)

	bankRows, err := e.db.Query(`
SELECT counterparty_name, summary, COALESCE(debit_amount, 0), COALESCE(credit_amount, 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
  AND counterparty_name LIKE ?
`, e.Company, e.Company, startDate, endDate, like)
	if err == nil {
		defer bankRows.Close()
		for bankRows.Next() {
			var counterparty, summary string
			var debitAmt, creditAmt float64
			if scanErr := bankRows.Scan(&counterparty, &summary, &debitAmt, &creditAmt); scanErr != nil {
				continue
			}
			evidence = append(evidence, LedgerEvidence{
				Source:       "bank_statement",
				Counterparty: counterparty,
				Summary:      summary,
				DebitAmount:  debitAmt,
				CreditAmount: creditAmt,
			})
		}
	}

	journalRows, err := e.db.Query(`
SELECT IFNULL(counterparty, ''), account_code, account_name, summary, direction, COALESCE(debit_amount, 0), COALESCE(credit_amount, 0)
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
  AND (summary LIKE ? OR IFNULL(counterparty, '') LIKE ?)
`, e.Company, e.Company, startDate, endDate, like, like)
	if err == nil {
		defer journalRows.Close()
		for journalRows.Next() {
			var counterparty, accountCode, accountName, summary, direction string
			var debitAmt, creditAmt float64
			if scanErr := journalRows.Scan(&counterparty, &accountCode, &accountName, &summary, &direction, &debitAmt, &creditAmt); scanErr != nil {
				continue
			}
			evidence = append(evidence, LedgerEvidence{
				Source:       "journal",
				Counterparty: counterparty,
				AccountCode:  accountCode,
				AccountName:  accountName,
				Summary:      summary,
				Direction:    direction,
				DebitAmount:  debitAmt,
				CreditAmount: creditAmt,
			})
		}
	}

	return evidence
}

func (e *Engine) buildCounterpartySnapshot(name, from, to string) counterpartySnapshot {
	snap := counterpartySnapshot{Name: name, Role: "unknown", EvidenceLevel: evidenceDerived}
	evidence := e.collectCounterpartyEvidence(name, from, to)
	classification := ClassifyCounterparty(name, evidence)
	taxReport := NormalizeTax(name, evidence)
	support := make([]string, 0, 8)
	for _, ev := range evidence {
		code := ev.AccountCode
		direction := ev.Direction
		amount := ev.CreditAmount
		if amount == 0 {
			amount = ev.DebitAmount
		}
		if ev.Source == "bank_statement" {
			snap.BankIn += ev.CreditAmount
			snap.BankOut += ev.DebitAmount
		}
		switch {
		case strings.HasPrefix(code, "1122"):
			if direction == "贷" {
				snap.ARDecrease += amount
			} else {
				snap.ARIncrease += amount
			}
		case strings.HasPrefix(code, "2202"):
			if direction == "借" {
				snap.APDecrease += amount
			} else {
				snap.APIncrease += amount
			}
		case strings.HasPrefix(code, "1123"):
			if direction == "借" {
				snap.PrepaymentIncrease += amount
			} else {
				snap.PrepaymentCleared += amount
			}
		case strings.HasPrefix(code, "6001"), strings.HasPrefix(code, "6051"):
			if direction == "贷" {
				snap.RevenueNet += amount
			} else {
				snap.RevenueNet -= amount
			}
		case strings.HasPrefix(code, "22210106"):
			if direction == "贷" {
				snap.OutputVAT += amount
			} else {
				snap.OutputVAT -= amount
			}
		case strings.HasPrefix(code, "22210101"):
			if direction == "借" {
				snap.InputVAT += amount
			} else {
				snap.InputVAT -= amount
			}
		case strings.HasPrefix(code, "6401"):
			if direction == "借" {
				snap.BookCost += amount
			} else {
				snap.BookCost -= amount
			}
		case strings.HasPrefix(code, "660"):
			if direction == "借" {
				snap.BookExpense += amount
			} else {
				snap.BookExpense -= amount
			}
		}

		if len(support) < 8 {
			brief := ev.Summary
			if brief == "" {
				brief = ev.Counterparty
			}
			support = append(support, fmt.Sprintf("%s %s %.2f %s", code, direction, amount, brief))
		}
	}
	snap.Support = support
	snap.Role = string(classification.Role)
	if snap.Role == "" {
		snap.Role = "unknown"
	}

	switch {
	case snap.BankIn > 0 && snap.ARDecrease > 0 && snap.RevenueNet > 0:
		snap.ComparisonBasis = "historical_receipt_and_current_revenue"
		snap.DifferenceReason = "同一对手方本月同时出现历史应收回款和当月确认收入，到账金额不能直接当成当月收入。"
		snap.EvidenceLevel = evidenceDerived
		snap.RequiresMonthDisclosure = true
	case taxReport.Output.Included && snap.BankIn > 0 && approxEqual(snap.BankIn, taxReport.Output.AccrualAmount+taxReport.Output.TaxAmount):
		snap.ComparisonBasis = "vat_gap_only"
		snap.DifferenceReason = taxReport.Output.DifferenceReason
		snap.EvidenceLevel = evidenceDirect
	case taxReport.Input.Included && snap.BankOut > 0 && (snap.BookCost > 0 || snap.BookExpense > 0 || snap.InputVAT > 0 || snap.PrepaymentIncrease > 0 || snap.APDecrease > 0):
		snap.ComparisonBasis = "supplier_payment_or_cost"
		snap.DifferenceReason = taxReport.Input.DifferenceReason
		snap.EvidenceLevel = evidenceDirect
	case strings.Contains(taxReport.Output.DifferenceReason, "历史应收回款") || (snap.BankIn > 0 && snap.ARDecrease > 0):
		snap.ComparisonBasis = "historical_receipt"
		snap.DifferenceReason = "数据库能确认这是一笔冲减历史应收的回款，但没有字段直接说明对应哪一个结算月份。"
		snap.EvidenceLevel = evidenceUnknown
		snap.RequiresMonthDisclosure = true
	case snap.RevenueNet > 0:
		snap.ComparisonBasis = "recognized_revenue"
		snap.DifferenceReason = "本月有账上确认收入。"
	}

	return snap
}

func (e *Engine) composeBossReconciliationMessage(period string, book monthlyBookView, bookSource string, cash *accounting.CashPerspective, highlights []counterpartySnapshot) string {
	lines := []string{
		fmt.Sprintf("%s 我拆成两层给你看：账上看收入 %.2f 元、成本及费用 %.2f 元、净利润 %.2f 元；银行卡上看收款 %.2f 元、付款 %.2f 元，净流入 %.2f 元。", period, book.Revenue, book.TotalCost, book.Profit, cash.Income, cash.Expense, cash.Net),
	}
	if bookSource == "income_statement" {
		lines = append(lines, "账上这组数优先取利润表当月发生额，银行卡这组数取当月真实收付。两边不是同一个口径，所以不能直接拿来做差。")
	}
	if len(highlights) > 0 {
		lines = append(lines, "差异主要来自这几类：")
	}
	for _, snap := range highlights {
		switch snap.ComparisonBasis {
		case "historical_receipt_and_current_revenue":
			lines = append(lines, fmt.Sprintf("1. %s：本月既有到账 %.2f 元，也有账上确认收入 %.2f 元和销项税 %.2f 元。库里能确认这是“历史应收回款 + 当月新确认收入”同时出现，不能把两笔直接相减；现有库里也看不出到账对应的是哪一个结算月份。", snap.Name, snap.BankIn, snap.RevenueNet, snap.OutputVAT))
		case "vat_gap_only":
			lines = append(lines, fmt.Sprintf("1. %s：到账 %.2f 元，账上收入 %.2f 元，差额 %.2f 元主要就是销项税，不是业务多赚少赚。", snap.Name, snap.BankIn, snap.RevenueNet, snap.OutputVAT))
		case "supplier_payment_or_cost":
			lines = append(lines, fmt.Sprintf("1. %s：这是供应商相关付款和成本确认。2 月付款 %.2f 元，账上成本/费用 %.2f 元，进项税 %.2f 元，不该放进收入差异里。", snap.Name, snap.BankOut, snap.BookCost+snap.BookExpense, snap.InputVAT))
		case "historical_receipt":
			lines = append(lines, fmt.Sprintf("1. %s：这笔到账 %.2f 元能确认是在冲历史应收，但数据库没有字段直接说明对应哪一个结算月份，所以最多只能说是历史回款。", snap.Name, snap.BankIn))
		case "recognized_revenue":
			lines = append(lines, fmt.Sprintf("1. %s：本月账上确认收入 %.2f 元。", snap.Name, snap.RevenueNet))
		}
	}
	if len(highlights) > 0 {
		lines = append(lines, "如果你要继续追问“这笔到底对应哪一月结算”，下一步得补结算单、开票记录或合同台账，单靠当前 `finance.db` 不能硬判。")
	}
	return strings.Join(lines, "\n")
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) <= 0.02
}
