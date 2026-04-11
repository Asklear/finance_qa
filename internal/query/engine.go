package query

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
	"financeqa/internal/config"
)

type Result struct {
	Success bool           `json:"success"`
	Data    map[string]any `json:"data"`
	Message string         `json:"message"`
	SQL     string         `json:"sql"`
}

type Engine struct {
	db     *sql.DB
	dbPath string
	Company string
	calc   *accounting.Calculator
}

func NewEngine(dbPath, company string) (*Engine, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	available, err := availableCompanies(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Engine{
		db:      db,
		dbPath:  dbPath,
		Company: ResolveCompany(company, available),
		calc:    accounting.NewCalculator(db),
	}, nil
}

func (e *Engine) Close() error {
	if e.db == nil {
		return nil
	}
	return e.db.Close()
}

func (e *Engine) Query(question string) Result {
	question = NormalizeQuestion(question)
	from, to := ExtractPeriodWithNow(question, time.Now())
	intent := ClassifyIntent(question)
	var result Result
	switch intent {
	case IntentTaxQuery:
		result = e.queryTax(from, to)
	case IntentARAPQuery:
		result = e.queryARAP(question, to)
	case IntentAnalysis:
		result = e.queryAnalysis(to)
	case IntentMonthlySummary:
		result = e.queryMonthlySummary(question, from, to)
	case IntentFallback:
		result = e.queryFallback(question, from, to, "")
	default:
		result = e.queryPrecise(question, to)
	}

	if result.Success {
		return result
	}
	return e.queryFallback(question, from, to, result.Message)
}

func (e *Engine) queryPrecise(question, period string) Result {
	accountName, err := e.findMatchingAccount(question, period)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}

	startDate := period + "-01"
	endDate := monthEndDay(period)

	// 1. 获取余额 (balance_sheet) - 对于损益科目可能不存在，不存在则为 null (nil)
	var opening, closing *float64
	e.db.QueryRow(`
SELECT opening_balance, closing_balance
FROM balance_sheet
WHERE company = ? AND period = ? AND account_name = ?
LIMIT 1`, e.Company, period, accountName).Scan(&opening, &closing)

	// 2. 获取期间发生流水 (journal) - 对于仅有静态余额无流水的科目可能为 null
	var debit, credit *float64
	e.db.QueryRow(`
SELECT SUM(debit_amount), SUM(credit_amount)
FROM journal
WHERE company = ? AND DATE(voucher_date) >= DATE(?) AND DATE(voucher_date) <= DATE(?) AND account_name = ?
`, e.Company, startDate, endDate, accountName).Scan(&debit, &credit)

	data := map[string]any{
		"period":       period,
		"account_name": accountName,
	}

	// 智能组装全景数据：让宿主 LLM 根据问题意图自己挑数据
	// 如果老板问"余额" -> LLM 取 closing_balance
	// 如果老板问"本月发生了多少" -> LLM 取 period_debit_flow 或 period_credit_flow
	if opening != nil {
		data["opening_balance"] = *opening
	}
	if closing != nil {
		data["closing_balance"] = *closing
	}
	if debit != nil {
		data["period_debit_flow"] = *debit
	}
	if credit != nil {
		data["period_credit_flow"] = *credit
	}

	// 如果这个科目既没有余额也没有流水（极端情况），回退处理
	if len(data) == 2 {
		return Result{Success: false, Message: fmt.Sprintf("%s 查无 %s 的任何余额与流水数据", period, accountName)}
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 综合账务（余额+发生额）查询成功", period, accountName),
		Data:    data,
		SQL:     "union precise balance and flow",
	}
}

