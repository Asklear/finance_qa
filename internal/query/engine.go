package query

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
	"financeqa/internal/dimensions"
)

type Result struct {
	Success         bool           `json:"success"`
	Data            map[string]any `json:"data"`
	Message         string         `json:"message"`
	AnswerMethod    string         `json:"answer_method,omitempty"`
	ExecutedSQL     []string       `json:"executed_sql"`
	CalculationLogs []string       `json:"calculation_logs"`
}

func (r Result) withTraceData() Result {
	if len(r.ExecutedSQL) == 0 {
		r.ExecutedSQL = []string{"(trace-sql) no explicit SQL captured in this branch"}
	}
	if len(r.CalculationLogs) == 0 {
		r.CalculationLogs = []string{"(trace-log) no explicit calculation logs captured in this branch"}
	}
	if r.Data == nil {
		r.Data = map[string]any{}
	}
	if r.AnswerMethod == "" {
		r.AnswerMethod = "sql"
	}
	r.Data["answer_method"] = r.AnswerMethod
	r.Data["trace"] = map[string]any{
		"executed_sql":     append([]string{}, r.ExecutedSQL...),
		"calculation_logs": append([]string{}, r.CalculationLogs...),
	}
	return r
}

type Engine struct {
	db        *sql.DB
	dbPath    string
	Company   string
	available []string
	calc      *accounting.Calculator
	dim       *dimensions.Manager
}

func NewEngine(dbPath, company string) (*Engine, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	available, _ := availableCompanies(db)
	dimRepo := dimensions.NewSQLiteRepository(db)
	dimMgr := dimensions.NewManager(dimRepo)
	resolvedCompany := ResolveCompany(company, available)
	calc := accounting.NewCalculator(db)
	if resolvedCompany != "" {
		if mapper, err := dimMgr.GetMapper(context.Background(), resolvedCompany); err == nil {
			calc.Mapper = mapper
		}
	}
	return &Engine{db: db, dbPath: dbPath, Company: resolvedCompany, available: available, calc: calc, dim: dimMgr}, nil
}

func (e *Engine) Close() error {
	if e.db == nil {
		return nil
	}
	return e.db.Close()
}

func (e *Engine) getLatestPeriodAnchor() time.Time {
	var maxDate string
	e.db.QueryRow(`SELECT MAX(voucher_date) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`, e.Company, e.Company).Scan(&maxDate)
	if maxDate == "" {
		return time.Now()
	}
	t, _ := time.Parse("2006-01-02", maxDate)
	return t
}

func (e *Engine) Query(question string) Result {
	q := NormalizeQuestion(question)
	resolved := ResolveCompany(q, e.available)
	if resolved != "" && resolved != e.Company {
		e.Company = resolved
	}

	anchor := e.getLatestPeriodAnchor()
	from, to := ExtractPeriodWithNow(q, anchor)
	intent := ClassifyIntent(q)

	entity := e.extractNamedEntity(q)

	var result Result
	if shouldForceDualPerspective(q) && !shouldBypassDualPerspective(q, entity) {
		result = e.queryDualPerspectiveForCoreMetric(q, from, to)
		if result.Success {
			return result.withTraceData()
		}
	}

	switch intent {
	case IntentHostPayload:
		result = e.queryHostLLMPayload(q, from, to)
	case IntentIdentityQuery:
		role, _ := e.detectEntityRole(entity)
		result = Result{
			Success: true,
			Message: fmt.Sprintf("识别结果: [%s] 是 [%s]", entity, role),
			Data:    map[string]any{"entity": entity, "role": role},
			ExecutedSQL: []string{
				"detectEntityRole: SELECT SUM(debit_amount), SUM(credit_amount) FROM bank_statement WHERE counterparty_name LIKE ?",
				"detectEntityRole: SELECT account_code, summary FROM journal WHERE summary LIKE ? OR account_name LIKE ?",
			},
			CalculationLogs: []string{
				fmt.Sprintf("[身份识别] entity=%s role=%s", entity, role),
			},
		}
	case IntentARAPQuery:
		result = e.queryARAP(q, entity, from, to)
	case IntentLargeTransactionQuery:
		result = e.queryLargeBankTransactions(q, from, to)
	case IntentTaxQuery:
		result = e.queryTax(q, from, to)
	case IntentMonthlySummary:
		result = e.queryMonthlySummary(q, from, to)
	case IntentAnalysis:
		result = e.queryAnalysis(to)
	case IntentFallback:
		result = e.queryFallback(q, from, to, "")
	default:
		result = e.queryPrecise(q, to)
	}

	if result.Success {
		return result.withTraceData()
	}

	// 智能分流降级：如果精确查询由于科目未发现而失败，且存在实体，则自动滑入往来款审计
	if entity != "" && (result.Message == "account not found" || !strings.Contains(result.Message, "no named")) {
		return e.queryFallback(q, from, to, result.Message).withTraceData()
	}
	if result.Message == "account not found" || strings.Contains(result.Message, "语义模糊") {
		return e.queryFallback(q, from, to, result.Message).withTraceData()
	}

	if result.Message != "" {
		return result.withTraceData()
	}
	return e.queryFallback(q, from, to, result.Message).withTraceData()
}

