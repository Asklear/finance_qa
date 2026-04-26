package query

import (
	"fmt"
	"sort"
	"strings"
)

type expenseBreakdownRow struct {
	Category         string
	CustomerName     string
	ContractContent  string
	AccountName      string
	Amount           float64
	SettlementAmount float64
	PaidAmount       float64
	InvoiceAmount    float64
	TxnCount         int
}

func shouldUseExpenseBreakdown(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	cfg := getRuleConfig()
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) || shouldUseHRBreakdown(q, cfg) {
		return false
	}
	asksBreakdown := containsAny(q, []string{"拆", "拆分", "拆开", "大类", "构成", "分类", "类别", "结构", "花哪", "花到哪", "花在哪"})
	if !asksBreakdown {
		return false
	}
	if containsAny(q, []string{"收入", "营收", "利润"}) && !containsAny(q, []string{"支出", "费用", "开支", "花", "付款", "支付", "成本"}) {
		return false
	}
	if containsAny(q, []string{"整体支出", "总支出", "全部支出", "支出", "费用", "开支", "钱花", "花了", "花哪", "付款", "支付"}) {
		return true
	}
	return containsAny(q, []string{"成本"}) && containsAny(q, []string{"拆分", "拆开", "大类", "构成", "分类", "类别"})
}