func (e *Engine) queryMonthlySummary(question, from, to string) Result {
	// Parse year/month from period string
	year, month := parsePeriod(to)

	// 口径一：钱（银行现金流）
	cash, cashErr := e.calc.ComputeCashFlow(e.Company, from, to)

	// 口径二：帐（权责发生制，从利润表/序时帐）
	var accrual *accounting.AccrualPerspective
	var accrualErr error

	// For monthly profit queries: check if we need single-month or cumulative
	needsSingleMonth := from == to // same month means single month query

	if needsSingleMonth && year > 0 && month > 0 {
		metrics, err := e.calc.ComputeMonthlyFromJournal(e.Company, year, month)
		if err == nil {
			accrual = &accounting.AccrualPerspective{
				Description: fmt.Sprintf("%d年%d月 权责发生制（从序时帐计算）", year, month),
				Revenue:     metrics.Revenue,
				TotalCost:   metrics.Cost,
				Profit:      metrics.Profit,
			}
		} else {
			accrualErr = err
		}
	} else {
		// Cumulative or fallback
		is, err := e.calc.ComputeIncomeStatement(e.Company, year, month)
		if err == nil {
			accrual = &accounting.AccrualPerspective{
				Description: fmt.Sprintf("%d年1-%d月累计 权责发生制", year, month),
				Revenue:     is.Revenue,
				TotalCost:   is.Cost + is.TaxSurcharge + is.SellingExpense + is.AdminExpense + is.FinanceExpense,
				Profit:      is.NetProfit,
			}
		} else {
			accrualErr = err
		}
	}

	data := map[string]any{"period": to}

	// Build dual-perspective response
	if cashErr == nil && cash != nil {
		data["业务现金流口径(看钱)"] = map[string]any{
			"说明": cash.Description,
			"现金流入": cash.Income,
			"现金流出": cash.Expense,
			"净现金流": cash.Net,
		}
	}
	if accrualErr == nil && accrual != nil {
		data["财务做账口径(看利润)"] = map[string]any{
			"说明":     accrual.Description,
			"营业收入":   accrual.Revenue,
			"营业总成本": accrual.TotalCost,
			"账面利润":     accrual.Profit,
		}
	}

	// Also include specific metrics if asked
	switch {
	case strings.Contains(question, "收入"):
		if accrual != nil {
			data["营业收入"] = accrual.Revenue
		}
		if cash != nil {
			data["现金流入"] = cash.Income
		}
	case strings.Contains(question, "支出") || strings.Contains(question, "费用") || strings.Contains(question, "成本"):
		if accrual != nil {
			data["营业总成本"] = accrual.TotalCost
		}
		if cash != nil {
			data["现金流出"] = cash.Expense
		}
	case strings.Contains(question, "利润"):
		if accrual != nil {
			data["账面利润"] = accrual.Profit
		}
		if cash != nil {
			data["净现金流"] = cash.Net
		}
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 月度汇总查询成功（双口径对比）", to),
		Data:    data,
		SQL:     "dual_perspective monthly summary",
	}
}

func parsePeriod(period string) (int, int) {
	parts := strings.Split(period, "-")
	if len(parts) != 2 {
		return 0, 0
	}
	year, _ := strconv.Atoi(parts[0])
	month, _ := strconv.Atoi(parts[1])
	return year, month
}

func (e *Engine) queryTax(from, to string) Result {
	startDate := from + "-01"
	endDate := monthEndDay(to)

	row := e.db.QueryRow(`
SELECT
  COALESCE(SUM(CASE WHEN account_name LIKE '%销项%' OR account_code LIKE '222101%' THEN COALESCE(credit_amount, 0) ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN account_name LIKE '%进项%' OR account_code LIKE '222102%' THEN COALESCE(debit_amount, 0) ELSE 0 END), 0)
FROM journal
WHERE company = ?
  AND DATE(voucher_date) >= DATE(?)
  AND DATE(voucher_date) <= DATE(?)
`, e.Company, startDate, endDate)

	var outputVAT, inputVAT float64
	if err := row.Scan(&outputVAT, &inputVAT); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("query tax: %v", err)}
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 税务查询成功", to),
		Data: map[string]any{
			"period_start": from,
			"period_end":   to,
			"total_output": outputVAT,
			"total_input":  inputVAT,
			"net_vat":      outputVAT - inputVAT,
		},
		SQL: "journal tax summary",
	}
}

func (e *Engine) queryARAP(question, period string) Result {
	accountName := "应收账款"
	prefix := "1122"
	if strings.Contains(question, "应付") {
		accountName = "应付账款"
		prefix = "2202"
	}

	// 1. 获取全盘汇总 Total
	row := e.db.QueryRow(`
SELECT COALESCE(SUM(COALESCE(closing_balance, 0)), 0)
FROM balance_sheet
WHERE company = ?
  AND period = ?
  AND account_code LIKE ?
`, e.Company, period, prefix+"%")

	var total float64
	if err := row.Scan(&total); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("query ar/ap total: %v", err)}
	}

	// 2. 获取排名前十挂账明细 Details
	rows, err := e.db.Query(`
SELECT account_name, closing_balance
FROM balance_sheet
WHERE company = ?
  AND period = ?
  AND account_code LIKE ?
  AND closing_balance <> 0
ORDER BY ABS(closing_balance) DESC
LIMIT 10
`, e.Company, period, prefix+"%")

	var details []map[string]any
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var bal float64
			if err := rows.Scan(&name, &bal); err == nil {
				details = append(details, map[string]any{"name": name, "balance": bal})
			}
		}
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s查询成功", period, accountName),
		Data: map[string]any{
			"period":  period,
			"type":    accountName,
			"total":   total,
			"details": details,
		},
		SQL: "balance_sheet ar/ap details by code",
	}
}