func (e *Engine) queryPrecise(question, period string) Result {
	accountName, err := e.findMatchingAccount(question, period)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	startDate, endDate := period+"-01", monthEndDay(period)
	var opening, closing, debit, credit float64
	e.db.QueryRow(`SELECT opening_balance, closing_balance FROM balance_sheet WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ? AND account_name = ?`, e.Company, e.Company, period, accountName).Scan(&opening, &closing)
	e.db.QueryRow(`SELECT SUM(debit_amount), SUM(credit_amount) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND voucher_date BETWEEN ? AND ? AND account_name = ?`, e.Company, e.Company, startDate, endDate, accountName).Scan(&debit, &credit)

	logs := []string{
		fmt.Sprintf("[余额对账] 科目:%s, 期间:%s", accountName, period),
		fmt.Sprintf("[轧账公式] 期初余:%.2f + 借方发生:%.2f - 贷方发生:%.2f = 期末余:%.2f", opening, debit, credit, closing),
	}

	bsSQL := fmt.Sprintf(`SELECT opening_balance, closing_balance FROM balance_sheet WHERE ... AND account_name = '%s'`, accountName)
	jrSQL := fmt.Sprintf(`SELECT SUM(debit_amount), SUM(credit_amount) FROM journal WHERE ... AND account_name = '%s'`, accountName)

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 综合账务余额为 %.2f 元", period, accountName, closing),
		Data: map[string]any{
			"period": period, "account": accountName, "opening": opening, "closing": closing, "debit": debit, "credit": credit,
			// 兼容旧字段
			"opening_balance": opening, "closing_balance": closing, "debit_amount": debit, "credit_amount": credit,
		},
		ExecutedSQL:     []string{bsSQL, jrSQL},
		CalculationLogs: logs,
	}
}