func (e *Engine) queryExpenseBreakdownAllPerspectives(from, to string) Result {
	periodLabel := displayPeriod(from, to)
	contractRows, contractTotal, contractPaid, contractSQL, contractLogs := e.collectContractProjectExpenseBreakdown(from, to)
	cashRows, cashTotal, cashSQL, cashLogs := e.collectCashCategoryExpenseBreakdown(from, to)
	accountRows, accountTotal, accountSQL, accountLogs := e.collectAccountCategoryExpenseBreakdown(from, to)

	views := map[string]any{
		"contract_project": map[string]any{
			"label":       "合同/项目口径",
			"description": "按合同成本结算表看成本确认和合同付款",
			"total":       round2(contractTotal),
			"paid_total":  round2(contractPaid),
			"rows":        contractProjectRowsToMaps(contractRows, contractTotal),
		},
		"cash_category": map[string]any{
			"label":       "现金流水口径",
			"description": "按银行流水实际流出拆大类",
			"total":       round2(cashTotal),
			"rows":        categoryRowsToMaps(cashRows, cashTotal),
		},
		"account_category": map[string]any{
			"label":       "账务科目口径",
			"description": "按序时账成本费用类借方发生额拆大类",
			"total":       round2(accountTotal),
			"rows":        accountRowsToMaps(accountRows, accountTotal),
		},
	}

	message := composeExpenseBreakdownMessage(periodLabel, contractRows, contractTotal, contractPaid, cashRows, cashTotal, accountRows, accountTotal)
	sqls := append([]string{}, contractSQL...)
	sqls = append(sqls, cashSQL...)
	sqls = append(sqls, accountSQL...)
	logs := append([]string{}, contractLogs...)
	logs = append(logs, cashLogs...)
	logs = append(logs, accountLogs...)

	return Result{
		Success:      true,
		Message:      message,
		AnswerMethod: "sql",
		Data: map[string]any{
			"period":                   periodLabel,
			"period_from":              from,
			"period_to":                to,
			"metric":                   "整体支出拆分",
			"breakdown_views":          views,
			"source_primary_tables":    []string{"fin_cost_settlements", "fin_bank_statement", "fin_journal"},
			"source_supporting_tables": []string{"fin_contracts"},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func (e *Engine) collectContractProjectExpenseBreakdown(from, to string) ([]expenseBreakdownRow, float64, float64, []string, []string) {
	sqlText := `
SELECT c.customer_name,
       c.contract_content,
       COALESCE(SUM(cs.settlement_amount), 0),
       COALESCE(SUM(cs.paid_amount), 0),
       COALESCE(SUM(cs.invoice_amount), 0)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?
GROUP BY c.customer_name, c.contract_content
HAVING COALESCE(SUM(cs.settlement_amount), 0) <> 0
    OR COALESCE(SUM(cs.paid_amount), 0) <> 0
ORDER BY 3 DESC, c.customer_name, c.contract_content`
	rows, err := e.db.Query(sqlText, from, to)
	executed := []string{"expense_breakdown(contract_project): SELECT customer_name, contract_content, SUM(settlement_amount), SUM(paid_amount), SUM(invoice_amount) FROM fin_cost_settlements JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ? GROUP BY customer_name, contract_content"}
	logs := []string{fmt.Sprintf("[整体支出拆分-合同项目] period=%s", displayPeriod(from, to))}
	if err != nil {
		logs = append(logs, fmt.Sprintf("[整体支出拆分-合同项目] skipped error=%v", err))
		return nil, 0, 0, executed, logs
	}
	defer rows.Close()

	out := make([]expenseBreakdownRow, 0)
	var total, paidTotal float64
	for rows.Next() {
		var row expenseBreakdownRow
		if err := rows.Scan(&row.CustomerName, &row.ContractContent, &row.SettlementAmount, &row.PaidAmount, &row.InvoiceAmount); err != nil {
			continue
		}
		row.SettlementAmount = round2(row.SettlementAmount)
		row.PaidAmount = round2(row.PaidAmount)
		row.InvoiceAmount = round2(row.InvoiceAmount)
		row.Amount = row.SettlementAmount
		total += row.SettlementAmount
		paidTotal += row.PaidAmount
		out = append(out, row)
	}
	total = round2(total)
	paidTotal = round2(paidTotal)
	logs = append(logs, fmt.Sprintf("[整体支出拆分-合同项目] rows=%d cost=%.2f paid=%.2f", len(out), total, paidTotal))
	return out, total, paidTotal, executed, logs
}

func (e *Engine) collectCashCategoryExpenseBreakdown(from, to string) ([]expenseBreakdownRow, float64, []string, []string) {
	startDate := from + "-01"
	endDate := monthEndDay(to)
	sqlText := `
SELECT COALESCE(counterparty_name, ''),
       COALESCE(summary, ''),
       COALESCE(debit_amount, 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
  AND COALESCE(debit_amount, 0) > 0`
	rows, err := e.db.Query(sqlText, e.Company, e.Company, startDate, endDate)
	executed := []string{"expense_breakdown(cash_category): SELECT counterparty_name, summary, debit_amount FROM bank_statement WHERE company/date match AND debit_amount > 0"}
	logs := []string{fmt.Sprintf("[整体支出拆分-现金流水] period=%s start=%s end=%s", displayPeriod(from, to), startDate, endDate)}
	if err != nil {
		logs = append(logs, fmt.Sprintf("[整体支出拆分-现金流水] skipped error=%v", err))
		return nil, 0, executed, logs
	}
	defer rows.Close()

	byCategory := map[string]*expenseBreakdownRow{}
	total := 0.0
	for rows.Next() {
		var name, summary string
		var amount float64
		if err := rows.Scan(&name, &summary, &amount); err != nil {
			continue
		}
		amount = round2(amount)
		category := e.classifyCashExpenseCategory(name, summary)
		row := byCategory[category]
		if row == nil {
			row = &expenseBreakdownRow{Category: category}
			byCategory[category] = row
		}
		row.Amount = round2(row.Amount + amount)
		row.TxnCount++
		total += amount
	}
	out := expenseBreakdownRowsFromMap(byCategory)
	total = round2(total)
	logs = append(logs, fmt.Sprintf("[整体支出拆分-现金流水] categories=%d total=%.2f", len(out), total))
	return out, total, executed, logs
}

func (e *Engine) collectAccountCategoryExpenseBreakdown(from, to string) ([]expenseBreakdownRow, float64, []string, []string) {
	sqlText := `
SELECT COALESCE(account_code, ''),
       COALESCE(account_name, ''),
       COALESCE(summary, ''),
       CASE WHEN COALESCE(debit_amount, 0) <> 0 THEN COALESCE(debit_amount, 0) ELSE COALESCE(amount, 0) END
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
  AND (CASE WHEN COALESCE(debit_amount, 0) <> 0 THEN COALESCE(debit_amount, 0) ELSE COALESCE(amount, 0) END) > 0
  AND (direction = '借' OR direction = 'debit' OR direction = 'DEBIT' OR COALESCE(TRIM(direction), '') = '')
  AND (
       account_code LIKE '6401%' OR account_code LIKE '6402%' OR
       account_code LIKE '6601%' OR account_code LIKE '6602%' OR account_code LIKE '6603%' OR
       account_code LIKE '6711%' OR account_code LIKE '6801%' OR
       account_name LIKE '%成本%' OR account_name LIKE '%费用%' OR
       account_name LIKE '%税金及附加%' OR account_name LIKE '%营业外支出%'
  )`
	rows, err := e.db.Query(sqlText, e.Company, e.Company, from, to)
	executed := []string{"expense_breakdown(account_category): SELECT account_code, account_name, summary, debit_amount FROM journal WHERE period match AND cost/expense accounts"}
	logs := []string{fmt.Sprintf("[整体支出拆分-账务科目] period=%s", displayPeriod(from, to))}
	if err != nil {
		logs = append(logs, fmt.Sprintf("[整体支出拆分-账务科目] skipped error=%v", err))
		return nil, 0, executed, logs
	}
	defer rows.Close()

	byCategory := map[string]*expenseBreakdownRow{}
	total := 0.0
	for rows.Next() {
		var accountCode, accountName, summary string
		var amount float64
		if err := rows.Scan(&accountCode, &accountName, &summary, &amount); err != nil {
			continue
		}
		amount = round2(amount)
		category := classifyAccountExpenseCategory(accountCode, accountName, summary)
		row := byCategory[category]
		if row == nil {
			row = &expenseBreakdownRow{Category: category, AccountName: accountName}
			byCategory[category] = row
		}
		row.Amount = round2(row.Amount + amount)
		row.TxnCount++
		total += amount
	}
	out := expenseBreakdownRowsFromMap(byCategory)
	total = round2(total)
	logs = append(logs, fmt.Sprintf("[整体支出拆分-账务科目] categories=%d total=%.2f", len(out), total))
	return out, total, executed, logs
}

func (e *Engine) classifyCashExpenseCategory(counterparty, summary string) string {
	cfg := getRuleConfig()
	text := normalizeEntityText(counterparty + " " + summary)
	switch {
	case containsAny(text, []string{"税务", "税局", "缴税", "纳税", "增值税", "所得税", "附加税", "印花税", "个税"}):
		return "税费"
	case containsAny(text, []string{"手续费", "利息", "财务费用", "银行费用"}):
		return "银行费用"
	case containsAny(text, cfg.CounterpartyRoleKeywords(CounterpartyEmployee)):
		return "人力薪酬"
	case internalPartyMatchesCompany(e.Company, counterparty) || looksLikeInternalOrgUnit(counterparty, cfg):
		return "内部往来"
	case containsAny(text, cfg.CounterpartyRoleKeywords(CounterpartySupplier)):
		return "供应商付款"
	case looksLikeExternalOrganizationCounterparty(counterparty):
		return "供应商付款"
	default:
		return "其他支出"
	}
}

func classifyAccountExpenseCategory(accountCode, accountName, summary string) string {
	text := normalizeEntityText(accountCode + " " + accountName + " " + summary)
	cfg := getRuleConfig()
	switch {
	case containsAny(text, cfg.CounterpartyRoleKeywords(CounterpartyEmployee)):
		return "人力薪酬"
	case containsAny(text, []string{"税金及附加", "所得税费用", "税费"}):
		return "税费"
	case containsAny(text, []string{"手续费", "利息", "财务费用", "6603"}):
		return "银行及财务费用"
	case containsAny(text, []string{"销售费用", "6601"}):
		return "销售费用"
	case containsAny(text, []string{"管理费用", "6602"}):
		return "管理费用"
	case containsAny(text, []string{"主营业务成本", "营业成本", "成本", "6401", "6402"}):
		return "业务成本"
	case containsAny(text, []string{"营业外支出", "6711"}):
		return "营业外支出"
	default:
		name := strings.TrimSpace(accountName)
		if name == "" {
			return "其他费用"
		}
		return name
	}
}

func expenseBreakdownRowsFromMap(rows map[string]*expenseBreakdownRow) []expenseBreakdownRow {
	out := make([]expenseBreakdownRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Amount == out[j].Amount {
			return out[i].Category < out[j].Category
		}
		return out[i].Amount > out[j].Amount
	})
	return out
}

func contractProjectRowsToMaps(rows []expenseBreakdownRow, total float64) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"customer_name":     row.CustomerName,
			"contract_content":  row.ContractContent,
			"settlement_amount": round2(row.SettlementAmount),
			"paid_amount":       round2(row.PaidAmount),
			"invoice_amount":    round2(row.InvoiceAmount),
			"share":             shareOf(row.SettlementAmount, total),
		})
	}
	return out
}