func (e *Engine) queryAnalysis(period string) Result {
	aging := analysis.NewAgingEngine(e.dbPath)
	defer aging.Close()

	summary, err := aging.AnalyzeSummary(e.Company, period)
	if err != nil {
		return Result{Success: false, Message: fmt.Sprintf("analysis query failed: %v", err)}
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 账龄分析成功", period),
		Data: map[string]any{
			"company":            summary.Company,
			"period":             summary.Period,
			"receivable_total":   summary.ReceivableTotal,
			"payable_total":      summary.PayableTotal,
			"health_score":       summary.HealthScore,
			"receivable_buckets": summary.ReceivableBuckets,
			"payable_buckets":    summary.PayableBuckets,
		},
		SQL: "journal aging analysis",
	}
}

func (e *Engine) findMatchingAccount(question, period string) (string, error) {
	startDate := period + "-01"
	endDate := monthEndDay(period)

	// 双擎共扫：同时从余额表和序时帐中提取可能被命中的科目名
	rows, err := e.db.Query(`
SELECT DISTINCT name FROM (
  SELECT account_name AS name FROM balance_sheet WHERE company = ? AND period = ?
  UNION
  SELECT account_name AS name FROM journal WHERE company = ? AND DATE(voucher_date) >= DATE(?) AND DATE(voucher_date) <= DATE(?)
) WHERE name IS NOT NULL AND name <> ''
ORDER BY LENGTH(name) DESC
`, e.Company, period, e.Company, startDate, endDate)
	if err != nil {
		return "", fmt.Errorf("load account names: %w", err)
	}
	defer rows.Close()

	var accounts []string
	for rows.Next() {
		var account string
		if err := rows.Scan(&account); err != nil {
			return "", fmt.Errorf("scan account name: %w", err)
		}
		accounts = append(accounts, account)
	}
	
	// 1. 精确包涵查找
	for _, account := range accounts {
		if strings.Contains(question, account) {
			return account, nil
		}
	}

	// 2. 别名查找映射
	aliases := accountAliases()
	for alias, accountName := range aliases {
		if strings.Contains(question, alias) {
			for _, existing := range accounts {
				if strings.Contains(existing, accountName) || strings.Contains(accountName, existing) {
					return existing, nil
				}
			}
			return accountName, nil
		}
	}
	return "", fmt.Errorf("no matching account found for question %q", question)
}

func (e *Engine) queryFallback(question, from, to, priorErr string) Result {
	if r := e.ruleFallback(question, from, to); r.Success {
		return r
	}
	// 不再调用外部 LLM API，而是返回结构化提示，
	// 让宿主 LLM（OpenClaw / Claude Code）自行理解后重新构造查询。
	return e.buildFallbackHint(question, from, to, priorErr)
}

func (e *Engine) ruleFallback(question, from, to string) Result {
	q := strings.TrimSpace(question)
	switch {
	case containsAny(q, []string{"供应商多少", "客户多少", "项目多少", "有多少供应商", "有多少客户"}):
		return e.queryEntityCountFallback(q, from, to)
	case containsAny(q, []string{"人力成本", "人工成本", "薪酬", "工资"}):
		return e.queryAccountAliasBalance(q, to)
	case containsAny(q, []string{"总成本", "整体成本"}):
		return e.queryTotalCostFallback(to)
	case containsAny(q, []string{"健康度", "财务健康"}):
		return e.queryAnalysis(to)
	case containsAny(q, []string{"数据出来了吗", "有数据吗", "有没有数据"}):
		return e.queryAvailabilityFallback(q, to)
	case containsAny(q, []string{"客户", "供应商", "销售额", "收款", "付款", "收入", "支出"}) && hasNamedEntity(q):
		return e.queryCounterpartyAmountFallback(q, from, to)
	default:
		return Result{Success: false, Message: "no rule fallback matched"}
	}
}