func (e *Engine) queryMonthlySummary(question, from, to string) Result {
	year, month := parsePeriod(to)
	e.calc.ResetTrace()

	// 1. 获取当月精确指标 (用于判断是否有数)
	monthly, _ := e.calc.ComputeMonthlyFromJournal(e.Company, year, month)
	// 2. 获取累计指标 (用于明细展现)
	is, _ := e.calc.ComputeIncomeStatement(e.Company, year, month)
	cash, _ := e.calc.ComputeCashFlow(e.Company, from, to)

	logs := append([]string{}, e.calc.CalculationLogs...)
	sqls := append([]string{}, e.calc.ExecutedSQLs...)

	revenue := monthly.Revenue
	expense := monthly.Cost

	mainMsg := fmt.Sprintf("%s 月度经营分析：当月收入 %.2f, 成本支出 %.2f, 净利润 %.2f", to, revenue, expense, monthly.Profit)

	// 智能回溯：如果本单月数据为空，则统计本年累计数据供参考
	if revenue == 0 && expense == 0 {
		logs = append(logs, fmt.Sprintf("[智能回溯] %s 当月无经营记账，正在为您还原年度累计经营体量...", to))
		if month > 1 {
			mainMsg = fmt.Sprintf("%s 暂无经营数据。2026年1月以来（YTD）累计：收入 %.2f, 支出 %.2f, 累计利润 %.2f", to, is.Revenue, is.Cost, is.NetProfit)
			logs = append(logs, fmt.Sprintf("[审计结论] 虽当月静默，但年度累计体量已达 %.2f 万元", is.Revenue/10000.0))
		} else {
			mainMsg = fmt.Sprintf("%s 暂无经营数据，且为年度首月，无历史数据可回溯", to)
		}
	}

	return Result{
		Success:      true,
		Message:      mainMsg,
		AnswerMethod: "sql",
		Data: map[string]any{
			"monthly": monthly, "cumulative": is, "cash_flow": cash,
			// 兼容旧版本 top-level 字段（测试与外部调用依赖）
			"现金流入": cash.Income, "现金流出": cash.Expense, "净现金流": cash.Net,
			"财务做账口径(看利润)": map[string]any{
				"营业收入":    is.Revenue,
				"营业成本及费用": is.Cost + is.TaxSurcharge + is.SellingExpense + is.AdminExpense + is.FinanceExpense,
				"账面利润":    is.NetProfit,
			},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func (e *Engine) queryDualPerspectiveForCoreMetric(question, from, to string) Result {
	year, month := parsePeriod(to)
	e.calc.ResetTrace()
	dual, err := e.calc.ComputeDualPerspective(e.Company, year, month)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}

	metric := detectCoreMetric(question)
	cashValue, accrualValue := pickMetricValue(metric, dual)
	msg := fmt.Sprintf("%s 双口径%s：钱口径 %.2f 元，账口径 %.2f 元", to, metric, cashValue, accrualValue)

	logs := append([]string{}, e.calc.CalculationLogs...)
	logs = append(logs, fmt.Sprintf("[双口径强制] metric=%s cash=%.2f accrual=%.2f", metric, cashValue, accrualValue))

	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	return Result{
		Success:      true,
		Message:      msg,
		AnswerMethod: "sql",
		Data: map[string]any{
			"period":        to,
			"metric":        metric,
			"money_view":    dual.Cash,
			"account_view":  dual.Accrual,
			"money_value":   cashValue,
			"account_value": accrualValue,
			"现金流入":          dual.Cash.Income,
			"现金流出":          dual.Cash.Expense,
			"净现金流":          dual.Cash.Net,
			"财务做账口径(看利润)": map[string]any{
				"营业收入":    dual.Accrual.Revenue,
				"营业成本及费用": dual.Accrual.TotalCost,
				"账面利润":    dual.Accrual.Profit,
			},
			"dual_perspective": map[string]any{
				"cash":    dual.Cash,
				"accrual": dual.Accrual,
			},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func (e *Engine) queryHostLLMPayload(question, from, to string) Result {
	payload := e.buildHostLLMPayload(from, to, question)
	logs := []string{
		fmt.Sprintf("[宿主LLM数据包] company=%s period=%s~%s", e.Company, from, to),
		"[宿主LLM数据包] 已输出全量财报原始数据（按期间过滤）",
	}
	sqls := []string{
		"host_payload(balance_sheet): SELECT * FROM balance_sheet WHERE ... AND period BETWEEN ? AND ?",
		"host_payload(income_statement): SELECT * FROM income_statement WHERE ... AND period BETWEEN ? AND ?",
		"host_payload(balance_detail): SELECT * FROM balance_detail WHERE ...",
		"host_payload(journal): SELECT * FROM journal WHERE ... AND voucher_date BETWEEN ? AND ?",
		"host_payload(bank_statement): SELECT * FROM bank_statement WHERE ... AND transaction_date BETWEEN ? AND ?",
	}
	return Result{
		Success:      true,
		Message:      "已生成宿主LLM可消费的原始财报数据包",
		AnswerMethod: "llm_payload",
		Data: map[string]any{
			"llm_payload": payload,
			"usage":       "请宿主LLM基于 payload.financial_tables 和 payload.trace 进行最终语义判别与回答",
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func (e *Engine) queryTax(question, from, to string) Result {
	startDate, endDate := from+"-01", monthEndDay(to)
	var output, input float64
	e.db.QueryRow(`SELECT COALESCE(SUM(credit_amount), 0) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND (account_name LIKE '%销项%' OR account_code LIKE '222101%') AND voucher_date BETWEEN ? AND ?`, e.Company, e.Company, startDate, endDate).Scan(&output)
	e.db.QueryRow(`SELECT COALESCE(SUM(debit_amount), 0) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND (account_name LIKE '%进项%' OR account_code LIKE '222101%') AND voucher_date BETWEEN ? AND ?`, e.Company, e.Company, startDate, endDate).Scan(&input)
	// 兼容部分样本使用 222102 记录进项税
	if input == 0 {
		e.db.QueryRow(`SELECT COALESCE(SUM(debit_amount), 0) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND (account_name LIKE '%进项%' OR account_code LIKE '222102%') AND voucher_date BETWEEN ? AND ?`, e.Company, e.Company, startDate, endDate).Scan(&input)
	}

	logs := []string{
		fmt.Sprintf("[税务审计] 销项税额: %.2f (贷方发生)", output),
		fmt.Sprintf("[税务审计] 进项税额: %.2f (借方发生)", input),
		fmt.Sprintf("[计算结果] 当月净应交: %.2f", output-input),
	}

	msg := fmt.Sprintf("%s 税额查询完成：销项 %.2f 元，进项 %.2f 元", to, output, input)
	if strings.Contains(question, "进项") {
		msg = fmt.Sprintf("%s 进项税额查询完成：应计 %.2f 元", to, input)
	} else if strings.Contains(question, "销项") {
		msg = fmt.Sprintf("%s 销项税额查询完成：应计 %.2f 元", to, output)
	}

	return Result{
		Success: true,
		Message: msg,
		Data: map[string]any{
			"output": output, "input": input,
			// 兼容旧字段
			"total_output": output, "total_input": input, "net_vat": output - input,
		},
		ExecutedSQL: []string{
			"queryTax(output): SELECT SUM(credit_amount) FROM journal WHERE ... (account_name LIKE '%销项%' OR account_code LIKE '222101%')",
			"queryTax(input): SELECT SUM(debit_amount) FROM journal WHERE ... (account_name LIKE '%进项%' OR account_code LIKE '222101%')",
		},
		CalculationLogs: logs,
	}
}

func (e *Engine) queryARAP(question, entity, from, to string) Result {
	period := to
	if entity != "" && strings.Contains(question, "项目") {
		return e.queryProjectARAP(entity, from, to)
	}
	if strings.Contains(question, "应收") {
		return e.queryAccountPayableReceivable(period, "应收账款", "1122", "receivable")
	}
	if strings.Contains(question, "应付") {
		return e.queryAccountPayableReceivable(period, "应付账款", "2202", "payable")
	}
	if entity != "" {
		return e.queryCounterpartyAmountFallback(question, entity, from, to)
	}
	return Result{
		Success: false,
		Message: "未识别应收/应付对象",
		CalculationLogs: []string{
			"[AR/AP分流] 问题未命中应收/应付且未识别实体",
		},
	}
}

func (e *Engine) queryAccountPayableReceivable(period, accountName, accountCodePrefix, typ string) Result {
	rows, err := e.db.Query(`
SELECT account_name, closing_balance
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND (account_name LIKE ? OR account_code LIKE ?)
`, e.Company, e.Company, period, accountName+"%", accountCodePrefix+"%")
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	defer rows.Close()

	details := make([]map[string]any, 0)
	total := 0.0
	for rows.Next() {
		var name string
		var closing float64
		if err := rows.Scan(&name, &closing); err != nil {
			continue
		}
		total += closing
		details = append(details, map[string]any{"account": name, "closing_balance": closing})
	}
	if len(details) == 0 {
		return Result{Success: false, Message: "该期间未找到应收/应付余额"}
	}

	msg := fmt.Sprintf("%s %s合计 %.2f 元", period, accountName, total)
	return Result{
		Success: true,
		Message: msg,
		Data: map[string]any{
			"type": typ, "period": period, "total": total, "details": details,
			"account": accountName, "closing": total,
		},
		ExecutedSQL: []string{
			"queryAccountPayableReceivable: SELECT account_name, closing_balance FROM balance_sheet WHERE ... AND (account_name LIKE ? OR account_code LIKE ?)",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[AR/AP汇总] period=%s account=%s total=%.2f detail_count=%d", period, accountName, total, len(details)),
		},
	}
}

func (e *Engine) queryCounterpartyAmountFallback(question, entity, from, to string) Result {
	if entity == "" {
		return Result{Success: false, Message: "no named counterparty found"}
	}
	role, _ := e.detectEntityRole(entity)

	bankSql := `(LENGTH(counterparty_name) > 1 AND (? LIKE '%' || counterparty_name || '%' OR counterparty_name LIKE '%' || ? || '%'))`
	journalSql := `(summary LIKE '%' || ? || '%')`

	var bIn, bOut, jIn, jOut float64
	bankSQL := fmt.Sprintf(`SELECT COALESCE(SUM(credit_amount), 0), COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE %s AND transaction_date BETWEEN '%s' AND '%s'`, bankSql, from+"-01", monthEndDay(to))
	e.db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(credit_amount), 0), COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE %s AND transaction_date BETWEEN ? AND ?`, bankSql), entity, entity, from+"-01", monthEndDay(to)).Scan(&bIn, &bOut)

	jourSQL := fmt.Sprintf(`SELECT COALESCE(SUM(CASE WHEN direction='贷' THEN amount ELSE 0 END), 0), COALESCE(SUM(CASE WHEN direction='借' THEN amount ELSE 0 END), 0) FROM journal WHERE %s AND voucher_date BETWEEN '%s' AND '%s'`, journalSql, from+"-01", monthEndDay(to))
	e.db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(CASE WHEN direction='贷' THEN amount ELSE 0 END), 0), COALESCE(SUM(CASE WHEN direction='借' THEN amount ELSE 0 END), 0) FROM journal WHERE %s AND voucher_date BETWEEN ? AND ?`, journalSql), entity, from+"-01", monthEndDay(to)).Scan(&jIn, &jOut)

	logs := []string{
		fmt.Sprintf("[银行流水审计] 收入(贷):%.2f, 支出(借):%.2f, 净额:%.2f", bIn, bOut, math.Abs(bIn-bOut)),
		fmt.Sprintf("[序时账审计] 贷方(收入/还款):%.2f, 借方(支出/报销):%.2f", jIn, jOut),
	}

	total := math.Max(math.Abs(bIn-bOut), math.Max(jIn, jOut))
	isRetro := false
	if total == 0 || (strings.Contains(question, "一年") || strings.Contains(question, "一共")) {
		isRetro = true
		logs = append(logs, "[策略触发] 由于月度数据不足或触发全量指令，切换至年度回溯审计模式")
		start, end := from[:4]+"-01-01", from[:4]+"-12-31"
		e.db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(credit_amount), 0), COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE %s AND transaction_date BETWEEN ? AND ?`, bankSql), entity, entity, start, end).Scan(&bIn, &bOut)
		e.db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(CASE WHEN direction='贷' THEN amount ELSE 0 END), 0), COALESCE(SUM(CASE WHEN direction='借' THEN amount ELSE 0 END), 0) FROM journal WHERE %s AND voucher_date BETWEEN ? AND ?`, journalSql), entity, start, end).Scan(&jIn, &jOut)
		total = math.Max(math.Abs(bIn-bOut), math.Max(jIn, jOut))
		logs = append(logs, fmt.Sprintf("[年度还原] 银行流转:%.2f, 序时账最大侧:%.2f", math.Abs(bIn-bOut), math.Max(jIn, jOut)))
	}

	total = math.Round(total*100) / 100
	logs = append(logs, fmt.Sprintf("[最终判定] 采用 Max-Abs 算法锁定流转总额: %.2f 元", total))

	if total != 0 {
		return Result{
			Success:         true,
			Message:         fmt.Sprintf("[%s]（识别为[%s]）穿透审计成功，期间流水 %.2f 元", entity, role, total),
			Data:            map[string]any{"entity": entity, "role": role, "amount": total, "total": total, "is_retro": isRetro},
			ExecutedSQL:     []string{bankSQL, jourSQL},
			CalculationLogs: logs,
		}
	}
	return Result{Success: false, Message: fmt.Sprintf("穿透审计失败：[%s] 无发生额", entity)}
}

func (e *Engine) queryLargeBankTransactions(question, from, to string) Result {
	var name string
	var amount float64
	sqlTxt := `SELECT counterparty_name, credit_amount FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND transaction_date BETWEEN ? AND ? ORDER BY credit_amount DESC LIMIT 1`
	e.db.QueryRow(sqlTxt, e.Company, e.Company, from+"-01", monthEndDay(to)).Scan(&name, &amount)
	if name == "" {
		return Result{
			Success: false,
			Message: "未发现大额记录",
			ExecutedSQL: []string{
				fmt.Sprintf("queryLargeBankTransactions: %s [args: %s, %s, %s]", sqlTxt, e.Company, from+"-01", monthEndDay(to)),
			},
		}
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 最大流入对手方为 [%s]，流水 %.2f 元", from, name, amount),
		Data:    map[string]any{"counterparty": name, "amount": amount},
		ExecutedSQL: []string{
			fmt.Sprintf("queryLargeBankTransactions: %s [args: %s, %s, %s]", sqlTxt, e.Company, from+"-01", monthEndDay(to)),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[大额流水] top_counterparty=%s credit=%.2f", name, amount),
		},
	}
}

func (e *Engine) detectEntityRole(name string) (role string, log string) {
	var bankOut, bankIn float64
	e.db.QueryRow(`SELECT COALESCE(SUM(debit_amount), 0), COALESCE(SUM(credit_amount), 0) FROM bank_statement WHERE counterparty_name LIKE ?`, "%"+name+"%").Scan(&bankOut, &bankIn)
	var hasSalary bool
	var employeeSignals, total int
	rows, _ := e.db.Query(`SELECT account_code, summary FROM journal WHERE summary LIKE ? OR account_name LIKE ?`, "%"+name+"%", "%"+name+"%")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var code, summary string
			rows.Scan(&code, &summary)
			total++
			if strings.HasPrefix(code, "2211") {
				hasSalary = true
			}
			if strings.Contains(summary, "报销") || strings.Contains(summary, "工资") {
				employeeSignals++
			}
		}
	}
	switch {
	case hasSalary || (total > 0 && float64(employeeSignals)/float64(total) > 0.3):
		return "employee", "employee"
	case bankIn > bankOut*2 && bankIn > 0:
		return "customer", "customer"
	case bankOut > bankIn*2 && bankOut > 0:
		return "supplier", "supplier"
	default:
		return "unknown", "unknown"
	}
}

