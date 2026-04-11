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
	ExecutedSQL     []string       `json:"executed_sql"`
	CalculationLogs []string       `json:"calculation_logs"`
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
	if err != nil { return nil, fmt.Errorf("open sqlite: %w", err) }
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
	if e.db == nil { return nil }
	return e.db.Close()
}

func (e *Engine) getLatestPeriodAnchor() time.Time {
	var maxDate string
	e.db.QueryRow(`SELECT MAX(voucher_date) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`, e.Company, e.Company).Scan(&maxDate)
	if maxDate == "" { return time.Now() }
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
	switch intent {
	case IntentIdentityQuery:
		role, _ := e.detectEntityRole(entity)
		result = Result{
			Success: true, 
			Message: fmt.Sprintf("识别结果: [%s] 是 [%s]", entity, role),
			Data: map[string]any{"entity": entity, "role": role},
		}
	case IntentARAPQuery:
		result = e.queryCounterpartyAmountFallback(q, entity, from, to)
	case IntentLargeTransactionQuery:
		result = e.queryLargeBankTransactions(q, from, to)
	case IntentTaxQuery:
		result = e.queryTax(from, to)
	case IntentMonthlySummary:
		if entity != "" {
			res := e.queryCounterpartyAmountFallback(q, entity, from, to)
			if res.Success { return res }
		}
		result = e.queryMonthlySummary(q, from, to)
	case IntentAnalysis:
		result = e.queryAnalysis(to)
	case IntentFallback:
		result = e.queryFallback(q, from, to, "")
	default:
		result = e.queryPrecise(q, to)
	}

	if result.Success { return result }
	
	// 智能分流降级：如果精确查询由于科目未发现而失败，且存在实体，则自动滑入往来款审计
	if entity != "" && (result.Message == "account not found" || !strings.Contains(result.Message, "no named")) {
		return e.queryFallback(q, from, to, result.Message)
	}
	
	if result.Message != "" { return result }
	return e.queryFallback(q, from, to, result.Message)
}

func (e *Engine) queryPrecise(question, period string) Result {
	accountName, err := e.findMatchingAccount(question, period)
	if err != nil { return Result{Success: false, Message: err.Error()} }
	startDate, endDate := period+"-01", monthEndDay(period)
	var opening, closing, debit, credit float64
	e.db.QueryRow(`SELECT opening_balance, closing_balance FROM balance_sheet WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ? AND account_name = ?`, e.Company, e.Company, period, accountName).Scan(&opening, &closing)
	e.db.QueryRow(`SELECT SUM(debit_amount), SUM(credit_amount) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND voucher_date BETWEEN ? AND ? AND account_name = ?`, e.Company, e.Company, startDate, endDate, accountName).Scan(&debit, &credit)
	return Result{Success: true, Message: fmt.Sprintf("%s %s 综合账务（余额+发生额）查询成功", period, accountName), Data: map[string]any{"period": period, "account": accountName, "opening": opening, "closing": closing, "debit": debit, "credit": credit}}
}

func (e *Engine) queryMonthlySummary(question, from, to string) Result {
	year, month := parsePeriod(to)
	cash, _ := e.calc.ComputeCashFlow(e.Company, from, to)
	is, _ := e.calc.ComputeIncomeStatement(e.Company, year, month)
	e.calc.ResetTrace()
	return Result{Success: true, Message: fmt.Sprintf("%s 月度汇总查询成功", to), Data: map[string]any{"cash_flow": cash, "income_statement": is}}
}

func (e *Engine) queryTax(from, to string) Result {
	startDate, endDate := from+"-01", monthEndDay(to)
	var output, input float64
	e.db.QueryRow(`SELECT COALESCE(SUM(credit_amount), 0), COALESCE(SUM(debit_amount), 0) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND (account_name LIKE '%销项%' OR account_code LIKE '222101%') AND voucher_date BETWEEN ? AND ?`, e.Company, e.Company, startDate, endDate).Scan(&output, &input)
	return Result{Success: true, Message: fmt.Sprintf("%s 销项税额 查询成功", to), Data: map[string]any{"output": output, "input": input}}
}

func (e *Engine) queryCounterpartyAmountFallback(question, entity, from, to string) Result {
	if entity == "" { return Result{Success: false, Message: "no named counterparty found"} }
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
			Success: true, 
			Message: fmt.Sprintf("[%s]（识别为[%s]）穿透审计成功，期间流水 %.2f 元", entity, role, total),
			Data: map[string]any{"entity": entity, "role": role, "amount": total, "is_retro": isRetro},
			ExecutedSQL: []string{bankSQL, jourSQL},
			CalculationLogs: logs,
		}
	}
	return Result{Success: false, Message: fmt.Sprintf("穿透审计失败：[%s] 无发生额", entity)}
}