func (e *Engine) queryEntityCountFallback(question, from, to string) Result {
	entityType := "counterparty"
	if strings.Contains(question, "供应商") {
		entityType = "supplier"
	}
	if strings.Contains(question, "客户") {
		entityType = "customer"
	}
	if strings.Contains(question, "项目") {
		entityType = "project"
	}

	var count int64
	if entityType == "project" {
		query := `SELECT COUNT(DISTINCT counterparty_name) FROM bank_statement WHERE company = ? AND counterparty_name IS NOT NULL AND counterparty_name <> ''`
		args := []any{e.Company}
		if from != "" && to != "" {
			query += ` AND DATE(transaction_date) >= DATE(?) AND DATE(transaction_date) <= DATE(?)`
			args = append(args, from+"-01", monthEndDay(to))
		}
		
		row := e.db.QueryRow(query, args...)
		if err := row.Scan(&count); err != nil {
			return Result{Success: false, Message: fmt.Sprintf("query project-like entities: %v", err)}
		}
		return Result{
			Success: true,
			Message: fmt.Sprintf("实体统计成功"),
			Data: map[string]any{
				"entity_type": entityType,
				"count":       count,
			},
			SQL: "bank_statement distinct counterparty",
		}
	}

	row := e.db.QueryRow(`SELECT COUNT(1) FROM entities WHERE entity_type = ?`, entityType)
	if err := row.Scan(&count); err != nil || count == 0 {
		query := `SELECT COUNT(DISTINCT counterparty_name) FROM bank_statement WHERE company = ? AND counterparty_name IS NOT NULL AND counterparty_name <> ''`
		args := []any{e.Company}
		if from != "" && to != "" {
			query += ` AND DATE(transaction_date) >= DATE(?) AND DATE(transaction_date) <= DATE(?)`
			args = append(args, from+"-01", monthEndDay(to))
		}
		row2 := e.db.QueryRow(query, args...)
		if err2 := row2.Scan(&count); err2 != nil {
			return Result{Success: false, Message: fmt.Sprintf("query entity count fallback: %v", err2)}
		}
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 实体统计成功", entityType),
		Data: map[string]any{
			"entity_type": entityType,
			"count":       count,
			"period_from": from,
			"period_to":   to,
		},
		SQL: "entities count",
	}
}

func (e *Engine) queryAccountAliasBalance(question, period string) Result {
	aliases := accountAliases()
	target := "应付职工薪酬"
	for alias, account := range aliases {
		if strings.Contains(question, alias) {
			target = account
			break
		}
	}

	row := e.db.QueryRow(`
SELECT COALESCE(SUM(COALESCE(closing_balance, 0)), 0)
FROM balance_sheet
WHERE company = ?
  AND period = ?
  AND account_name LIKE ?
`, e.Company, period, "%"+target+"%")

	var total float64
	if err := row.Scan(&total); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("query alias balance: %v", err)}
	}
	
	var history []map[string]any
	if !strings.Contains(question, period) && !strings.Contains(question, "月") {
		rows, err := e.db.Query(`
		SELECT period, COALESCE(SUM(COALESCE(closing_balance, 0)), 0)
		FROM balance_sheet
		WHERE company = ?
		  AND account_name LIKE ?
		GROUP BY period ORDER BY period`, e.Company, "%"+target+"%")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var p string
				var am float64
				rows.Scan(&p, &am)
				history = append(history, map[string]any{"period": p, "amount": am})
			}
		}
	}

	data := map[string]any{
		"period": period,
		"account": target,
		"total": total,
	}
	if len(history) > 0 {
		data["history"] = history
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s查询成功", period, target),
		Data:    data,
		SQL: "balance_sheet alias account",
	}
}

func (e *Engine) queryTotalCostFallback(period string) Result {
	row := e.db.QueryRow(`
SELECT COALESCE(SUM(COALESCE(current_amount, 0)), 0)
FROM income_statement
WHERE company = ?
  AND period = ?
  AND item_name LIKE '%成本%'
`, e.Company, period)

	var total float64
	if err := row.Scan(&total); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("query total cost: %v", err)}
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 总成本查询成功", period),
		Data: map[string]any{
			"period": period,
			"total_cost": total,
		},
		SQL: "income_statement total cost",
	}
}