var namedEntityPattern = regexp.MustCompile(`([A-Za-z0-9_\-\(\)（）\x{4e00}-\x{9fa5}]{2,})(?:客户|供应商|公司|项目|单位|人|报销|报账|支出|往来|金|账|款|明细)`)

func (e *Engine) extractNamedEntity(question string) string {
	q := strings.TrimSpace(question)

	// 策略 0：全名/高置信匹配（优先匹配真实对手方，避免“技术有限公司”被截断）
	if c := e.matchCounterpartyByName(q); c != "" {
		return c
	}

	// 策略 1：数据库优先匹配 (Sliding Window over DB)
	zhRe := regexp.MustCompile(`[\x{4e00}-\x{9fa5}]+`)
	best := ""
	for _, seg := range zhRe.FindAllString(q, -1) {
		runes := []rune(seg)
		for length := len(runes); length >= 2; length-- {
			for i := 0; i <= len(runes)-length; i++ {
				sub := string(runes[i : i+length])
				if len(sub) < 2 || containsAny(sub, []string{"帮我", "一下", "查询", "多少", "哪些", "价格", "一共", "支出", "报销", "经营", "分析", "风险", "健康", "评价", "应收", "应付", "账款", "费用", "资金", "货币", "流水"}) {
					continue
				}
				var exists int
				e.db.QueryRow(`SELECT 1 FROM bank_statement WHERE counterparty_name LIKE ? LIMIT 1`, "%"+sub+"%").Scan(&exists)
				if exists == 0 {
					e.db.QueryRow(`SELECT 1 FROM journal WHERE summary LIKE ? OR account_name LIKE ? LIMIT 1`, "%"+sub+"%", "%"+sub+"%").Scan(&exists)
				}
				if exists == 1 && len(sub) > len(best) {
					best = sub
				}
			}
		}
	}
	if best != "" {
		return best
	}

	// 策略 2：正则兜底匹配
	var entity string
	if m := namedEntityPattern.FindStringSubmatch(q); len(m) == 2 {
		entity = strings.TrimSpace(m[1])
		// 最终清洗：剔除年份、代词及核算科目干扰
		garbage := []string{"2024", "2025", "2026", "年", "一共", "总计", "的", "多少", "是", "在", "发生", "产生了", "合计", "账款", "收入", "支出", "费用", "成本", "利润"}
		for m := 1; m <= 12; m++ {
			garbage = append(garbage, fmt.Sprintf("%d月", m))
		}
		for d := 1; d <= 31; d++ {
			garbage = append(garbage, fmt.Sprintf("%d日", d))
		}

		for _, g := range garbage {
			entity = strings.ReplaceAll(entity, g, "")
		}
		entity = strings.TrimSpace(entity)
	}

	if len(entity) >= 2 {
		return entity
	}
	return ""
}