func (e *Engine) queryLargeBankTransactions(question, from, to string) Result {
	var name string
	var amount float64
	e.db.QueryRow(`SELECT counterparty_name, credit_amount FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND transaction_date BETWEEN ? AND ? ORDER BY credit_amount DESC LIMIT 1`, e.Company, e.Company, from+"-01", monthEndDay(to)).Scan(&name, &amount)
	if name == "" { return Result{Success: false, Message: "未发现大额记录"} }
	return Result{Success: true, Message: fmt.Sprintf("%s 最大流入对手方为 [%s]，流水 %.2f 元", from, name, amount), Data: map[string]any{"counterparty": name, "amount": amount}}
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
			if strings.HasPrefix(code, "2211") { hasSalary = true }
			if strings.Contains(summary, "报销") || strings.Contains(summary, "工资") { employeeSignals++ }
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
	
	// 策略 1：数据库优先匹配 (Sliding Window over DB)
	zhRe := regexp.MustCompile(`[\x{4e00}-\x{9fa5}]+`)
	best := ""
	for _, seg := range zhRe.FindAllString(q, -1) {
		runes := []rune(seg)
		for length := len(runes); length >= 2; length-- {
			for i := 0; i <= len(runes)-length; i++ {
				sub := string(runes[i : i+length])
				if len(sub) < 2 || containsAny(sub, []string{"帮我", "一下", "查询", "多少", "哪些", "价格", "一共", "支出", "报销", "经营", "分析", "风险", "健康", "评价", "应收", "应付", "账款", "费用", "资金", "货币", "流水"}) { continue }
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
	if best != "" { return best }

	// 策略 2：正则兜底匹配
	var entity string
	if m := namedEntityPattern.FindStringSubmatch(q); len(m) == 2 {
		entity = strings.TrimSpace(m[1])
		// 最终清洗：剔除年份、月度、日期和数量代词干扰
		garbage := []string{"2024", "2025", "2026", "年", "一共", "总计", "的", "多少", "是", "在", "发生", "产生了", "合计", "账款"}
		for m := 1; m <= 12; m++ { garbage = append(garbage, fmt.Sprintf("%d月", m)) }
		for d := 1; d <= 31; d++ { garbage = append(garbage, fmt.Sprintf("%d日", d)) }

		for _, g := range garbage {
			entity = strings.ReplaceAll(entity, g, "")
		}
		entity = strings.TrimSpace(entity)
	}

	if len(entity) >= 2 { return entity }
	return ""
}

func (e *Engine) queryAnalysis(period string) Result {
	aging := analysis.NewAgingEngine(e.dbPath)
	defer aging.Close()
	summary, err := aging.AnalyzeSummary(e.Company, period)
	if err != nil { return Result{Success: false, Message: "analysis failed"} }
	return Result{Success: true, Message: "账龄分析成功", Data: map[string]any{"health": summary.HealthScore}}
}

func (e *Engine) queryFallback(q, from, to, err string) Result {
	if r := e.ruleFallback(q, from, to); r.Success { return r }
	return Result{Success: false, Message: "指令语义模糊", Data: map[string]any{"hint": "尝试查询科目余额、利润或往来款"}}
}

func (e *Engine) ruleFallback(q, from, to string) Result {
	entity := e.extractNamedEntity(q)
	if entity != "" { return e.queryCounterpartyAmountFallback(q, entity, from, to) }
	return Result{Success: false}
}

func (e *Engine) findMatchingAccount(question, period string) (string, error) {
	rows, _ := e.db.Query(`SELECT DISTINCT account_name FROM balance_sheet WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND period = ?`, e.Company, e.Company, period)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var n string
			rows.Scan(&n)
			if strings.Contains(question, n) { return n, nil }
		}
	}
	return "", fmt.Errorf("account not found")
}

func availableCompanies(db *sql.DB) ([]string, error) {
	rows, _ := db.Query(`SELECT DISTINCT company FROM balance_sheet UNION SELECT DISTINCT company FROM bank_statement UNION SELECT DISTINCT company FROM journal`)
	if rows == nil { return nil, nil }
	defer rows.Close()
	var companies []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		if c != "" { companies = append(companies, c) }
	}
	return companies, nil
}

func monthEndDay(period string) string {
	t, err := time.Parse("2006-01", period)
	if err != nil { return "2026-02-28" }
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