func (e *Engine) queryAvailabilityFallback(question, period string) Result {
	entity := extractNamedEntity(question)
	startDate := period + "-01"
	endDate := monthEndDay(period)
	var n int64

	sqlText := `
SELECT COUNT(1)
FROM bank_statement
WHERE company = ?
  AND DATE(transaction_date) >= DATE(?)
  AND DATE(transaction_date) <= DATE(?)
`
	args := []any{e.Company, startDate, endDate}
	if entity != "" {
		sqlText += "  AND counterparty_name LIKE ?"
		args = append(args, "%"+entity+"%")
	}
	row := e.db.QueryRow(sqlText, args...)
	if err := row.Scan(&n); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("query availability: %v", err)}
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 数据可用性检查完成", period),
		Data: map[string]any{
			"period": period,
			"entity": entity,
			"available": n > 0,
			"rows": n,
		},
		SQL: "bank_statement availability",
	}
}

func (e *Engine) queryCounterpartyAmountFallback(question, from, to string) Result {
	entity := extractNamedEntity(question)
	if entity == "" {
		return Result{Success: false, Message: "no named counterparty found"}
	}
	metric := "income"
	field := "credit_amount"
	if containsAny(question, []string{"支出", "付款", "成本"}) {
		metric = "expense"
		field = "debit_amount"
	}
	if strings.Contains(question, "今年") {
		year := from[:4]
		from = year + "-01"
		to = year + "-12"
	}

	row := e.db.QueryRow(fmt.Sprintf(`
SELECT COALESCE(SUM(COALESCE(%s, 0)), 0)
FROM bank_statement
WHERE company = ?
  AND DATE(transaction_date) >= DATE(?)
  AND DATE(transaction_date) <= DATE(?)
  AND counterparty_name LIKE ?
`, field), e.Company, from+"-01", monthEndDay(to), "%"+entity+"%")

	var total float64
	if err := row.Scan(&total); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("query counterparty amount: %v", err)}
	}
	
	var history []map[string]any
	if !strings.Contains(question, from) && !strings.Contains(question, "月") {
		rows, err := e.db.Query(fmt.Sprintf(`
		SELECT strftime('%%Y-%%m', transaction_date) as period, COALESCE(SUM(COALESCE(%s, 0)), 0)
		FROM bank_statement
		WHERE company = ?
		  AND DATE(transaction_date) >= DATE(?)
		  AND DATE(transaction_date) <= DATE(?)
		  AND counterparty_name LIKE ?
		GROUP BY period ORDER BY period`, field), e.Company, from+"-01", monthEndDay(to), "%"+entity+"%")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var p string
				var am float64
				rows.Scan(&p, &am)
				history = append(history, map[string]any{"period": p, "amount": am})
			}
		}
	}

	data := map[string]any{
		"entity": entity,
		"metric": metric,
		"period_from": from,
		"period_to": to,
		"total": total,
	}
	if len(history) > 0 {
		data["history"] = history
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 金额查询成功", entity, metric),
		Data:    data,
		SQL:     "bank_statement counterparty aggregate",
	}
}

func accountAliases() map[string]string {
	mgr := config.GetKeywordsManager()
	raw := mgr.Get("accounts.aliases", map[string]any{})
	m, ok := raw.(map[string]any)
	if !ok {
		return map[string]string{
			"人力成本": "应付职工薪酬",
			"总成本":  "营业成本",
		}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if sv, ok := v.(string); ok {
			out[k] = sv
		}
	}
	return out
}

var namedEntityPattern = regexp.MustCompile(`([A-Za-z0-9_\-\x{4e00}-\x{9fa5}]{2,})(?:客户|供应商|公司|项目)`)

func extractNamedEntity(question string) string {
	m := namedEntityPattern.FindStringSubmatch(strings.TrimSpace(question))
	if len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func hasNamedEntity(question string) bool {
	return extractNamedEntity(question) != ""
}

func availableCompanies(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
SELECT DISTINCT company FROM balance_sheet
UNION
SELECT DISTINCT company FROM income_statement
UNION
SELECT DISTINCT company FROM bank_statement
UNION
SELECT DISTINCT company FROM journal
`)
	if err != nil {
		return nil, fmt.Errorf("load companies: %w", err)
	}
	defer rows.Close()

	var companies []string
	for rows.Next() {
		var company string
		if err := rows.Scan(&company); err != nil {
			return nil, fmt.Errorf("scan company: %w", err)
		}
		if strings.TrimSpace(company) != "" {
			companies = append(companies, company)
		}
	}
	return companies, rows.Err()
}

func monthEndDay(period string) string {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return period + "-28"
	}
	return t.AddDate(0, 1, -1).Format("2006-01-02")
}