func (e *Engine) queryAnalysis(period string) Result {
	aging := analysis.NewAgingEngine(e.dbPath)
	defer aging.Close()
	summary, err := aging.AnalyzeSummary(e.Company, period)
	if err != nil {
		return Result{Success: false, Message: "analysis failed"}
	}
	return Result{
		Success: true,
		Message: "账龄分析成功",
		Data: map[string]any{
			"health":             summary.HealthScore,
			"receivable_total":   summary.ReceivableTotal,
			"payable_total":      summary.PayableTotal,
			"receivable_buckets": summary.ReceivableBuckets,
			"payable_buckets":    summary.PayableBuckets,
		},
		ExecutedSQL: []string{
			"queryAnalysis: internal aging engine SQL over journal with account_code LIKE '1122%'/'2202%'",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[账龄分析] period=%s health=%d AR=%.2f AP=%.2f", period, summary.HealthScore, summary.ReceivableTotal, summary.PayableTotal),
		},
	}
}

func (e *Engine) queryFallback(q, from, to, err string) Result {
	if r := e.ruleFallback(q, from, to); r.Success {
		return r
	}

	accounts := e.availableAccounts(to)
	samples := e.counterpartySamples()
	entity := e.extractNamedEntity(q)
	logs := []string{fmt.Sprintf("[识别] fallback实体识别结果: %s", entity)}
	payload := e.buildHostLLMPayload(from, to, q)
	return Result{
		Success:      false,
		Message:      "指令语义模糊",
		AnswerMethod: "llm_payload",
		Data: map[string]any{
			"fallback_attempted":  true,
			"hint":                "请给出更具体的问题，例如“2026年2月应收账款多少”或“飞未云科2月回款多少”",
			"available_accounts":  accounts,
			"counterparty_sample": samples,
			"llm_payload":         payload,
		},
		CalculationLogs: logs,
	}
}

