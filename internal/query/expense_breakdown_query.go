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

type expenseBreakdownPerspective string

const (
	expenseBreakdownPerspectiveContract expenseBreakdownPerspective = "contract_project"
	expenseBreakdownPerspectiveCash     expenseBreakdownPerspective = "cash_category"
	expenseBreakdownPerspectiveAccount  expenseBreakdownPerspective = "account_category"
	expenseBreakdownPerspectiveAll      expenseBreakdownPerspective = "all"
)

func shouldUseExpenseBreakdown(q string) bool {
	return shouldUseExpenseBreakdownWithConfig(q, getRuleConfig())
}

func shouldUseExpenseBreakdownWithConfig(q string, cfg RuleConfig) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) || shouldUseHRBreakdown(q, cfg) {
		return false
	}
	if shouldUseContractCostAnalysisQuestion(q, cfg) {
		return false
	}
	asksBreakdown := containsAny(q, cfg.ExpenseBreakdownTriggerKeywords())
	if !asksBreakdown {
		return false
	}
	if containsAny(q, cfg.ExpenseBreakdownMetricBlockKeywords()) && !containsAny(q, cfg.ExpenseBreakdownMetricAllowKeywords()) {
		return false
	}
	if containsAny(q, cfg.ExpenseBreakdownExpenseKeywords()) {
		return true
	}
	return containsAny(q, cfg.ExpenseBreakdownCostKeywords())
}

func resolveExpenseBreakdownPerspective(q string) expenseBreakdownPerspective {
	q = strings.TrimSpace(q)
	if containsAny(q, []string{"全部口径", "所有口径", "三种口径", "多口径", "各口径", "口径对比", "对比"}) {
		return expenseBreakdownPerspectiveAll
	}
	if containsAny(q, []string{"流水", "银行", "现金", "现金口径", "实际付款", "实际支出", "实际支付", "银行付款", "银行支出"}) {
		return expenseBreakdownPerspectiveCash
	}
	if containsAny(q, []string{"序时账", "科目", "入账", "账务", "账上", "账面", "会计口径", "科目口径"}) {
		return expenseBreakdownPerspectiveAccount
	}
	return expenseBreakdownPerspectiveContract
}

func (e *Engine) queryExpenseBreakdown(q, from, to string) Result {
	switch resolveExpenseBreakdownPerspective(q) {
	case expenseBreakdownPerspectiveAll:
		return e.queryExpenseBreakdownAllPerspectives(from, to)
	case expenseBreakdownPerspectiveCash:
		return e.queryCashExpenseBreakdown(from, to, "")
	case expenseBreakdownPerspectiveAccount:
		return e.queryAccountExpenseBreakdown(from, to, "")
	default:
		return e.queryContractFirstExpenseBreakdown(from, to)
	}
}

func (e *Engine) queryContractFirstExpenseBreakdown(from, to string) Result {
	cfg := e.currentRuleConfig()
	contractRows, contractTotal, contractPaid, contractSQL, contractLogs := e.collectContractProjectExpenseBreakdown(from, to)
	if len(contractRows) > 0 || contractTotal != 0 || contractPaid != 0 {
		return buildContractProjectExpenseBreakdownResult(displayPeriod(from, to), from, to, contractRows, contractTotal, contractPaid, contractSQL, contractLogs, "", cfg)
	}

	accountRows, accountTotal, accountSQL, accountLogs := e.collectAccountCategoryExpenseBreakdown(from, to)
	sqls := append([]string{}, contractSQL...)
	sqls = append(sqls, accountSQL...)
	logs := append([]string{}, contractLogs...)
	logs = append(logs, accountLogs...)
	if len(accountRows) > 0 || accountTotal != 0 {
		return buildAccountExpenseBreakdownResult(displayPeriod(from, to), from, to, accountRows, accountTotal, sqls, logs, "说明：合同/项目口径没有可用记录，已回退到账务科目口径。", cfg)
	}

	cashRows, cashTotal, cashSQL, cashLogs := e.collectCashCategoryExpenseBreakdown(from, to)
	sqls = append(sqls, cashSQL...)
	logs = append(logs, cashLogs...)
	if len(cashRows) > 0 || cashTotal != 0 {
		return buildCashExpenseBreakdownResult(displayPeriod(from, to), from, to, cashRows, cashTotal, sqls, logs, "说明：合同/项目和账务科目口径没有可用记录，已回退到现金流水口径。", cfg)
	}
	return buildContractProjectExpenseBreakdownResult(displayPeriod(from, to), from, to, contractRows, contractTotal, contractPaid, sqls, logs, "说明：合同/项目、账务科目和现金流水口径均未找到可用支出记录。", cfg)
}

