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

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
	dbpkg "financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/openitems"
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
	executed := append([]string{}, r.ExecutedSQL...)
	logs := append([]string{}, r.CalculationLogs...)
	process := map[string]any{
		"answer_method":    r.AnswerMethod,
		"executed_sql":     executed,
		"calculation_logs": logs,
	}
	r.Data["answer_method"] = r.AnswerMethod
	r.Data["trace"] = process
	r.Data["process"] = process
	r.Data["executed_sql"] = executed
	r.Data["calculation_logs"] = logs
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
	if err := dbpkg.Bootstrap(context.Background(), dbPath); err != nil {
		return nil, fmt.Errorf("bootstrap db: %w", err)
	}
	db, err := dbpkg.Open(context.Background(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
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
	var maxDate any
	if err := e.db.QueryRow(`SELECT MAX(voucher_date) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`, e.Company, e.Company).Scan(&maxDate); err != nil {
		return time.Now()
	}
	if t, ok := parseAnchorDateValue(maxDate); ok {
		return t
	}
	return time.Now()
}

func parseAnchorDateValue(v any) (time.Time, bool) {
	switch raw := v.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		return raw, !raw.IsZero()
	case string:
		return parseAnchorDateString(raw)
	case []byte:
		return parseAnchorDateString(string(raw))
	default:
		return time.Time{}, false
	}
}