func (e *Engine) ruleFallback(q, from, to string) Result {
	// 供应商数量
	if strings.Contains(q, "供应商") && strings.Contains(q, "多少") {
		return e.querySupplierCount()
	}
	// 人力成本
	if containsAny(q, []string{"人力成本", "工资成本", "薪酬成本"}) {
		return e.queryHRCost(from, to)
	}
	// 整体支出
	if containsAny(q, []string{"整体支出", "总支出", "全部支出"}) {
		return e.queryMonthlyExpenseFromBank(from, to)
	}
	entity := e.extractNamedEntity(q)
	if entity != "" && strings.Contains(q, "数据出来") {
		return e.queryEntityDataReady(entity, from, to)
	}
	if entity != "" && strings.Contains(q, "项目") && containsAny(q, []string{"收入", "成本", "支出"}) {
		return e.queryProjectIncomeCost(entity, from, to, q)
	}
	if entity != "" {
		return e.queryCounterpartyAmountFallback(q, entity, from, to)
	}
	return Result{Success: false}
}

func (e *Engine) queryMonthlyExpenseFromBank(from, to string) Result {
	var total float64
	sqlTxt := `SELECT COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND transaction_date BETWEEN ? AND ?`
	e.db.QueryRow(sqlTxt, e.Company, e.Company, from+"-01", monthEndDay(to)).Scan(&total)
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 整体支出 %.2f 元", to, total),
		Data:    map[string]any{"period": to, "total": total, "现金流出": total},
		ExecutedSQL: []string{
			fmt.Sprintf("queryMonthlyExpenseFromBank: %s [args: %s, %s, %s]", sqlTxt, e.Company, from+"-01", monthEndDay(to)),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[银行现金口径] %s 期间总支出(借方) %.2f 元", to, total),
		},
	}
}

func (e *Engine) querySupplierCount() Result {
	sqlTxt := `
SELECT COUNT(*) FROM (
  SELECT counterparty_name
  FROM bank_statement
  WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
    AND IFNULL(TRIM(counterparty_name),'') <> ''
  GROUP BY counterparty_name
  HAVING COALESCE(SUM(debit_amount),0) > COALESCE(SUM(credit_amount),0)
) t
`
	var count int
	e.db.QueryRow(sqlTxt, e.Company, e.Company).Scan(&count)

	rows, _ := e.db.Query(`
SELECT counterparty_name,
       ROUND(COALESCE(SUM(debit_amount),0),2) AS out_amt,
       ROUND(COALESCE(SUM(credit_amount),0),2) AS in_amt,
       ROUND(COALESCE(SUM(debit_amount),0)-COALESCE(SUM(credit_amount),0),2) AS net_out
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND IFNULL(TRIM(counterparty_name),'') <> ''
GROUP BY counterparty_name
HAVING COALESCE(SUM(debit_amount),0) > COALESCE(SUM(credit_amount),0)
ORDER BY net_out DESC
`, e.Company, e.Company)
	suppliers := make([]map[string]any, 0, count)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var outAmt, inAmt, netOut float64
			_ = rows.Scan(&name, &outAmt, &inAmt, &netOut)
			suppliers = append(suppliers, map[string]any{
				"name": name, "out_amount": outAmt, "in_amount": inAmt, "net_out": netOut,
			})
		}
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("供应商数量约为 %d 个", count),
		Data:    map[string]any{"count": count, "suppliers": suppliers},
		ExecutedSQL: []string{
			fmt.Sprintf("querySupplierCount: %s [args: %s]", sqlTxt, e.Company),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[供应商识别规则] 对手方净流出>净流入，共 %d 个", count),
		},
	}
}

func (e *Engine) queryHRCost(from, to string) Result {
	sqlTxt := `
SELECT COALESCE(SUM(CASE WHEN direction='借' THEN amount ELSE 0 END), 0)
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
  AND (account_name IN ('工资','社保','公积金','福利费') OR account_code LIKE '2211%' OR account_code LIKE '660201%')
`
	var total float64
	e.db.QueryRow(sqlTxt, e.Company, e.Company, from+"-01", monthEndDay(to)).Scan(&total)
	usedFallback := false
	if total == 0 {
		// 兜底：有些库只保留余额表，不保留工资分录
		e.db.QueryRow(`
SELECT COALESCE(SUM(closing_balance), 0)
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND (account_name LIKE '%应付职工薪酬%' OR account_code LIKE '2211%')
`, e.Company, e.Company, to).Scan(&total)
		usedFallback = true
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 人力成本 %.2f 元", to, total),
		Data:    map[string]any{"total": total, "period": to},
		ExecutedSQL: []string{
			fmt.Sprintf("queryHRCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, from+"-01", monthEndDay(to)),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[人力成本口径] 工资/社保/公积金/福利费借方合计 %.2f 元", total),
			fmt.Sprintf("[兜底触发] %v", usedFallback),
		},
	}
}