func (e *Engine) queryCashExpenseBreakdown(from, to, fallbackNote string) Result {
	rows, total, sqls, logs := e.collectCashCategoryExpenseBreakdown(from, to)
	return buildCashExpenseBreakdownResult(displayPeriod(from, to), from, to, rows, total, sqls, logs, fallbackNote, e.currentRuleConfig())
}

func (e *Engine) queryAccountExpenseBreakdown(from, to, fallbackNote string) Result {
	rows, total, sqls, logs := e.collectAccountCategoryExpenseBreakdown(from, to)
	return buildAccountExpenseBreakdownResult(displayPeriod(from, to), from, to, rows, total, sqls, logs, fallbackNote, e.currentRuleConfig())
}

func (e *Engine) queryExpenseBreakdownAllPerspectives(from, to string) Result {
	periodLabel := displayPeriod(from, to)
	cfg := e.currentRuleConfig()
	contractView := cfg.ExpenseBreakdownView("contract_project")
	cashView := cfg.ExpenseBreakdownView("cash_category")
	accountView := cfg.ExpenseBreakdownView("account_category")
	contractRows, contractTotal, contractPaid, contractSQL, contractLogs := e.collectContractProjectExpenseBreakdown(from, to)
	cashRows, cashTotal, cashSQL, cashLogs := e.collectCashCategoryExpenseBreakdown(from, to)
	accountRows, accountTotal, accountSQL, accountLogs := e.collectAccountCategoryExpenseBreakdown(from, to)

	views := map[string]any{
		"contract_project": map[string]any{
			"label":       contractView.Label,
			"description": contractView.Description,
			"total":       round2(contractTotal),
			"paid_total":  round2(contractPaid),
			"rows":        contractProjectRowsToMaps(contractRows, contractTotal),
		},
		"cash_category": map[string]any{
			"label":       cashView.Label,
			"description": cashView.Description,
			"total":       round2(cashTotal),
			"rows":        categoryRowsToMaps(cashRows, cashTotal),
		},
		"account_category": map[string]any{
			"label":       accountView.Label,
			"description": accountView.Description,
			"total":       round2(accountTotal),
			"rows":        accountRowsToMaps(accountRows, accountTotal),
		},
	}

	message := composeExpenseBreakdownMessage(periodLabel, contractRows, contractTotal, contractPaid, cashRows, cashTotal, accountRows, accountTotal, cfg)
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
			"metric":                   cfg.ExpenseBreakdownMetricName(),
			"breakdown_views":          views,
			"source_primary_tables":    []string{"fin_cost_settlements", "fin_bank_statement", "fin_journal"},
			"source_supporting_tables": []string{"fin_contracts"},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func buildContractProjectExpenseBreakdownResult(periodLabel, from, to string, rows []expenseBreakdownRow, total, paidTotal float64, sqls, logs []string, note string, cfg RuleConfig) Result {
	contractView := cfg.ExpenseBreakdownView("contract_project")
	return Result{
		Success:      true,
		Message:      composeContractProjectExpenseBreakdownMessage(periodLabel, rows, total, paidTotal, note, cfg),
		AnswerMethod: "sql",
		Data: map[string]any{
			"period":      periodLabel,
			"period_from": from,
			"period_to":   to,
			"metric":      cfg.ExpenseBreakdownMetricName(),
			"breakdown_views": map[string]any{
				"contract_project": map[string]any{
					"label":       contractView.Label,
					"description": contractView.Description,
					"total":       round2(total),
					"paid_total":  round2(paidTotal),
					"rows":        contractProjectRowsToMaps(rows, total),
				},
			},
			"source_primary_tables":    []string{"fin_cost_settlements", "fin_cost_settlement_groups"},
			"source_supporting_tables": []string{"fin_contracts"},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func buildCashExpenseBreakdownResult(periodLabel, from, to string, rows []expenseBreakdownRow, total float64, sqls, logs []string, note string, cfg RuleConfig) Result {
	cashView := cfg.ExpenseBreakdownView("cash_category")
	return Result{
		Success:      true,
		Message:      composeCashExpenseBreakdownMessage(periodLabel, rows, total, note, cfg),
		AnswerMethod: "sql",
		Data: map[string]any{
			"period":      periodLabel,
			"period_from": from,
			"period_to":   to,
			"metric":      cfg.ExpenseBreakdownMetricName(),
			"breakdown_views": map[string]any{
				"cash_category": map[string]any{
					"label":       cashView.Label,
					"description": cashView.Description,
					"total":       round2(total),
					"rows":        categoryRowsToMaps(rows, total),
				},
			},
			"source_primary_tables": []string{"fin_bank_statement"},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func buildAccountExpenseBreakdownResult(periodLabel, from, to string, rows []expenseBreakdownRow, total float64, sqls, logs []string, note string, cfg RuleConfig) Result {
	accountView := cfg.ExpenseBreakdownView("account_category")
	return Result{
		Success:      true,
		Message:      composeAccountExpenseBreakdownMessage(periodLabel, rows, total, note, cfg),
		AnswerMethod: "sql",
		Data: map[string]any{
			"period":      periodLabel,
			"period_from": from,
			"period_to":   to,
			"metric":      cfg.ExpenseBreakdownMetricName(),
			"breakdown_views": map[string]any{
				"account_category": map[string]any{
					"label":       accountView.Label,
					"description": accountView.Description,
					"total":       round2(total),
					"rows":        accountRowsToMaps(rows, total),
				},
			},
			"source_primary_tables": []string{"fin_journal"},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func (e *Engine) collectContractProjectExpenseBreakdown(from, to string) ([]expenseBreakdownRow, float64, float64, []string, []string) {
	directSQL := `
SELECT c.customer_name,
       c.contract_content,
       COALESCE(cs.settlement_amount, 0) AS settlement_amount,
       COALESCE(cs.paid_amount, 0) AS paid_amount,
       COALESCE(cs.invoice_amount, 0) AS invoice_amount
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?`
	args := []any{from, to}
	unionSQL := directSQL
	executed := []string{"expense_breakdown(contract_project): SELECT customer_name, contract_content, SUM(settlement_amount), SUM(paid_amount), SUM(invoice_amount) FROM fin_cost_settlements + fin_cost_settlement_groups ... WHERE year_month BETWEEN ? AND ? GROUP BY customer_name, contract_content"}
	if e.hasCostSettlementGroupTables() {
		unionSQL += `
UNION ALL
SELECT g.customer_name,
       CASE
         WHEN COALESCE(TRIM(g.merge_range), '') <> '' THEN '合并金额组 ' || g.merge_range
         ELSE '合并金额组'
       END AS contract_content,
       COALESCE(g.settlement_amount, 0) AS settlement_amount,
       COALESCE(g.paid_amount, 0) AS paid_amount,
       COALESCE(g.invoice_amount, 0) AS invoice_amount
FROM fin_cost_settlement_groups g
WHERE g.year_month BETWEEN ? AND ?`
		args = append(args, from, to)
	}
	sqlText := `
SELECT customer_name,
       contract_content,
       COALESCE(SUM(settlement_amount), 0),
       COALESCE(SUM(paid_amount), 0),
       COALESCE(SUM(invoice_amount), 0)
FROM (` + unionSQL + `) contract_project_expense
GROUP BY customer_name, contract_content
HAVING COALESCE(SUM(settlement_amount), 0) <> 0
    OR COALESCE(SUM(paid_amount), 0) <> 0
ORDER BY 3 DESC, customer_name, contract_content`
	rows, err := e.db.Query(sqlText, args...)
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
	cfg := e.currentRuleConfig()
	for rows.Next() {
		var accountCode, accountName, summary string
		var amount float64
		if err := rows.Scan(&accountCode, &accountName, &summary, &amount); err != nil {
			continue
		}
		amount = round2(amount)
		category := classifyAccountExpenseCategoryWithConfig(accountCode, accountName, summary, cfg)
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
	cfg := e.currentRuleConfig()
	text := normalizeEntityText(counterparty + " " + summary)
	for _, rule := range cfg.ExpenseBreakdownCashCategoryRules() {
		if e.matchesCashExpenseCategory(rule, counterparty, text, cfg) {
			return rule.Category
		}
	}
	return cfg.ExpenseBreakdownCashDefaultCategoryName()
}

func classifyAccountExpenseCategory(accountCode, accountName, summary string) string {
	return classifyAccountExpenseCategoryWithConfig(accountCode, accountName, summary, getRuleConfig())
}

func classifyAccountExpenseCategoryWithConfig(accountCode, accountName, summary string, cfg RuleConfig) string {
	text := normalizeEntityText(accountCode + " " + accountName + " " + summary)
	for _, rule := range cfg.ExpenseBreakdownAccountCategoryRules() {
		if matchesAccountExpenseCategory(rule, accountCode, text, cfg) {
			return rule.Category
		}
	}
	if category := cfg.ExpenseBreakdownAccountDefaultCategoryName(); category != "" {
		return category
	}
	name := strings.TrimSpace(accountName)
	if name == "" {
		return "其他费用"
	}
	return name
}

func (e *Engine) matchesCashExpenseCategory(rule ExpenseBreakdownCategoryRule, counterparty, text string, cfg RuleConfig) bool {
	if containsAny(text, rule.Keywords) {
		return true
	}
	if role := CounterpartyRole(strings.TrimSpace(rule.CounterpartyRole)); role != "" && containsAny(text, cfg.CounterpartyRoleKeywords(role)) {
		return true
	}
	if rule.InternalParty && (internalPartyMatchesCompany(e.Company, counterparty) || looksLikeInternalOrgUnit(counterparty, cfg)) {
		return true
	}
	return rule.ExternalOrganization && looksLikeExternalOrganizationCounterparty(counterparty)
}

func matchesAccountExpenseCategory(rule ExpenseBreakdownCategoryRule, accountCode, text string, cfg RuleConfig) bool {
	if containsAny(text, rule.Keywords) {
		return true
	}
	if hasAnyPrefix(strings.TrimSpace(accountCode), rule.AccountCodePrefixes) {
		return true
	}
	if role := CounterpartyRole(strings.TrimSpace(rule.CounterpartyRole)); role != "" && containsAny(text, cfg.CounterpartyRoleKeywords(role)) {
		return true
	}
	return false
}

func hasAnyPrefix(value string, prefixes []string) bool {
	value = strings.TrimSpace(value)
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, strings.TrimSpace(prefix)) {
			return true
		}
	}
	return false
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

func composeExpenseBreakdownMessage(period string, contractRows []expenseBreakdownRow, contractTotal, contractPaid float64, cashRows []expenseBreakdownRow, cashTotal float64, accountRows []expenseBreakdownRow, accountTotal float64, cfg RuleConfig) string {
	contractView := cfg.ExpenseBreakdownView("contract_project")
	cashView := cfg.ExpenseBreakdownView("cash_category")
	accountView := cfg.ExpenseBreakdownView("account_category")
	return strings.Join([]string{
		fmt.Sprintf("%s %s已按所有可用口径拆开：", period, cfg.ExpenseBreakdownMetricName()),
		fmt.Sprintf("1. %s：项目成本 %.2f 元，项目付款 %.2f 元。主要项目：%s。", contractView.Label, round2(contractTotal), round2(contractPaid), summarizeContractProjectRows(contractRows, contractView.SummaryLimit)),
		fmt.Sprintf("2. %s：银行实际流出 %.2f 元。大类：%s。", cashView.Label, round2(cashTotal), summarizeCategoryRows(cashRows, cashView.SummaryLimit)),
		fmt.Sprintf("3. %s：账上成本及费用 %.2f 元。科目：%s。", accountView.Label, round2(accountTotal), summarizeCategoryRows(accountRows, accountView.SummaryLimit)),
		"说明：三种口径分别看项目成本确认、银行实际付款、账务入账成本费用，金额不要求相加一致。",
	}, "\n")
}

func composeContractProjectExpenseBreakdownMessage(period string, rows []expenseBreakdownRow, total, paidTotal float64, note string, cfg RuleConfig) string {
	contractView := cfg.ExpenseBreakdownView("contract_project")
	parts := []string{
		fmt.Sprintf("%s %s按%s为 %.2f 元，合同付款 %.2f 元。", period, cfg.ExpenseBreakdownMetricName(), contractView.Label, round2(total), round2(paidTotal)),
		fmt.Sprintf("主要项目：%s。", summarizeContractProjectRows(rows, contractView.SummaryLimit)),
	}
	if strings.TrimSpace(note) != "" {
		parts = append(parts, strings.TrimSpace(note))
	}
	return strings.Join(parts, "\n")
}

func composeCashExpenseBreakdownMessage(period string, rows []expenseBreakdownRow, total float64, note string, cfg RuleConfig) string {
	cashView := cfg.ExpenseBreakdownView("cash_category")
	parts := []string{
		fmt.Sprintf("%s %s按%s为 %.2f 元。", period, cfg.ExpenseBreakdownMetricName(), cashView.Label, round2(total)),
		fmt.Sprintf("大类：%s。", summarizeCategoryRows(rows, cashView.SummaryLimit)),
	}
	if strings.TrimSpace(note) != "" {
		parts = append(parts, strings.TrimSpace(note))
	}
	return strings.Join(parts, "\n")
}

func composeAccountExpenseBreakdownMessage(period string, rows []expenseBreakdownRow, total float64, note string, cfg RuleConfig) string {
	accountView := cfg.ExpenseBreakdownView("account_category")
	parts := []string{
		fmt.Sprintf("%s %s按%s为 %.2f 元。", period, cfg.ExpenseBreakdownMetricName(), accountView.Label, round2(total)),
		fmt.Sprintf("科目：%s。", summarizeCategoryRows(rows, accountView.SummaryLimit)),
	}
	if strings.TrimSpace(note) != "" {
		parts = append(parts, strings.TrimSpace(note))
	}
	return strings.Join(parts, "\n")
}

func summarizeContractProjectRows(rows []expenseBreakdownRow, limit int) string {
	if len(rows) == 0 {
		return "暂无合同/项目成本记录"
	}
	if limit <= 0 {
		limit = len(rows)
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
	if limit <= 0 {
		limit = len(rows)
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