func categoryRowsToMaps(rows []expenseBreakdownRow, total float64) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"category":  row.Category,
			"amount":    round2(row.Amount),
			"share":     shareOf(row.Amount, total),
			"txn_count": row.TxnCount,
		})
	}
	return out
}

func accountRowsToMaps(rows []expenseBreakdownRow, total float64) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"category":  row.Category,
			"amount":    round2(row.Amount),
			"share":     shareOf(row.Amount, total),
			"txn_count": row.TxnCount,
		})
	}
	return out
}

func shareOf(amount, total float64) float64 {
	if total == 0 {
		return 0
	}
	return round2(amount / total * 100)
}

func composeExpenseBreakdownMessage(period string, contractRows []expenseBreakdownRow, contractTotal, contractPaid float64, cashRows []expenseBreakdownRow, cashTotal float64, accountRows []expenseBreakdownRow, accountTotal float64) string {
	return strings.Join([]string{
		fmt.Sprintf("%s 整体支出已按所有可用口径拆开：", period),
		fmt.Sprintf("1. 合同/项目口径：合同成本 %.2f 元，合同付款 %.2f 元。主要项目：%s。", round2(contractTotal), round2(contractPaid), summarizeContractProjectRows(contractRows, 5)),
		fmt.Sprintf("2. 现金流水口径：银行实际流出 %.2f 元。大类：%s。", round2(cashTotal), summarizeCategoryRows(cashRows, 6)),
		fmt.Sprintf("3. 账务科目口径：账上成本及费用 %.2f 元。科目：%s。", round2(accountTotal), summarizeCategoryRows(accountRows, 6)),
		"说明：三种口径分别看合同成本确认、银行实际付款、账务入账成本费用，金额不要求相加一致。",
	}, "\n")
}

func summarizeContractProjectRows(rows []expenseBreakdownRow, limit int) string {
	if len(rows) == 0 {
		return "暂无合同/项目成本记录"
	}
	parts := make([]string, 0, minInt(len(rows), limit))
	for i, row := range rows {
		if i >= limit {
			break
		}
		name := strings.TrimSpace(row.CustomerName)
		content := strings.TrimSpace(row.ContractContent)
		label := name
		if content != "" {
			if label != "" {
				label += "-" + content
			} else {
				label = content
			}
		}
		if label == "" {
			label = "未命名项目"
		}
		parts = append(parts, fmt.Sprintf("%s %.2f 元", label, round2(row.SettlementAmount)))
	}
	return strings.Join(parts, "；")
}

func summarizeCategoryRows(rows []expenseBreakdownRow, limit int) string {
	if len(rows) == 0 {
		return "暂无记录"
	}
	parts := make([]string, 0, minInt(len(rows), limit))
	for i, row := range rows {
		if i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %.2f 元", row.Category, round2(row.Amount)))
	}
	return strings.Join(parts, "；")
}