func (e *Engine) queryEntityDataReady(entity, from, to string) Result {
	sqlJournal := `SELECT COUNT(*) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND summary LIKE ? AND voucher_date BETWEEN ? AND ?`
	sqlBank := `SELECT COUNT(*) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND counterparty_name LIKE ? AND transaction_date BETWEEN ? AND ?`
	var jCnt, bCnt int
	e.db.QueryRow(sqlJournal, e.Company, e.Company, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&jCnt)
	e.db.QueryRow(sqlBank, e.Company, e.Company, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&bCnt)
	total := jCnt + bCnt
	if total > 0 {
		return Result{
			Success: true,
			Message: fmt.Sprintf("%s 在 %s 有 %d 条数据", entity, to, total),
			Data:    map[string]any{"entity": entity, "period": to, "has_data": true, "rows": total},
			ExecutedSQL: []string{
				fmt.Sprintf("queryEntityDataReady(journal): %s [args: %s, %s, %s]", sqlJournal, e.Company, "%"+entity+"%", from+"-01"),
				fmt.Sprintf("queryEntityDataReady(bank): %s [args: %s, %s, %s]", sqlBank, e.Company, "%"+entity+"%", from+"-01"),
			},
			CalculationLogs: []string{
				fmt.Sprintf("[数据完备性] journal=%d, bank=%d, total=%d", jCnt, bCnt, total),
			},
		}
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 在 %s 暂无数据", entity, to),
		Data:    map[string]any{"entity": entity, "period": to, "has_data": false, "rows": 0},
		ExecutedSQL: []string{
			fmt.Sprintf("queryEntityDataReady(journal): %s [args: %s, %s, %s]", sqlJournal, e.Company, "%"+entity+"%", from+"-01"),
			fmt.Sprintf("queryEntityDataReady(bank): %s [args: %s, %s, %s]", sqlBank, e.Company, "%"+entity+"%", from+"-01"),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[数据完备性] journal=%d, bank=%d, total=%d", jCnt, bCnt, total),
		},
	}
}

func (e *Engine) queryProjectIncomeCost(entity, from, to, question string) Result {
	sqlTxt := `SELECT COALESCE(SUM(credit_amount), 0), COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND counterparty_name LIKE ? AND transaction_date BETWEEN ? AND ?`
	var inAmt, outAmt float64
	e.db.QueryRow(sqlTxt, e.Company, e.Company, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&inAmt, &outAmt)
	if strings.Contains(question, "收入") {
		return Result{
			Success: true,
			Message: fmt.Sprintf("%s %s 项目收入 %.2f 元", to, entity, inAmt),
			Data:    map[string]any{"entity": entity, "period": to, "income": inAmt},
			ExecutedSQL: []string{
				fmt.Sprintf("queryProjectIncomeCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
			},
			CalculationLogs: []string{
				fmt.Sprintf("[项目收支] 收入=%.2f, 成本=%.2f", inAmt, outAmt),
			},
		}
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 项目成本 %.2f 元", to, entity, outAmt),
		Data:    map[string]any{"entity": entity, "period": to, "cost": outAmt},
		ExecutedSQL: []string{
			fmt.Sprintf("queryProjectIncomeCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[项目收支] 收入=%.2f, 成本=%.2f", inAmt, outAmt),
		},
	}
}

func (e *Engine) queryProjectARAP(entity, from, to string) Result {
	sqlTxt := `SELECT COALESCE(SUM(credit_amount), 0), COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND counterparty_name LIKE ? AND transaction_date BETWEEN ? AND ?`
	var inAmt, outAmt float64
	e.db.QueryRow(sqlTxt, e.Company, e.Company, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&inAmt, &outAmt)
	receivable := math.Max(inAmt-outAmt, 0)
	payable := math.Max(outAmt-inAmt, 0)
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 项目应收 %.2f 元，应付 %.2f 元", to, entity, receivable, payable),
		Data: map[string]any{
			"entity": entity, "period": to, "receivable": receivable, "payable": payable,
		},
		ExecutedSQL: []string{
			fmt.Sprintf("queryProjectARAP: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[项目应收应付] 收入=%.2f, 成本=%.2f, 应收=%.2f, 应付=%.2f", inAmt, outAmt, receivable, payable),
		},
	}
}

func (e *Engine) availableAccounts(period string) []string {
	rows, err := e.db.Query(`
SELECT DISTINCT account_name
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ?
ORDER BY account_name
LIMIT 30
`, e.Company, e.Company, period)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]string, 0, 30)
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		if n != "" {
			out = append(out, n)
		}
	}
	if len(out) > 0 {
		return out
	}

	// 回退：部分样本库仅有序时账，没有余额表
	rows2, err := e.db.Query(`
SELECT DISTINCT account_name
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
ORDER BY account_name
LIMIT 30
`, e.Company, e.Company)
	if err != nil {
		return out
	}
	defer rows2.Close()
	for rows2.Next() {
		var n string
		_ = rows2.Scan(&n)
		if n != "" {
			out = append(out, n)
		}
	}
	out = appendUniqueStrings(out, "货币资金", "银行存款", "应收账款", "应付账款", "管理费用", "研发支出", "人工成本", "支出", "费用")
	return out
}

func (e *Engine) counterpartySamples() []string {
	rows, err := e.db.Query(`
SELECT counterparty_name
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND IFNULL(TRIM(counterparty_name),'') <> ''
GROUP BY counterparty_name
ORDER BY SUM(ABS(COALESCE(credit_amount,0)-COALESCE(debit_amount,0))) DESC
LIMIT 10
`, e.Company, e.Company)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]string, 0, 10)
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func (e *Engine) matchCounterpartyByName(question string) string {
	nq := normalizeEntityText(question)
	if nq == "" {
		return ""
	}
	rows, err := e.db.Query(`SELECT DISTINCT counterparty_name FROM bank_statement WHERE IFNULL(TRIM(counterparty_name),'') <> '' ORDER BY LENGTH(counterparty_name) DESC`)
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		nm := normalizeEntityText(name)
		if len([]rune(nm)) < 2 {
			continue
		}
		if strings.Contains(nq, nm) {
			return name
		}
	}
	return ""
}