func parseAnchorDateString(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	if len(raw) >= len("2006-01-02") {
		if t, err := time.Parse("2006-01-02", raw[:10]); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (e *Engine) Query(question string) Result {
	q := NormalizeQuestion(question)
	resolved := ResolveCompany(q, e.available)
	if resolved != "" && resolved != e.Company {
		e.Company = resolved
	}
	intent, intentTrace := ClassifyIntentV2(q)
	traceMap := map[string]any{
		"router_version": intentTrace.RouterVersion,
		"matched":        append([]string{}, intentTrace.Matched...),
		"scores":         intentTrace.Scores,
		"final_intent":   intentTrace.FinalIntent,
		"confidence":     intentTrace.Confidence,
	}
	finalize := func(r Result) Result {
		if r.Data == nil {
			r.Data = map[string]any{}
		}
		r.Data["intent_trace"] = traceMap
		return r.withTraceData()
	}

	anchor := e.getLatestPeriodAnchor()
	from, to := ExtractPeriodWithNow(q, anchor)
	cfg := getRuleConfig()
	var result Result

	if shouldUseHRBreakdown(q, cfg) {
		result = e.queryHRBreakdown(from, to)
		if result.Success {
			return finalize(result)
		}
	}

	entity := e.extractNamedEntity(q)
	hasRealEntity := e.isRealBusinessEntity(q, entity)
	if !hasRealEntity && shouldUseSingleAccrualCoreMetrics(q) {
		result = e.queryAccrualCoreMetrics(q, from, to)
		if result.Success {
			return finalize(result)
		}
	}
	if shouldUseSupplierPaymentStats(q) {
		result = e.querySupplierPayments(from, to)
		if result.Success {
			return finalize(result)
		}
	}
	if hasRealEntity && isCounterpartyClassificationQuestion(q) {
		result = e.queryCounterpartyAmountFallback(q, entity, from, to)
		if result.Success {
			return finalize(result)
		}
	}
	if shouldUseReconciliation(q) {
		result = e.queryReconciliation(q, from, to)
		if result.Success {
			return finalize(result)
		}
	}
	// 只有在问题明确提到现金/到账/差异解释时，整体核心指标才展开双视角。
	// 实体类问题优先走主体审计路径，不再默认改写为“银行卡上看 vs 账上看”。
	if !hasRealEntity && shouldForceDualPerspective(q) && !shouldUseSingleAccrualCoreMetrics(q) {
		result = e.queryDualPerspectiveForCoreMetric(q, from, to)
		if result.Success {
			return finalize(result)
		}
	}
	if hasRealEntity && containsAny(q, append(metricQuestionKeywords(cfg), "回款", "到账", "收款", "费用", "支出", "付款", "付了", "支付")) {
		result = e.queryCounterpartyAmountFallback(q, entity, from, to)
		if result.Success {
			return finalize(result)
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
		return finalize(result)
	}

	// 智能分流降级：如果精确查询由于科目未发现而失败，且存在实体，则自动滑入往来款审计
	if entity != "" && result.Message == "account not found" {
		return finalize(e.queryFallback(q, from, to, result.Message))
	}
	if result.Message == "account not found" || strings.Contains(result.Message, "语义模糊") {
		return finalize(e.queryFallback(q, from, to, result.Message))
	}

	if result.Message != "" {
		return finalize(result)
	}
	return finalize(e.queryFallback(q, from, to, result.Message))
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

	// 1) 帐口径当月核心指标：优先利润表当月发生额，缺项再回退序时账
	book, bookSource, err := e.monthlyBookSummary(year, month)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	// 2) 累计指标（用于明细展现和回溯）
	is, _ := e.calc.ComputeIncomeStatement(e.Company, year, month)
	// 3) 钱口径
	cash, _ := e.calc.ComputeCashFlow(e.Company, from, to)
	logs := append([]string{}, e.calc.CalculationLogs...)
	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	var bridgeMap map[string]any
	if bridge, bridgeErr := analysis.AnalyzeProfitCashBridgeWithDB(context.Background(), e.db, e.Company, to); bridgeErr == nil {
		bridgeMap = bridgeToMap(&bridge)
		logs = append(logs, fmt.Sprintf("[利润调现金桥] period=%s estimated_operating_cash=%.2f bank_net_cash=%.2f non_operating_delta=%.2f", to, bridge.EstimatedOperatingCash, bridge.BankNetCash, bridge.NonOperatingCashDelta))
		sqls = appendUniqueStrings(sqls,
			"profit_cash_bridge(balance_detail): SELECT closing_debit, closing_credit FROM balance_detail WHERE ... AND period IN (?, previous_period) AND account_code IN ('1602','1122','1123','1221','2202','2203','2211','2221','2241','22210101','22210106')",
			"profit_cash_bridge(income_statement): SELECT current_amount FROM income_statement WHERE ... AND period = ? AND item_name LIKE '%净利润%'",
		)
	}
	sqls = appendUniqueStrings(sqls,
		"monthlyBookSummary(income_statement): SELECT item_name, current_amount FROM income_statement WHERE ... AND period = ?",
		"monthlyBookSummary(fallback_journal): ComputeMonthlyFromJournal + ComputeIncomeStatement when income_statement missing required rows",
	)
	logs = append(logs, fmt.Sprintf("[月度口径] period=%s source=%s", to, bookSource))

	revenue := book.Revenue
	expense := book.TotalCost

	mainMsg := fmt.Sprintf("%s 月度经营分析：账上收入 %.2f 元，成本及费用 %.2f 元，账面利润 %.2f 元；同时银行卡收款 %.2f 元、付款 %.2f 元。", to, revenue, expense, book.Profit, cash.Income, cash.Expense)

	// 智能回溯：如果本单月数据为空，则统计本年累计数据供参考
	if revenue == 0 && expense == 0 && book.Profit == 0 {
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
			"monthly": map[string]any{
				"year":    year,
				"month":   month,
				"source":  bookSource,
				"revenue": book.Revenue,
				"cost":    book.TotalCost,
				"profit":  book.Profit,
				"cost_detail": map[string]any{
					"operating_cost":  book.Cost,
					"tax_surcharge":   book.TaxSurcharge,
					"selling_expense": book.SellingExpense,
					"admin_expense":   book.AdminExpense,
					"finance_expense": book.FinanceExpense,
				},
			},
			"cumulative": is, "cash_flow": cash,
			"profit_cash_bridge": bridgeMap,
			// 兼容旧版本 top-level 字段（测试与外部调用依赖）
			"现金流入": cash.Income, "现金流出": cash.Expense, "净现金流": cash.Net,
			"财务做账口径(看利润)": map[string]any{
				"营业收入":    book.Revenue,
				"营业成本及费用": book.TotalCost,
				"账面利润":    book.Profit,
			},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func (e *Engine) queryAccrualCoreMetrics(question, from, to string) Result {
	year, month := parsePeriod(to)
	e.calc.ResetTrace()

	book, bookSource, err := e.monthlyBookSummary(year, month)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	cash, _ := e.calc.ComputeCashFlow(e.Company, from, to)
	logs := append([]string{}, e.calc.CalculationLogs...)
	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	requestedMetrics := detectRequestedMetrics(question)
	if len(requestedMetrics) == 0 {
		requestedMetrics = []string{detectCoreMetric(question)}
	}
	metric := metricDisplayName(detectCoreMetric(question))
	if len(requestedMetrics) == 1 {
		metric = requestedMetrics[0]
	}
	metrics := map[string]any{
		"收入": round2(book.Revenue),
		"成本": round2(book.TotalCost),
		"利润": round2(book.Profit),
	}
	accountValue := round2(metricValueFromBook(metric, book))

	var bridgeMap map[string]any
	if containsString(requestedMetrics, "利润") {
		if bridge, bridgeErr := analysis.AnalyzeProfitCashBridgeWithDB(context.Background(), e.db, e.Company, to); bridgeErr == nil {
			bridgeMap = bridgeToMap(&bridge)
			logs = append(logs, fmt.Sprintf("[核心指标-单口径] period=%s profit=%.2f estimated_operating_cash=%.2f", to, book.Profit, bridge.EstimatedOperatingCash))
			sqls = appendUniqueStrings(sqls,
				"profit_cash_bridge(balance_detail): SELECT closing_debit, closing_credit FROM balance_detail WHERE ... AND period IN (?, previous_period) AND account_code IN ('1602','1122','1123','1221','2202','2203','2211','2221','2241','22210101','22210106')",
				"profit_cash_bridge(income_statement): SELECT current_amount FROM income_statement WHERE ... AND period = ? AND item_name LIKE '%净利润%'",
			)
		}
	}

	sqls = appendUniqueStrings(sqls,
		"monthlyBookSummary(income_statement): SELECT item_name, current_amount FROM income_statement WHERE ... AND period = ?",
		"monthlyBookSummary(fallback_journal): ComputeMonthlyFromJournal + ComputeIncomeStatement when income_statement missing required rows",
	)
	logs = append(logs, fmt.Sprintf("[核心指标-单口径] period=%s source=%s requested=%v metric=%s account_value=%.2f", to, bookSource, requestedMetrics, metric, accountValue))

	return Result{
		Success:      true,
		AnswerMethod: "sql",
		Message:      buildAccrualCoreMetricsMessage(to, requestedMetrics, book),
		Data: map[string]any{
			"period":            to,
			"metric":            metric,
			"requested_metrics": requestedMetrics,
			"account_value":     accountValue,
			"total":             accountValue,
			"metrics":           metrics,
			"monthly": map[string]any{
				"year":    year,
				"month":   month,
				"source":  bookSource,
				"revenue": book.Revenue,
				"cost":    book.TotalCost,
				"profit":  book.Profit,
			},
			"财务做账口径(看利润)": map[string]any{
				"营业收入":    book.Revenue,
				"营业成本及费用": book.TotalCost,
				"账面利润":    book.Profit,
			},
			"现金流入": cash.Income,
			"现金流出": cash.Expense,
			"净现金流": cash.Net,
			"cash_flow": map[string]any{
				"现金流入": cash.Income,
				"现金流出": cash.Expense,
				"净现金流": cash.Net,
			},
			"profit_cash_bridge": bridgeMap,
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func (e *Engine) queryAccrualProfitOnly(from, to string) Result {
	return e.queryAccrualCoreMetrics("利润", from, to)
}

func (e *Engine) queryDualPerspectiveForCoreMetric(question, from, to string) Result {
	year, month := parsePeriod(to)
	e.calc.ResetTrace()
	unified, sqls, logs, err := e.computeUnifiedCoreMetrics(from, to, year, month)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	dualCash := unified.Cash
	dualAccrual := unified.Accrual

	requestedMetrics := detectRequestedMetrics(question)
	metric := detectCoreMetric(question)
	if len(requestedMetrics) == 1 {
		metric = requestedMetrics[0]
	}
	cashValue, accrualValue := pickMetricValue(metric, &accounting.DualPerspective{
		Cash: accounting.CashPerspective{
			Description: dualCash.Description,
			Income:      dualCash.Income,
			Expense:     dualCash.Expense,
			Net:         dualCash.Net,
		},
		Accrual: accounting.AccrualPerspective{
			Description: "账上看",
			Revenue:     dualAccrual.Revenue,
			TotalCost:   dualAccrual.TotalCost,
			Profit:      dualAccrual.Profit,
		},
	})
	msg := buildBossDualPerspectiveMessage(to, dualCash, dualAccrual, unified.Bridge)
	if len(requestedMetrics) == 1 {
		msg = fmt.Sprintf("%s\n补充你当前关注的指标：%s - 银行卡上看 %.2f 元，账上看 %.2f 元。", msg, metric, cashValue, accrualValue)
	}

	logs = append(logs, fmt.Sprintf("[双口径强制] metric=%s requested=%v cash=%.2f accrual=%.2f", metric, requestedMetrics, cashValue, accrualValue))
	sqls = appendUniqueStrings(sqls,
		"dual_perspective(cash): ComputeCashFlow over bank_statement in selected period",
		"dual_perspective(accrual): monthlyBookSummary => income_statement.current_amount (fallback journal if missing)",
	)
	return Result{
		Success:      true,
		Message:      msg,
		AnswerMethod: "sql",
		Data: map[string]any{
			"period":             to,
			"metric":             metric,
			"money_view":         dualCash,
			"account_view":       dualAccrual,
			"money_value":        cashValue,
			"account_value":      accrualValue,
			"requested_metrics":  requestedMetrics,
			"一致性守卫":              unified.Guard,
			"profit_cash_bridge": bridgeToMap(unified.Bridge),
			"现金流入":               dualCash.Income,
			"现金流出":               dualCash.Expense,
			"净现金流":               dualCash.Net,
			"财务做账口径(看利润)": map[string]any{
				"营业收入":    dualAccrual.Revenue,
				"营业成本及费用": dualAccrual.TotalCost,
				"账面利润":    dualAccrual.Profit,
			},
			"difference_bridge": map[string]any{
				"利润与现金净额差":     round2(dualAccrual.Profit - dualCash.Net),
				"收入确认回款时间差":    round2(dualAccrual.Revenue - dualCash.Income),
				"成本付款确认时间差":    round2(dualCash.Expense - dualAccrual.TotalCost),
				"其他调节项":        round2((dualAccrual.Profit - dualCash.Net) - (dualAccrual.Revenue - dualCash.Income) - (dualCash.Expense - dualAccrual.TotalCost)),
				"经营现金净额估算":     bridgeEstimatedCash(unified.Bridge),
				"含税项调节后经营现金估算": bridgeAdjustedEstimatedCash(unified.Bridge),
				"非经营现金差额":      bridgeNonOperatingDelta(unified.Bridge),
			},
			"dual_perspective": map[string]any{
				"cash": map[string]any{
					"说明":   "银行卡上看",
					"现金流入": dualCash.Income,
					"现金流出": dualCash.Expense,
					"净现金流": dualCash.Net,
				},
				"accrual": map[string]any{
					"说明":      "账上看",
					"营业收入":    dualAccrual.Revenue,
					"营业成本及费用": dualAccrual.TotalCost,
					"账面利润":    dualAccrual.Profit,
				},
			},
		},
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
}

func bridgeToMap(bridge *analysis.ProfitCashBridge) map[string]any {
	if bridge == nil {
		return nil
	}
	return map[string]any{
		"net_profit":                       bridge.NetProfit,
		"depreciation":                     bridge.Depreciation,
		"ar_increase":                      bridge.ARIncrease,
		"prepayment_increase":              bridge.PrepaymentIncrease,
		"other_receivable_increase":        bridge.OtherReceivableIncrease,
		"other_payable_increase":           bridge.OtherPayableIncrease,
		"ap_increase":                      bridge.APIncrease,
		"advance_receipt_increase":         bridge.AdvanceReceiptIncrease,
		"payroll_increase":                 bridge.PayrollIncrease,
		"tax_balance_increase":             bridge.TaxBalanceIncrease,
		"tax_timing_adjustment":            bridge.TaxTimingAdjustment,
		"estimated_operating_cash":         bridge.EstimatedOperatingCash,
		"adjusted_operating_cash_estimate": bridge.AdjustedOperatingCashEstimate,
		"operating_cash_in":                bridge.OperatingCashIn,
		"operating_cash_out":               bridge.OperatingCashOut,
		"operating_cash_net":               bridge.OperatingCashNet,
		"non_operating_cash_in":            bridge.NonOperatingCashIn,
		"non_operating_cash_out":           bridge.NonOperatingCashOut,
		"non_operating_cash_net":           bridge.NonOperatingCashNet,
		"mixed_cash_in":                    bridge.MixedCashIn,
		"mixed_cash_out":                   bridge.MixedCashOut,
		"mixed_cash_net":                   bridge.MixedCashNet,
		"bank_net_cash":                    bridge.BankNetCash,
		"excluded_cash_net":                bridge.ExcludedCashNet,
		"operating_cash_gap":               bridge.OperatingCashGap,
		"adjusted_operating_cash_gap":      bridge.AdjustedOperatingCashGap,
		"non_operating_cash_delta":         bridge.NonOperatingCashDelta,
	}
}

func bridgeEstimatedCash(bridge *analysis.ProfitCashBridge) float64 {
	if bridge == nil {
		return 0
	}
	return bridge.EstimatedOperatingCash
}

func bridgeAdjustedEstimatedCash(bridge *analysis.ProfitCashBridge) float64 {
	if bridge == nil {
		return 0
	}
	return bridge.AdjustedOperatingCashEstimate
}

func bridgeNonOperatingDelta(bridge *analysis.ProfitCashBridge) float64 {
	if bridge == nil {
		return 0
	}
	return bridge.NonOperatingCashDelta
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
		"host_payload(balance_detail): SELECT * FROM balance_detail WHERE ... AND period BETWEEN ? AND ?",
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
	if strings.Contains(question, "净税额") {
		msg = fmt.Sprintf("%s 净税额 %.2f 元（销项 %.2f 元 - 进项 %.2f 元）", to, output-input, output, input)
	} else if strings.Contains(question, "销项") && strings.Contains(question, "进项") {
		msg = fmt.Sprintf("%s 销项税额 %.2f 元，进项税额 %.2f 元，净税额 %.2f 元", to, output, input, output-input)
	} else if strings.Contains(question, "进项") {
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
	if entity != "" && e.isRealBusinessEntity(question, entity) && containsAny(question, []string{"应收", "应付"}) {
		return e.queryEntityARAP(entity, period)
	}
	if strings.Contains(question, "应收") {
		return e.queryAccountPayableReceivable(period, "应收账款", "1122", "receivable", "")
	}
	if strings.Contains(question, "应付") {
		return e.queryAccountPayableReceivable(period, "应付账款", "2202", "payable", "")
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

func (e *Engine) queryAccountPayableReceivable(period, accountName, accountCodePrefix, typ, entity string) Result {
	if strings.TrimSpace(entity) == "" {
		official := e.queryAccountPayableReceivableFromBalanceSheet(period, accountName, accountCodePrefix, typ)
		if official.Success {
			if openSummary, openResult := e.queryAccountPayableReceivableOpenItems(period, accountName, accountCodePrefix, typ, entity); openSummary {
				if official.Data == nil {
					official.Data = map[string]any{}
				}
				official.Data["open_item_analysis"] = openResult.Data
				official.Data["open_item_match"] = approxEqual(anyToFloat64(official.Data["total"]), anyToFloat64(openResult.Data["total"]))
				official.CalculationLogs = append(official.CalculationLogs,
					fmt.Sprintf("[开放项补充] open_item_total=%.2f official_total=%.2f matched=%v",
						anyToFloat64(openResult.Data["total"]), anyToFloat64(official.Data["total"]), official.Data["open_item_match"]),
				)
			}
			return official
		}
	}

	if ok, openResult := e.queryAccountPayableReceivableOpenItems(period, accountName, accountCodePrefix, typ, entity); ok {
		return openResult
	}

	if strings.TrimSpace(entity) != "" {
		return Result{
			Success: false,
			Message: fmt.Sprintf("[%s] 未找到%s余额", entity, accountName),
			Data: map[string]any{
				"entity":  entity,
				"account": accountName,
				"period":  period,
			},
		}
	}

	return e.queryAccountPayableReceivableFromBalanceSheet(period, accountName, accountCodePrefix, typ)
}

func (e *Engine) queryAccountPayableReceivableOpenItems(period, accountName, accountCodePrefix, typ, entity string) (bool, Result) {
	kind := openitems.Receivable
	historyLabel := "历史应收"
	currentLabel := "当月新增应收"
	if typ == "payable" {
		kind = openitems.Payable
		historyLabel = "历史应付"
		currentLabel = "当月新增应付"
	}
	openSummary, err := openitems.BuildSummary(context.Background(), e.db, openitems.Options{
		Company:           e.Company,
		Period:            period,
		AccountCodePrefix: accountCodePrefix,
		Kind:              kind,
		Counterparty:      entity,
	})
	if err == nil && openSummary.HasData {
		details := make([]map[string]any, 0, len(openSummary.CounterpartySummaries))
		for _, item := range openSummary.CounterpartySummaries {
			detailOpenItems := make([]map[string]any, 0, len(item.OpenItems))
			for _, openItem := range item.OpenItems {
				detailOpenItems = append(detailOpenItems, map[string]any{
					"counterparty": openItem.Counterparty,
					"source_date":  openItem.SourceDate,
					"voucher_no":   openItem.VoucherNo,
					"amount":       openItem.Amount,
					"age_days":     openItem.AgeDays,
				})
			}
			details = append(details, map[string]any{
				"counterparty":              item.Counterparty,
				"opening_balance":           item.OpeningBalance,
				"current_increase":          item.CurrentIncrease,
				"current_decrease":          item.CurrentDecrease,
				"historical_settlement":     item.HistoricalSettlement,
				"current_period_settlement": item.CurrentSettlement,
				"settlement_confidence":     settlementConfidenceMap(item.SettlementConfidence),
				"closing_balance":           item.ClosingBalance,
				"open_items":                detailOpenItems,
			})
		}

		openItemMaps := make([]map[string]any, 0, len(openSummary.OpenItems))
		for _, item := range openSummary.OpenItems {
			openItemMaps = append(openItemMaps, map[string]any{
				"counterparty": item.Counterparty,
				"source_date":  item.SourceDate,
				"voucher_no":   item.VoucherNo,
				"amount":       item.Amount,
				"age_days":     item.AgeDays,
			})
		}

		sumCheck := CheckSumEqualsTotal(extractClosingBalances(details), openSummary.ClosingBalance)
		rollforwardCheck := CheckOpeningDeltaClosing(openSummary.OpeningBalance, openSummary.CurrentIncrease-openSummary.CurrentDecrease, openSummary.ClosingBalance)
		scopeLabel := accountName
		if strings.TrimSpace(entity) != "" {
			scopeLabel = fmt.Sprintf("%s %s", entity, accountName)
		}
		settlementParts := make([]string, 0, 5)
		if openSummary.HistoricalSettlement > 0 {
			settlementParts = append(settlementParts, fmt.Sprintf("冲销%s %.2f", historyLabel, openSummary.HistoricalSettlement))
		}
		if openSummary.CurrentSettlement > 0 {
			settlementParts = append(settlementParts, fmt.Sprintf("冲销%s %.2f", currentLabel, openSummary.CurrentSettlement))
		}
		if openSummary.SettlementConfidence.ProbableHistoricalSettlement > 0 {
			settlementParts = append(settlementParts, fmt.Sprintf("高概率冲销%s %.2f", historyLabel, openSummary.SettlementConfidence.ProbableHistoricalSettlement))
		}
		if openSummary.SettlementConfidence.ProbableCurrentSettlement > 0 {
			settlementParts = append(settlementParts, fmt.Sprintf("高概率冲销%s %.2f", currentLabel, openSummary.SettlementConfidence.ProbableCurrentSettlement))
		}
		if openSummary.SettlementConfidence.UnmatchedDecrease > 0 {
			settlementParts = append(settlementParts, fmt.Sprintf("未能直接配对的本月减少 %.2f", openSummary.SettlementConfidence.UnmatchedDecrease))
		}
		msg := fmt.Sprintf("%s %s合计 %.2f 元（期初 %.2f，本月新增 %.2f，本月减少 %.2f",
			period, scopeLabel, openSummary.ClosingBalance, openSummary.OpeningBalance, openSummary.CurrentIncrease, openSummary.CurrentDecrease)
		if len(settlementParts) > 0 {
			msg += "，其中" + strings.Join(settlementParts, "，")
		}
		msg += "）"
		return true, Result{
			Success: true,
			Message: msg,
			Data: map[string]any{
				"type":                      typ,
				"period":                    period,
				"total":                     openSummary.ClosingBalance,
				"details":                   details,
				"account":                   accountName,
				"entity":                    entity,
				"closing":                   openSummary.ClosingBalance,
				"opening_balance":           openSummary.OpeningBalance,
				"current_increase":          openSummary.CurrentIncrease,
				"current_decrease":          openSummary.CurrentDecrease,
				"historical_settlement":     openSummary.HistoricalSettlement,
				"current_period_settlement": openSummary.CurrentSettlement,
				"settlement_confidence":     settlementConfidenceMap(openSummary.SettlementConfidence),
				"open_items":                openItemMaps,
				"source":                    "journal_open_items",
				"arithmetic_checks": map[string]any{
					"sum_equals_total":  sumCheck,
					"rollforward_check": rollforwardCheck,
				},
			},
			ExecutedSQL: []string{
				"queryAccountPayableReceivable(open_items): SELECT voucher_date, account_code, voucher_no, account_name, summary, counterparty, debit_amount, credit_amount FROM journal WHERE ... AND account_code LIKE ? AND voucher_date <= ? ORDER BY DATE(voucher_date), voucher_no, account_code, account_name, summary, counterparty, debit_amount, credit_amount",
			},
			CalculationLogs: []string{
				fmt.Sprintf("[AR/AP开放项] period=%s account=%s opening=%.2f increase=%.2f decrease=%.2f historical_settlement=%.2f current_settlement=%.2f probable_historical=%.2f probable_current=%.2f unmatched=%.2f closing=%.2f counterparty_count=%d", period, accountName, openSummary.OpeningBalance, openSummary.CurrentIncrease, openSummary.CurrentDecrease, openSummary.HistoricalSettlement, openSummary.CurrentSettlement, openSummary.SettlementConfidence.ProbableHistoricalSettlement, openSummary.SettlementConfidence.ProbableCurrentSettlement, openSummary.SettlementConfidence.UnmatchedDecrease, openSummary.ClosingBalance, len(details)),
				fmt.Sprintf("[算术校验] sum_equals_total passed=%v diff=%.2f", sumCheck.Passed, sumCheck.Diff),
				fmt.Sprintf("[滚动校验] rollforward passed=%v diff=%.2f", rollforwardCheck.Passed, rollforwardCheck.Diff),
			},
		}
	}
	return false, Result{}
}

func (e *Engine) queryEntityARAP(entity, period string) Result {
	receivable := e.queryAccountPayableReceivable(period, "应收账款", "1122", "receivable", entity)
	payable := e.queryAccountPayableReceivable(period, "应付账款", "2202", "payable", entity)
	if !receivable.Success && !payable.Success {
		return Result{Success: false, Message: fmt.Sprintf("[%s] 未找到应收/应付余额", entity)}
	}

	receivableTotal := 0.0
	receivableDetails := []map[string]any{}
	if receivable.Success {
		receivableTotal, _ = receivable.Data["total"].(float64)
		receivableDetails = mapsFromAnySlice(receivable.Data["details"])
	}
	payableTotal := 0.0
	payableDetails := []map[string]any{}
	if payable.Success {
		payableTotal, _ = payable.Data["total"].(float64)
		payableDetails = mapsFromAnySlice(payable.Data["details"])
	}
	inferencePrefix := ""
	if resultUsesInferredOpenItemSettlement(receivable) || resultUsesInferredOpenItemSettlement(payable) {
		inferencePrefix = "按开放项推断："
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("[%s] %s %s应收 %.2f 元，应付 %.2f 元", entity, period, inferencePrefix, receivableTotal, payableTotal),
		Data: map[string]any{
			"entity":           entity,
			"period":           period,
			"receivable_total": round2(receivableTotal),
			"payable_total":    round2(payableTotal),
			"receivable":       receivable.Data,
			"payable":          payable.Data,
			"details": map[string]any{
				"receivable": receivableDetails,
				"payable":    payableDetails,
			},
		},
		ExecutedSQL:     append(append([]string{}, receivable.ExecutedSQL...), payable.ExecutedSQL...),
		CalculationLogs: append(append([]string{}, receivable.CalculationLogs...), payable.CalculationLogs...),
	}
}

func mapsFromAnySlice(v any) []map[string]any {
	raw, ok := v.([]map[string]any)
	if ok {
		return raw
	}
	return []map[string]any{}
}

func settlementConfidenceMap(v openitems.SettlementConfidence) map[string]any {
	return map[string]any{
		"confirmed_historical_settlement": v.ConfirmedHistoricalSettlement,
		"probable_historical_settlement":  v.ProbableHistoricalSettlement,
		"confirmed_current_settlement":    v.ConfirmedCurrentSettlement,
		"probable_current_settlement":     v.ProbableCurrentSettlement,
		"unmatched_decrease":              v.UnmatchedDecrease,
	}
}

func resultUsesInferredOpenItemSettlement(result Result) bool {
	if !result.Success || result.Data == nil {
		return false
	}
	if result.Data["source"] != "journal_open_items" {
		return false
	}
	raw, ok := result.Data["settlement_confidence"].(map[string]any)
	if !ok {
		return false
	}
	return anyToFloat64(raw["probable_historical_settlement"])+
		anyToFloat64(raw["probable_current_settlement"])+
		anyToFloat64(raw["unmatched_decrease"]) > 0
}

func anyToFloat64(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func (e *Engine) queryAccountPayableReceivableFromBalanceSheet(period, accountName, accountCodePrefix, typ string) Result {
	rows, err := e.db.Query(`
SELECT account_name, opening_balance, closing_balance
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
	openingTotal := 0.0
	total := 0.0
	for rows.Next() {
		var name string
		var opening float64
		var closing float64
		if err := rows.Scan(&name, &opening, &closing); err != nil {
			continue
		}
		openingTotal += opening
		total += closing
		details = append(details, map[string]any{"account": name, "opening_balance": opening, "closing_balance": closing})
	}
	if len(details) == 0 {
		return Result{Success: false, Message: "该期间未找到应收/应付余额"}
	}

	sumCheck := CheckSumEqualsTotal(extractClosingBalances(details), total)
	rollforward, hasRollforward := e.queryBalanceDetailRollforward(period, accountCodePrefix)
	rollforwardCheck := ArithmeticCheckResult{Passed: true, Diff: 0, Message: "balance_detail rollforward not available"}
	if hasRollforward {
		rollforwardCheck = CheckOpeningDeltaClosing(rollforward.OpeningNet, rollforward.DeltaNet, rollforward.ClosingNet)
	}

	msg := fmt.Sprintf("%s %s期末余额 %.2f 元", period, accountName, total)
	return Result{
		Success: true,
		Message: msg,
		Data: map[string]any{
			"type": typ, "period": period, "total": total, "details": details,
			"account": accountName, "source": "balance_sheet", "opening_balance": round2(openingTotal), "closing": total,
			"arithmetic_checks": map[string]any{
				"sum_equals_total":    sumCheck,
				"rollforward_check":   rollforwardCheck,
				"balance_rollforward": rollforward,
			},
		},
		ExecutedSQL: []string{
			"queryAccountPayableReceivable: SELECT account_name, opening_balance, closing_balance FROM balance_sheet WHERE ... AND (account_name LIKE ? OR account_code LIKE ?)",
			"queryAccountPayableReceivable(balance_detail): SELECT SUM(opening_debit), SUM(opening_credit), SUM(current_debit), SUM(current_credit), SUM(closing_debit), SUM(closing_credit) FROM balance_detail WHERE ... AND account_code LIKE ?",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[AR/AP汇总] period=%s account=%s total=%.2f detail_count=%d", period, accountName, total, len(details)),
			fmt.Sprintf("[算术校验] sum_equals_total passed=%v diff=%.2f", sumCheck.Passed, sumCheck.Diff),
			fmt.Sprintf("[滚动校验] rollforward passed=%v diff=%.2f", rollforwardCheck.Passed, rollforwardCheck.Diff),
		},
	}
}

type balanceRollforward struct {
	OpeningNet float64 `json:"opening_net"`
	DeltaNet   float64 `json:"delta_net"`
	ClosingNet float64 `json:"closing_net"`
}

func (e *Engine) queryBalanceDetailRollforward(period, accountCodePrefix string) (balanceRollforward, bool) {
	var openingDebit, openingCredit, currentDebit, currentCredit, closingDebit, closingCredit float64
	err := e.db.QueryRow(`
SELECT
  COALESCE(SUM(opening_debit),0),
  COALESCE(SUM(opening_credit),0),
  COALESCE(SUM(current_debit),0),
  COALESCE(SUM(current_credit),0),
  COALESCE(SUM(closing_debit),0),
  COALESCE(SUM(closing_credit),0)
FROM balance_detail
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND account_code LIKE ?
`, e.Company, e.Company, period, accountCodePrefix+"%").Scan(&openingDebit, &openingCredit, &currentDebit, &currentCredit, &closingDebit, &closingCredit)
	if err != nil {
		return balanceRollforward{}, false
	}
	openingNet := openingDebit - openingCredit
	deltaNet := currentDebit - currentCredit
	closingNet := closingDebit - closingCredit
	if openingNet == 0 && deltaNet == 0 && closingNet == 0 {
		return balanceRollforward{}, false
	}
	return balanceRollforward{
		OpeningNet: round2(openingNet),
		DeltaNet:   round2(deltaNet),
		ClosingNet: round2(closingNet),
	}, true
}

func extractClosingBalances(details []map[string]any) []float64 {
	out := make([]float64, 0, len(details))
	for _, d := range details {
		if v, ok := d["closing_balance"].(float64); ok {
			out = append(out, v)
		}
	}
	return out
}

func (e *Engine) queryCounterpartyAmountFallback(question, entity, from, to string) Result {
	if entity == "" {
		return Result{Success: false, Message: "no named counterparty found"}
	}
	snap := e.buildCounterpartySnapshot(entity, from, to)
	evidence := e.collectCounterpartyEvidence(entity, from, to)
	classification := ClassifyCounterparty(entity, evidence)
	taxReport := NormalizeTax(entity, evidence)
	role := string(classification.Role)
	if role == "" {
		role = snap.Role
	}
	q := NormalizeQuestion(question)
	usedRetro := false
	if snap.BankIn == 0 && snap.BankOut == 0 && snap.RevenueNet == 0 && snap.BookCost == 0 && snap.BookExpense == 0 {
		retroFrom := from[:4] + "-01"
		snap = e.buildCounterpartySnapshot(entity, retroFrom, to)
		evidence = e.collectCounterpartyEvidence(entity, retroFrom, to)
		classification = ClassifyCounterparty(entity, evidence)
		taxReport = NormalizeTax(entity, evidence)
		role = string(classification.Role)
		if role == "" {
			role = snap.Role
		}
		usedRetro = true
	}
	roleLabel := fmt.Sprintf("（识别为[%s]）", role)
	periodLabel := displayPeriod(from, to)
	receiptPeriodLabel := displayReceiptPeriodLabel(q, from, to)

	logs := []string{
		fmt.Sprintf("[对手方识别] entity=%s role=%s confidence=%.3f signals=%v", entity, role, classification.Confidence, classification.Signals),
		fmt.Sprintf("[往来快照] bank_in=%.2f bank_out=%.2f revenue_net=%.2f cost=%.2f expense=%.2f output_vat=%.2f input_vat=%.2f basis=%s", snap.BankIn, snap.BankOut, snap.RevenueNet, snap.BookCost, snap.BookExpense, snap.OutputVAT, snap.InputVAT, snap.ComparisonBasis),
		TraceTaxNormalization(taxReport),
	}
	if usedRetro {
		logs = append(logs, fmt.Sprintf("[年度回溯] %s 当月无记录，已回溯到 %s~%s", entity, from[:4]+"-01", to))
	}
	sqls := []string{
		"counterparty(bank_statement): SELECT counterparty_name, summary, debit_amount, credit_amount FROM bank_statement WHERE ... AND counterparty_name LIKE ?",
		"counterparty(journal): SELECT counterparty, account_code, account_name, summary, direction, debit_amount, credit_amount FROM journal WHERE ... AND (summary LIKE ? OR counterparty LIKE ?)",
	}

	resultData := map[string]any{
		"entity":            entity,
		"role":              role,
		"bank_in":           round2(snap.BankIn),
		"bank_out":          round2(snap.BankOut),
		"revenue_net":       round2(snap.RevenueNet),
		"book_cost":         round2(snap.BookCost),
		"book_expense":      round2(snap.BookExpense),
		"output_vat":        round2(snap.OutputVAT),
		"input_vat":         round2(snap.InputVAT),
		"difference_reason": snap.DifferenceReason,
		"comparison_basis":  snap.ComparisonBasis,
		"evidence":          evidence,
		"tax_breakdown":     taxReport,
	}

	cfg := getRuleConfig()
	if containsAny(q, cfg.CounterpartyClassificationQuestionKeywords()) {
		reasonParts := make([]string, 0, 4)
		if snap.BookCost+snap.BookExpense > 0 {
			reasonParts = append(reasonParts, fmt.Sprintf("账上成本/费用 %.2f 元", round2(snap.BookCost+snap.BookExpense)))
		}
		if snap.RevenueNet > 0 {
			reasonParts = append(reasonParts, fmt.Sprintf("账上收入 %.2f 元", round2(snap.RevenueNet)))
		}
		if snap.BankOut > 0 {
			reasonParts = append(reasonParts, fmt.Sprintf("银行付款 %.2f 元", round2(snap.BankOut)))
		}
		if snap.BankIn > 0 {
			reasonParts = append(reasonParts, fmt.Sprintf("银行收款 %.2f 元", round2(snap.BankIn)))
		}
		reason := strings.Join(reasonParts, "；")
		switch role {
		case "supplier":
			return Result{
				Success: true,
				Message: fmt.Sprintf("[%s]%s %s 判断为供应商/成本侧往来。%s。", entity, roleLabel, periodLabel, reason),
				Data: map[string]any{
					"entity": entity,
					"role":   role,
					"basis":  reasonParts,
				},
				ExecutedSQL:     sqls,
				CalculationLogs: logs,
			}
		case "customer":
			return Result{
				Success: true,
				Message: fmt.Sprintf("[%s]%s %s 判断为客户/收入侧往来。%s。", entity, roleLabel, periodLabel, reason),
				Data: map[string]any{
					"entity": entity,
					"role":   role,
					"basis":  reasonParts,
				},
				ExecutedSQL:     sqls,
				CalculationLogs: logs,
			}
		}
	}

	switch {
	case containsAny(q, []string{"回款", "到账", "收款"}) && !containsAny(q, []string{"预收款", "应收款"}):
		amount := round2(snap.BankIn)
		if amount == 0 {
			return Result{Success: false, Message: fmt.Sprintf("[%s] 在 %s 未找到回款/到账记录", entity, periodLabel)}
		}
		msg := fmt.Sprintf("[%s]%s %s回款 %.2f 元", entity, roleLabel, receiptPeriodLabel, amount)
		if snap.ComparisonBasis == "historical_receipt" || snap.ComparisonBasis == "historical_receipt_and_current_revenue" {
			msg = fmt.Sprintf("[%s]%s %s到账 %.2f 元。数据库能确认这是历史应收回款相关，但不能直接当成当期新收入。", entity, roleLabel, receiptPeriodLabel, amount)
		}
		if subPeriod, ok := extractReceiptSubPeriod(q, from, to); ok {
			subAmount := round2(e.counterpartyBankReceipts(entity, subPeriod, subPeriod))
			msg = fmt.Sprintf("[%s]%s %s回款 %.2f 元；其中%s到账 %.2f 元", entity, roleLabel, receiptPeriodLabel, amount, displaySubPeriodLabel(subPeriod), subAmount)
			if snap.ComparisonBasis == "historical_receipt" || snap.ComparisonBasis == "historical_receipt_and_current_revenue" {
				msg = fmt.Sprintf("[%s]%s %s到账 %.2f 元；其中%s到账 %.2f 元。数据库能确认这类到账包含历史应收回款因素，不能直接当成当期新收入。", entity, roleLabel, receiptPeriodLabel, amount, displaySubPeriodLabel(subPeriod), subAmount)
			}
			resultData["sub_period"] = subPeriod
			resultData["sub_period_receipts"] = subAmount
			logs = append(logs, fmt.Sprintf("[回款拆分] cumulative=%s amount=%.2f sub_period=%s sub_amount=%.2f", periodLabel, amount, subPeriod, subAmount))
		}
		resultData["amount"] = amount
		resultData["total"] = amount
		return Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: sqls, CalculationLogs: logs}
	case containsAny(q, []string{"销售额", "收入", "营收"}):
		if snap.RevenueNet > 0 {
			amount := round2(snap.RevenueNet)
			msg := fmt.Sprintf("[%s]%s %s 账上确认收入 %.2f 元", entity, roleLabel, periodLabel, amount)
			if snap.ComparisonBasis == "historical_receipt_and_current_revenue" {
				msg = fmt.Sprintf("[%s]%s %s 账上确认收入 %.2f 元；另有到账 %.2f 元属于历史应收回款相关，不能直接并成当期销售额。", entity, roleLabel, periodLabel, amount, round2(snap.BankIn))
			} else if taxReport.Output.Included && taxReport.Output.TaxAmount > 0 && approxEqual(snap.BankIn, taxReport.Output.AccrualAmount+taxReport.Output.TaxAmount) {
				msg = fmt.Sprintf("[%s]%s %s 账上确认收入 %.2f 元；到账和收入的差额 %.2f 元主要是销项税。", entity, roleLabel, periodLabel, amount, round2(taxReport.Output.TaxAmount))
			}
			resultData["amount"] = amount
			resultData["total"] = amount
			return Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: sqls, CalculationLogs: logs}
		}
		if snap.BankIn > 0 {
			amount := round2(snap.BankIn)
			msg := fmt.Sprintf("[%s]%s %s 仅看到到账 %.2f 元，暂未看到同期间收入确认分录。", entity, roleLabel, periodLabel, amount)
			resultData["amount"] = amount
			resultData["total"] = amount
			return Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: sqls, CalculationLogs: logs}
		}
	case role == "employee" || strings.Contains(q, "报销"):
		amount := round2(snap.BankOut)
		if amount == 0 {
			amount = round2(snap.BookExpense + snap.BookCost)
		}
		if amount > 0 {
			msg := fmt.Sprintf("[%s]%s %s 报销/费用 %.2f 元", entity, roleLabel, periodLabel, amount)
			resultData["amount"] = amount
			resultData["total"] = amount
			return Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: sqls, CalculationLogs: logs}
		}
	case containsAny(q, []string{"成本", "费用", "支出", "付款"}):
		amount := round2(snap.BookCost + snap.BookExpense)
		label := "账上成本/费用"
		if containsAny(q, []string{"付款", "付了", "支付"}) || amount == 0 {
			amount = round2(snap.BankOut)
			label = "付款"
		}
		if amount > 0 {
			msg := fmt.Sprintf("[%s]%s %s %s %.2f 元", entity, roleLabel, periodLabel, label, amount)
			if role == "supplier" || role == "mixed" {
				msg = fmt.Sprintf("[%s]%s %s 属于供应商相关，%s %.2f 元，不应归到收入差异里。", entity, roleLabel, periodLabel, label, amount)
			}
			resultData["amount"] = amount
			resultData["total"] = amount
			if label == "付款" {
				resultData["payment"] = amount
			} else {
				resultData["cost"] = amount
			}
			return Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: sqls, CalculationLogs: logs}
		}
	}

	fallbackAmount := round2(math.Max(snap.BankIn, math.Max(snap.BankOut, math.Max(snap.RevenueNet, snap.BookCost+snap.BookExpense))))
	if fallbackAmount == 0 {
		return Result{Success: false, Message: fmt.Sprintf("穿透审计失败：[%s] 无发生额", entity)}
	}
	resultData["amount"] = fallbackAmount
	resultData["total"] = fallbackAmount
	return Result{
		Success:         true,
		Message:         fmt.Sprintf("[%s]%s 已提取相关发生额 %.2f 元", entity, roleLabel, fallbackAmount),
		Data:            resultData,
		ExecutedSQL:     sqls,
		CalculationLogs: logs,
	}
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
	endDate := monthEndDay(e.getLatestPeriodAnchor().Format("2006-01"))
	startDate := "2000-01-01"
	evidence := e.collectCounterpartyEvidence(name, startDate[:7], endDate[:7])
	classification := ClassifyCounterparty(name, evidence)
	if classification.Role == CounterpartyUnknown {
		return "unknown", "unknown"
	}
	return string(classification.Role), fmt.Sprintf("role=%s confidence=%.3f signals=%v", classification.Role, classification.Confidence, classification.Signals)
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
				if len(sub) < 2 || isGenericMetricEntity(sub) || containsAny(sub, []string{"帮我", "一下", "查询", "多少", "哪些", "价格", "一共", "支出", "报销", "经营", "分析", "风险", "健康", "评价", "应收", "应付", "账款", "费用", "资金", "货币", "流水", "工资", "社保", "公积金", "人力成本", "薪酬"}) {
					continue
				}
				var exists int
				e.db.QueryRow(`SELECT 1 FROM bank_statement WHERE counterparty_name LIKE ? LIMIT 1`, "%"+sub+"%").Scan(&exists)
				if exists == 0 {
					e.db.QueryRow(`SELECT 1 FROM journal WHERE summary LIKE ? OR counterparty LIKE ? LIMIT 1`, "%"+sub+"%", "%"+sub+"%").Scan(&exists)
				}
				if exists == 1 && len(sub) > len(best) {
					best = sub
				}
			}
		}
	}
	best = trimEntityNoiseSuffixes(best)
	if best != "" && !isGenericMetricEntity(best) {
		return best
	}

	// 策略 2：正则兜底匹配
	var entity string
	if m := namedEntityPattern.FindStringSubmatch(q); len(m) == 2 {
		entity = strings.TrimSpace(m[1])
		// 最终清洗：剔除年份、代词及核算科目干扰
		garbage := []string{"2024", "2025", "2026", "年", "一共", "总计", "的", "多少", "是", "在", "发生", "产生了", "合计", "账款", "收入", "支出", "费用", "成本", "利润", "营收", "销售额", "总成本", "人力成本", "工资", "社保", "公积金", "薪酬", "销项税", "进项税", "应收", "应付", "应收账款", "应付账款"}
		for m := 1; m <= 12; m++ {
			garbage = append(garbage, fmt.Sprintf("%d月", m))
		}
		for d := 1; d <= 31; d++ {
			garbage = append(garbage, fmt.Sprintf("%d日", d))
		}

		for _, g := range garbage {
			entity = strings.ReplaceAll(entity, g, "")
		}
		entity = trimEntityNoiseSuffixes(strings.TrimSpace(entity))
	}

	if len(entity) >= 2 && !isGenericMetricEntity(entity) {
		return entity
	}
	return ""
}

func (e *Engine) isRealBusinessEntity(question, entity string) bool {
	name := strings.TrimSpace(entity)
	if len([]rune(name)) < 2 || isGenericMetricEntity(name) {
		return false
	}

	// 项目问法不强依赖往来名录，先放行（例如“XX项目2月收入多少”）
	if strings.Contains(question, "项目") {
		return true
	}

	like := "%" + name + "%"
	var exists int
	e.db.QueryRow(`SELECT 1 FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND counterparty_name LIKE ? LIMIT 1`, e.Company, e.Company, like).Scan(&exists)
	if exists == 1 {
		return true
	}
	e.db.QueryRow(`SELECT 1 FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND (summary LIKE ? OR (IFNULL(TRIM(counterparty),'') <> '' AND counterparty LIKE ?)) LIMIT 1`, e.Company, e.Company, like, like).Scan(&exists)
	return exists == 1
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
	cfg := getRuleConfig()
	if strings.Contains(q, "供应商") && strings.Contains(q, "多少") {
		return e.querySupplierPayments(from, to)
	}
	// 人力成本
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) {
		return e.queryHRBreakdown(from, to)
	}
	// 整体支出
	if containsAny(q, cfg.FallbackMonthlyExpenseKeywords) {
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

func (e *Engine) querySupplierPayments(from, to string) Result {
	startDate := from + "-01"
	endDate := monthEndDay(to)
	periodLabel := displayPeriod(from, to)
	sqlTxt := `
SELECT counterparty_name,
       ROUND(COALESCE(SUM(debit_amount),0),2) AS out_amt,
       ROUND(COALESCE(SUM(credit_amount),0),2) AS in_amt,
       COUNT(*) AS txn_count
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
  AND COALESCE(TRIM(counterparty_name),'') <> ''
  AND COALESCE(debit_amount,0) > 0
GROUP BY counterparty_name
ORDER BY out_amt DESC, counterparty_name
`
	rows, err := e.db.Query(sqlTxt, e.Company, e.Company, startDate, endDate)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	defer rows.Close()

	suppliers := make([]map[string]any, 0)
	excluded := make([]map[string]any, 0)
	total := 0.0
	logs := []string{
		fmt.Sprintf("[供应商付款] period=%s start=%s end=%s", periodLabel, startDate, endDate),
	}
	for rows.Next() {
		var name string
		var outAmt, inAmt float64
		var txnCount int
		if scanErr := rows.Scan(&name, &outAmt, &inAmt, &txnCount); scanErr != nil {
			continue
		}

		evidence := e.collectCounterpartyEvidence(name, from, to)
		classification := ClassifyCounterparty(name, evidence)
		include, reason := e.shouldIncludeSupplierPaymentCounterparty(name, classification)
		row := map[string]any{
			"name":       name,
			"out_amount": round2(outAmt),
			"in_amount":  round2(inAmt),
			"txn_count":  txnCount,
			"role":       string(classification.Role),
			"confidence": classification.Confidence,
			"signals":    classification.Signals,
		}
		if include {
			suppliers = append(suppliers, row)
			total += outAmt
			logs = append(logs, fmt.Sprintf("[供应商付款-纳入] %s out=%.2f role=%s reason=%s", name, round2(outAmt), classification.Role, reason))
			continue
		}
		row["exclude_reason"] = reason
		excluded = append(excluded, row)
		logs = append(logs, fmt.Sprintf("[供应商付款-剔除] %s out=%.2f role=%s reason=%s", name, round2(outAmt), classification.Role, reason))
	}

	total = round2(total)
	msg := fmt.Sprintf("%s 发生付款的外部供应商共 %d 家，合计 %.2f 元。", periodLabel, len(suppliers), total)
	if len(suppliers) == 0 {
		msg = fmt.Sprintf("%s 暂未识别到外部供应商付款。", periodLabel)
	}

	return Result{
		Success: true,
		Message: msg,
		Data: map[string]any{
			"period":                  periodLabel,
			"count":                   len(suppliers),
			"total":                   total,
			"suppliers":               suppliers,
			"excluded_counterparties": excluded,
		},
		ExecutedSQL: []string{
			fmt.Sprintf("querySupplierPayments(bank_statement): %s [args: %s, %s, %s]", sqlTxt, e.Company, startDate, endDate),
			"supplier_payment_classification: collectCounterpartyEvidence + ClassifyCounterparty + internal-party filter per counterparty",
		},
		CalculationLogs: logs,
	}
}

func (e *Engine) shouldIncludeSupplierPaymentCounterparty(name string, classification CounterpartyClassification) (bool, string) {
	cfg := getRuleConfig()
	switch {
	case looksLikeSupplierPaymentExcludedName(name, cfg):
		return false, "non_counterparty_flow"
	case internalPartyMatchesCompany(e.Company, name) || looksLikeInternalOrgUnit(name, cfg):
		return false, "internal_party"
	case classification.Role == CounterpartyEmployee:
		return false, "employee_related"
	case classification.Role == CounterpartyCustomer:
		return false, "customer_only"
	case classification.Role == CounterpartySupplier:
		return true, "classified_supplier"
	case classification.Role == CounterpartyMixed:
		return true, "classified_mixed"
	case looksLikeExternalOrganizationCounterparty(name):
		return true, "organization_name_fallback"
	default:
		return false, "unknown_non_organization"
	}
}

func looksLikeSupplierPaymentExcludedName(name string, cfg RuleConfig) bool {
	normalized := normalizeEntityText(name)
	if normalized == "" {
		return false
	}
	for _, kw := range cfg.SupplierPaymentExcludeNames() {
		nk := normalizeEntityText(kw)
		if nk != "" && strings.Contains(normalized, nk) {
			return true
		}
	}
	return false
}

func looksLikeExternalOrganizationCounterparty(name string) bool {
	normalized := normalizeEntityText(name)
	if normalized == "" {
		return false
	}
	hints := []string{
		"公司", "有限责任公司", "有限公司", "事务所", "管理中心", "中心",
		"研究院", "合伙企业", "基金会", "协会", "学院", "大学",
	}
	for _, hint := range hints {
		if strings.Contains(normalized, normalizeEntityText(hint)) {
			return true
		}
	}
	return false
}

func (e *Engine) queryHRCost(from, to string) Result {
	return e.queryHRBreakdown(from, to)
}

func (e *Engine) queryHRBreakdown(from, to string) Result {
	start := from + "-01"
	end := monthEndDay(to)
	periodLabel := displayPeriod(from, to)

	accountSQL := `
SELECT
  COALESCE(SUM(CASE WHEN direction='借' AND account_code IN ('66020101','66022301') THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN direction='借' AND account_code IN ('66020102','66022302') THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN direction='借' AND account_code IN ('66020103','66022303') THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS housing
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
`
	cashSQL := `
SELECT
  COALESCE(SUM(CASE WHEN account_code LIKE '1002%' AND direction='贷' AND summary LIKE '%工资%' THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN account_code LIKE '1002%' AND direction='贷' AND (summary LIKE '%社保%' OR summary LIKE '%社保扣款%') THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN account_code LIKE '1002%' AND direction='贷' AND summary LIKE '%公积金%' THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS housing,
  0 AS branch_transfer
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
`
	if e.journalHasVoucherGrouping() {
		cashSQL = `
SELECT
  COALESCE(SUM(CASE WHEN (base.account_code LIKE '1001%' OR base.account_code LIKE '1002%') AND base.direction='贷' AND (
    NOT (base.summary LIKE '%分公司%' OR IFNULL(base.counterparty,'') LIKE '%分公司%') AND (
      base.summary LIKE '%工资%' OR
      base.summary LIKE '%薪酬%' OR
      EXISTS (
        SELECT 1 FROM journal sibling
        WHERE (? LIKE '%' || sibling.company || '%' OR sibling.company LIKE '%' || ? || '%')
          AND sibling.voucher_date = base.voucher_date
          AND sibling.voucher_no = base.voucher_no
          AND sibling.direction = '借'
          AND (sibling.account_code LIKE '2211%' OR sibling.account_name LIKE '%应付职工薪酬%')
          AND (sibling.summary LIKE '%工资%' OR sibling.summary LIKE '%薪酬%' OR sibling.account_name LIKE '%工资%' OR sibling.account_name LIKE '%薪酬%')
      )
    )
  ) THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN (base.account_code LIKE '1001%' OR base.account_code LIKE '1002%') AND base.direction='贷' AND (
    base.summary LIKE '%社保%' OR
    base.summary LIKE '%社保扣款%' OR
    EXISTS (
      SELECT 1 FROM journal sibling
      WHERE (? LIKE '%' || sibling.company || '%' OR sibling.company LIKE '%' || ? || '%')
        AND sibling.voucher_date = base.voucher_date
        AND sibling.voucher_no = base.voucher_no
        AND sibling.direction = '借'
        AND (sibling.account_code LIKE '2211%' OR sibling.account_name LIKE '%应付职工薪酬%')
        AND (sibling.summary LIKE '%社保%' OR sibling.account_name LIKE '%社保%')
    )
  ) THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN (base.account_code LIKE '1001%' OR base.account_code LIKE '1002%') AND base.direction='贷' AND (
    base.summary LIKE '%公积金%' OR
    EXISTS (
      SELECT 1 FROM journal sibling
      WHERE (? LIKE '%' || sibling.company || '%' OR sibling.company LIKE '%' || ? || '%')
        AND sibling.voucher_date = base.voucher_date
        AND sibling.voucher_no = base.voucher_no
        AND sibling.direction = '借'
        AND (sibling.account_code LIKE '2211%' OR sibling.account_name LIKE '%应付职工薪酬%')
        AND (sibling.summary LIKE '%公积金%' OR sibling.account_name LIKE '%公积金%')
    )
  ) THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS housing,
  COALESCE(SUM(CASE WHEN (base.account_code LIKE '1001%' OR base.account_code LIKE '1002%') AND base.direction='贷' AND (
    base.summary LIKE '%分公司%' OR
    IFNULL(base.counterparty,'') LIKE '%分公司%'
  ) AND EXISTS (
      SELECT 1 FROM journal sibling
      WHERE (? LIKE '%' || sibling.company || '%' OR sibling.company LIKE '%' || ? || '%')
        AND sibling.voucher_date = base.voucher_date
        AND sibling.voucher_no = base.voucher_no
        AND sibling.direction = '借'
        AND (sibling.account_code LIKE '2211%' OR sibling.account_name LIKE '%应付职工薪酬%')
    ) THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS branch_transfer
FROM journal base
WHERE (? LIKE '%' || base.company || '%' OR base.company LIKE '%' || ? || '%')
  AND base.voucher_date BETWEEN ? AND ?
`
	}

	var accountWage, accountSocial, accountHousing float64
	var cashWage, cashSocial, cashHousing, cashBranchTransfer float64
	branchTransferSQL := ""
	branchTransferLogs := []string{"[分公司内部转账] voucher grouping unavailable"}
	e.db.QueryRow(accountSQL, e.Company, e.Company, start, end).Scan(&accountWage, &accountSocial, &accountHousing)
	if e.journalHasVoucherGrouping() {
		e.db.QueryRow(
			cashSQL,
			e.Company, e.Company,
			e.Company, e.Company,
			e.Company, e.Company,
			e.Company, e.Company,
			e.Company, e.Company, start, end,
		).Scan(&cashWage, &cashSocial, &cashHousing, &cashBranchTransfer)
		cashBranchTransfer, branchTransferSQL, branchTransferLogs = e.detectInternalBranchTransferCash(start, end)
	} else {
		e.db.QueryRow(cashSQL, e.Company, e.Company, start, end).Scan(&cashWage, &cashSocial, &cashHousing, &cashBranchTransfer)
	}

	accountTotal := round2(accountWage + accountSocial + accountHousing)
	cashTotal := round2(cashWage + cashSocial + cashHousing + cashBranchTransfer)
	usedFallback := false
	if accountTotal == 0 {
		var fallbackTotal float64
		e.db.QueryRow(`
SELECT COALESCE(SUM(closing_balance), 0)
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND (account_name LIKE '%应付职工薪酬%' OR account_code LIKE '2211%')
`, e.Company, e.Company, to).Scan(&fallbackTotal)
		if fallbackTotal > 0 {
			accountTotal = round2(fallbackTotal)
			usedFallback = true
		}
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 人力成本（账上）%.2f 元；银行卡实际支出 %.2f 元。工资/社保/公积金已拆分返回。", periodLabel, accountTotal, cashTotal),
		Data: map[string]any{
			"period": periodLabel,
			"total":  accountTotal,
			"hr_breakdown": map[string]any{
				"accounting": map[string]any{
					"工资":  round2(accountWage),
					"社保":  round2(accountSocial),
					"公积金": round2(accountHousing),
					"合计":  accountTotal,
				},
				"cash": map[string]any{
					"工资":      round2(cashWage),
					"社保":      round2(cashSocial),
					"公积金":     round2(cashHousing),
					"分公司内部转账": round2(cashBranchTransfer),
					"合计":      cashTotal,
				},
			},
		},
		ExecutedSQL: []string{
			fmt.Sprintf("queryHRBreakdown(accounting): %s [args: %s, %s, %s]", accountSQL, e.Company, start, end),
			fmt.Sprintf("queryHRBreakdown(cash): %s [args: %s, %s, %s]", cashSQL, e.Company, start, end),
			fmt.Sprintf("queryHRBreakdown(branch_transfer): %s [args: %s, %s, %s]", branchTransferSQL, e.Company, start, end),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[人力成本-账上] 工资=%.2f 社保=%.2f 公积金=%.2f 合计=%.2f", round2(accountWage), round2(accountSocial), round2(accountHousing), accountTotal),
			fmt.Sprintf("[人力成本-现金] 工资=%.2f 社保=%.2f 公积金=%.2f 分公司内部转账=%.2f 合计=%.2f", round2(cashWage), round2(cashSocial), round2(cashHousing), round2(cashBranchTransfer), cashTotal),
			fmt.Sprintf("[兜底触发] %v", usedFallback),
			strings.Join(branchTransferLogs, " | "),
		},
	}
}

func (e *Engine) journalHasVoucherGrouping() bool {
	cols := e.tableColumns("journal")
	return cols["voucher_date"] && cols["voucher_no"]
}

func (e *Engine) tableColumns(table string) map[string]bool {
	rows, err := e.db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 0", table))
	if err != nil {
		return map[string]bool{}
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(cols))
	for _, col := range cols {
		out[strings.ToLower(strings.TrimSpace(col))] = true
	}
	return out
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
	snap := e.buildCounterpartySnapshot(entity, from, to)
	sqlTxt := `SELECT counterparty_name, credit_amount, debit_amount FROM bank_statement WHERE ... AND counterparty_name LIKE ?`
	if strings.Contains(question, "收入") {
		amount := round2(snap.RevenueNet)
		if amount == 0 {
			amount = round2(snap.BankIn)
		}
		return Result{
			Success: true,
			Message: fmt.Sprintf("%s %s 项目收入 %.2f 元", to, entity, amount),
			Data:    map[string]any{"entity": entity, "period": to, "income": amount, "bank_in": round2(snap.BankIn), "revenue_net": round2(snap.RevenueNet)},
			ExecutedSQL: []string{
				fmt.Sprintf("queryProjectIncomeCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
			},
			CalculationLogs: []string{
				fmt.Sprintf("[项目收支] bank_in=%.2f revenue_net=%.2f", snap.BankIn, snap.RevenueNet),
			},
		}
	}
	amount := round2(snap.BookCost + snap.BookExpense)
	if amount == 0 {
		amount = round2(snap.BankOut)
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 项目成本 %.2f 元", to, entity, amount),
		Data:    map[string]any{"entity": entity, "period": to, "cost": amount, "bank_out": round2(snap.BankOut), "book_cost": round2(snap.BookCost), "book_expense": round2(snap.BookExpense)},
		ExecutedSQL: []string{
			fmt.Sprintf("queryProjectIncomeCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[项目收支] bank_out=%.2f book_cost=%.2f book_expense=%.2f", snap.BankOut, snap.BookCost, snap.BookExpense),
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
	rows, err := e.db.Query(counterpartyNameCandidatesQuery(), e.Company, e.Company, e.Company, e.Company)
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

func counterpartyNameCandidatesQuery() string {
	return `
SELECT name
FROM (
  SELECT counterparty_name AS name
  FROM bank_statement
  WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
    AND COALESCE(TRIM(counterparty_name), '') <> ''
  UNION
  SELECT counterparty AS name
  FROM journal
  WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
    AND COALESCE(TRIM(counterparty), '') <> ''
) candidates
ORDER BY LENGTH(name) DESC, name
`
}

func normalizeEntityText(s string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "（", "", "）", "", "(", "", ")", "", "-", "", "_", "", ",", "", "，", "", ".", "", "。", "")
	return replacer.Replace(strings.TrimSpace(s))
}

func trimEntityNoiseSuffixes(entity string) string {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return ""
	}
	suffixes := append([]string{
		"报销了", "报销", "报账", "到账", "回款", "收款", "付款",
		"费用", "支出", "收入", "成本", "利润", "明细", "金额",
		"产生了", "产生", "多少", "报",
	}, getRuleConfig().GenericMetricStopwords...)
	for {
		trimmed := entity
		for _, suffix := range suffixes {
			suffix = strings.TrimSpace(suffix)
			if suffix == "" {
				continue
			}
			if strings.HasSuffix(trimmed, suffix) {
				trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, suffix))
			}
		}
		if trimmed == entity {
			return trimmed
		}
		entity = trimmed
	}
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
	cfg := getRuleConfig()
	blockers := append([]string{"项目成本"}, cfg.intentKeywordGroup(routerGroupHRCost)...)
	blockers = append(blockers, cfg.intentKeywordGroup(string(IntentARAPQuery))...)
	blockers = append(blockers, cfg.intentKeywordGroup(string(IntentTaxQuery))...)
	if containsAny(q, blockers) {
		return false
	}
	return containsAny(q, metricQuestionKeywords(cfg))
}

func shouldUseSingleAccrualCoreMetrics(q string) bool {
	cfg := getRuleConfig()
	if shouldUseReconciliation(q) || containsAny(q, cfg.ProfitSingleViewBlockKeywords()) {
		return false
	}
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) {
		return false
	}
	return containsAny(q, metricQuestionKeywords(cfg))
}

func shouldUseSupplierPaymentStats(q string) bool {
	return strings.Contains(q, "供应商") && strings.Contains(q, "多少")
}

func isCounterpartyClassificationQuestion(q string) bool {
	return containsAny(q, getRuleConfig().CounterpartyClassificationQuestionKeywords())
}

func shouldBypassDualPerspective(q, entity string) bool {
	return strings.TrimSpace(entity) != "" && !isGenericMetricEntity(entity)
}

func shouldUseHRBreakdown(q string, cfg RuleConfig) bool {
	asksBreakdown := containsAny(q, cfg.HRBreakdownKeywords())
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) && asksBreakdown {
		return true
	}
	return containsAny(q, []string{"工资", "社保", "公积金"}) && containsAny(q, []string{"多少", "明细", "分别", "合计", "成本"})
}

func isGenericMetricEntity(entity string) bool {
	key := normalizeEntityText(entity)
	if key == "" {
		return true
	}
	cfg := getRuleConfig()
	for _, s := range cfg.GenericMetricStopwords {
		if normalizeEntityText(s) == key {
			return true
		}
	}
	return false
}

func detectRequestedMetrics(q string) []string {
	metrics := make([]string, 0, 3)
	cfg := getRuleConfig()
	if containsAny(q, cfg.MetricKeywords(metricKeyRevenue)) {
		metrics = append(metrics, "收入")
	}
	if containsAny(q, cfg.MetricKeywords(metricKeyCost)) {
		metrics = append(metrics, "成本")
	}
	if containsAny(q, cfg.MetricKeywords(metricKeyProfit)) {
		metrics = append(metrics, "利润")
	}
	if len(metrics) == 0 {
		metrics = append(metrics, detectCoreMetric(q))
	}
	return metrics
}

func detectCoreMetric(q string) string {
	switch {
	case containsAny(q, getRuleConfig().MetricKeywords(metricKeyProfit)):
		return "利润"
	case containsAny(q, getRuleConfig().MetricKeywords(metricKeyCost)):
		return "成本"
	default:
		return "收入"
	}
}

func metricQuestionKeywords(cfg RuleConfig) []string {
	keywords := make([]string, 0, 8)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyRevenue)...)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyCost)...)
	keywords = append(keywords, cfg.MetricKeywords(metricKeyProfit)...)
	return dedupeStrings(keywords)
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

func metricValueFromBook(metric string, book monthlyBookView) float64 {
	switch metricDisplayName(metric) {
	case "利润":
		return book.Profit
	case "成本":
		return book.TotalCost
	default:
		return book.Revenue
	}
}

func buildAccrualCoreMetricsMessage(period string, requestedMetrics []string, book monthlyBookView) string {
	if len(requestedMetrics) <= 1 {
		switch metricDisplayName(firstMetricOrDefault(requestedMetrics, "收入")) {
		case "利润":
			return fmt.Sprintf("%s 账面利润 %.2f 元（收入 %.2f 元，成本及费用 %.2f 元）。", period, book.Profit, book.Revenue, book.TotalCost)
		case "成本":
			return fmt.Sprintf("%s 账上成本及费用 %.2f 元。", period, book.TotalCost)
		default:
			return fmt.Sprintf("%s 账上收入 %.2f 元。", period, book.Revenue)
		}
	}
	return fmt.Sprintf("%s 账上收入 %.2f 元，成本及费用 %.2f 元，账面利润 %.2f 元。", period, book.Revenue, book.TotalCost, book.Profit)
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == strings.TrimSpace(want) {
			return true
		}
	}
	return false
}

func firstMetricOrDefault(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	if strings.TrimSpace(items[0]) == "" {
		return fallback
	}
	return items[0]
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
  AND period BETWEEN ? AND ?
ORDER BY year, period, account_code
`, e.Company, e.Company, from, to),
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

func displayPeriod(from, to string) string {
	if strings.TrimSpace(from) == "" {
		return to
	}
	if from == to {
		return to
	}
	return from + "~" + to
}

func extractReceiptSubPeriod(q, from, to string) (string, bool) {
	if !strings.Contains(q, "其中") {
		return "", false
	}
	idx := strings.Index(q, "其中")
	if idx < 0 || idx >= len(q)-len("其中") {
		return "", false
	}
	subQuestion := strings.TrimSpace(q[idx+len("其中"):])
	if subQuestion == "" {
		return "", false
	}
	anchorYear, _ := parsePeriod(to)
	anchor := time.Date(anchorYear, time.December, 1, 0, 0, 0, 0, time.UTC)
	subFrom, subTo := ExtractPeriodWithNow(subQuestion, anchor)
	if subFrom == "" || subTo == "" || subFrom != subTo {
		return "", false
	}
	return subFrom, true
}

func displaySubPeriodLabel(period string) string {
	year, month := parsePeriod(period)
	if year == 0 || month == 0 {
		return period
	}
	return fmt.Sprintf("%d月", month)
}

func displayReceiptPeriodLabel(q, from, to string) string {
	if strings.Contains(q, "今年") && strings.HasSuffix(strings.TrimSpace(from), "-01") {
		return "今年"
	}
	return displayPeriod(from, to) + " "
}

func (e *Engine) counterpartyBankReceipts(entity, from, to string) float64 {
	var amount float64
	e.db.QueryRow(`
SELECT COALESCE(SUM(credit_amount), 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND counterparty_name LIKE ?
  AND transaction_date BETWEEN ? AND ?
`, e.Company, e.Company, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&amount)
	return amount
}