func normalizeEntityText(s string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "（", "", "）", "", "(", "", ")", "", "-", "", "_", "", ",", "", "，", "", ".", "", "。", "")
	return replacer.Replace(strings.TrimSpace(s))
}

func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		base = append(base, v)
		seen[v] = true
	}
	return base
}

func (e *Engine) HostLLMPayload(from, to, question string) Result {
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		anchor := e.getLatestPeriodAnchor().Format("2006-01")
		if strings.TrimSpace(from) == "" {
			from = anchor
		}
		if strings.TrimSpace(to) == "" {
			to = anchor
		}
	}
	return e.queryHostLLMPayload(question, from, to).withTraceData()
}

func shouldForceDualPerspective(q string) bool {
	if containsAny(q, []string{"人力成本", "工资成本", "薪酬成本", "项目成本", "应收", "应付", "税"}) {
		return false
	}
	return containsAny(q, []string{"收入", "成本", "利润", "销售额"})
}

func shouldBypassDualPerspective(q, entity string) bool {
	if strings.TrimSpace(entity) == "" {
		return false
	}
	return containsAny(q, []string{"客户", "供应商", "项目", "报销", "数据出来", "应收", "应付", "往来"})
}

func detectCoreMetric(q string) string {
	switch {
	case strings.Contains(q, "利润"):
		return "利润"
	case strings.Contains(q, "成本"):
		return "成本"
	case strings.Contains(q, "销售额"):
		return "销售额"
	default:
		return "收入"
	}
}

func pickMetricValue(metric string, dual *accounting.DualPerspective) (float64, float64) {
	switch metric {
	case "利润":
		return dual.Cash.Net, dual.Accrual.Profit
	case "成本":
		return dual.Cash.Expense, dual.Accrual.TotalCost
	case "销售额", "收入":
		return dual.Cash.Income, dual.Accrual.Revenue
	default:
		return dual.Cash.Income, dual.Accrual.Revenue
	}
}

func (e *Engine) buildHostLLMPayload(from, to, question string) map[string]any {
	startDate := from + "-01"
	endDate := monthEndDay(to)

	financialTables := map[string]any{
		"balance_sheet": e.queryRowsAsMaps(`
SELECT company, period, account_code, account_name, opening_balance, closing_balance
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
ORDER BY period, account_code
`, e.Company, e.Company, from, to),
		"income_statement": e.queryRowsAsMaps(`
SELECT company, period, item_name, current_amount, cumulative_amount
FROM income_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
ORDER BY period, item_name
`, e.Company, e.Company, from, to),
		"balance_detail": e.queryRowsAsMaps(`
SELECT company, year, period, account_code, account_name, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit
FROM balance_detail
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
ORDER BY year, period, account_code
`, e.Company, e.Company),
		"journal": e.queryRowsAsMaps(`
SELECT company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
ORDER BY voucher_date, voucher_no
`, e.Company, e.Company, startDate, endDate),
		"bank_statement": e.queryRowsAsMaps(`
SELECT company, transaction_date, transaction_time, transaction_type, debit_amount, credit_amount, balance, summary, counterparty_name, counterparty_account
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
ORDER BY transaction_date, transaction_time
`, e.Company, e.Company, startDate, endDate),
	}

	return map[string]any{
		"question": question,
		"company":  e.Company,
		"period": map[string]any{
			"from":       from,
			"to":         to,
			"start_date": startDate,
			"end_date":   endDate,
		},
		"financial_tables": financialTables,
		"trace": map[string]any{
			"intent":   "host_payload_or_fallback",
			"strategy": "sql_extract_then_host_llm_reasoning",
		},
	}
}

func (e *Engine) queryRowsAsMaps(sqlTxt string, args ...any) []map[string]any {
	rows, err := e.db.Query(sqlTxt, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil
	}

	out := make([]map[string]any, 0)
	for rows.Next() {
		raw := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if err := rows.Scan(dest...); err != nil {
			continue
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			v := raw[i]
			if b, ok := v.([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = v
			}
		}
		out = append(out, m)
	}
	return out
}

func (e *Engine) findMatchingAccount(question, period string) (string, error) {
	rows, _ := e.db.Query(`SELECT DISTINCT account_name FROM balance_sheet WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ?`, e.Company, e.Company, period)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var n string
			rows.Scan(&n)
			if strings.Contains(question, n) {
				return n, nil
			}
		}
	}
	return "", fmt.Errorf("account not found")
}

func availableCompanies(db *sql.DB) ([]string, error) {
	rows, _ := db.Query(`SELECT DISTINCT company FROM balance_sheet UNION SELECT DISTINCT company FROM bank_statement UNION SELECT DISTINCT company FROM journal`)
	if rows == nil {
		return nil, nil
	}
	defer rows.Close()
	var companies []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		if c != "" {
			companies = append(companies, c)
		}
	}
	return companies, nil
}

func monthEndDay(period string) string {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return "2026-02-28"
	}
	return t.AddDate(0, 1, -1).Format("2006-01-02")
}

func parsePeriod(period string) (int, int) {
	parts := strings.Split(period, "-")
	if len(parts) == 2 {
		y, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		return y, m
	}
	return 0, 0
}
