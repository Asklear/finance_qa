package query

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
)

type unifiedCoreMetrics struct {
	Period            string
	Cash              accounting.CashPerspective
	Accrual           monthlyBookView
	AccrualFrom       string
	AccrualValidation map[string]any
	Bridge            *analysis.ProfitCashBridge
	Guard             map[string]any
}

func (e *Engine) computeUnifiedCoreMetrics(from, to string) (*unifiedCoreMetrics, []string, []string, error) {
	cash, err := e.calc.ComputeCashFlow(e.Company, from, to)
	if err != nil {
		return nil, nil, nil, err
	}

	book, source, validation, extraSQLs, extraLogs, err := e.bookSummaryForRange(from, to)
	if err != nil {
		return nil, nil, nil, err
	}

	rawAccrual := accounting.AccrualPerspective{
		Description: "经营口径",
		Revenue:     book.Revenue,
		TotalCost:   book.TotalCost,
		Profit:      book.Profit,
	}

	guard := buildConsistencyGuard(rawAccrual, book, *cash, source, validation)
	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	sqls = appendUniqueStrings(sqls, extraSQLs...)
	logs := append([]string{}, e.calc.CalculationLogs...)
	logs = append(logs, extraLogs...)
	logs = append(logs, fmt.Sprintf("[统一口径] accrual_source=%s cash_source=bank_statement", source))
	var bridge *analysis.ProfitCashBridge
	if from == to {
		if cashBridge, bridgeErr := analysis.AnalyzeProfitCashBridgeWithDB(context.Background(), e.db, e.Company, to); bridgeErr == nil {
			bridge = &cashBridge
			logs = append(logs, fmt.Sprintf("[利润调现金桥] profit=%.2f depreciation=%.2f ar=%.2f prepayment=%.2f other_receivable=%.2f other_payable=%.2f ap=%.2f advance_receipt=%.2f payroll=%.2f tax_balance=%.2f tax_timing=%.2f estimated_operating_cash=%.2f adjusted_operating_cash=%.2f bank_net_cash=%.2f non_operating_delta=%.2f",
				bridge.NetProfit, bridge.Depreciation, bridge.ARIncrease, bridge.PrepaymentIncrease, bridge.OtherReceivableIncrease, bridge.OtherPayableIncrease, bridge.APIncrease, bridge.AdvanceReceiptIncrease, bridge.PayrollIncrease, bridge.TaxBalanceIncrease, bridge.TaxTimingAdjustment, bridge.EstimatedOperatingCash, bridge.AdjustedOperatingCashEstimate, bridge.BankNetCash, bridge.NonOperatingCashDelta))
			sqls = appendUniqueStrings(sqls,
				"profit_cash_bridge(balance_detail): SELECT closing_debit, closing_credit FROM balance_detail WHERE ... AND period IN (?, previous_period) AND account_code IN ('1602','1122','1123','1221','2202','2203','2211','2221','2241','22210101','22210106')",
				"profit_cash_bridge(income_statement): SELECT current_amount FROM income_statement WHERE ... AND period = ? AND item_name LIKE '%净利润%'",
			)
		} else {
			logs = append(logs, fmt.Sprintf("[利润调现金桥] skipped: %v", bridgeErr))
		}
	} else {
		logs = append(logs, fmt.Sprintf("[利润调现金桥] skipped: multi-period aggregation %s~%s", from, to))
	}
	if passed, _ := guard["passed"].(bool); !passed {
		logs = append(logs, "[一致性守卫] 发现跨口径漂移，已强制采用标准口径输出")
	}

	return &unifiedCoreMetrics{
		Period:            displayPeriod(from, to),
		Cash:              *cash,
		Accrual:           book,
		AccrualFrom:       source,
		AccrualValidation: validation,
		Bridge:            bridge,
		Guard:             guard,
	}, sqls, logs, nil
}

func (e *Engine) bookSummaryForRange(from, to string) (monthlyBookView, string, map[string]any, []string, []string, error) {
	periods, err := periodsBetween(from, to)
	if err != nil {
		return monthlyBookView{}, "", nil, nil, nil, err
	}
	if len(periods) == 0 {
		return monthlyBookView{}, "", nil, nil, nil, fmt.Errorf("no periods resolved for range %s~%s", from, to)
	}
	if len(periods) == 1 {
		year, month := parsePeriod(periods[0])
		book, source, err := e.monthlyBookSummary(year, month)
		if err != nil {
			return monthlyBookView{}, "", nil, nil, nil, err
		}
		return book, source, nil,
			[]string{
				"monthlyBookSummary(income_statement): SELECT item_name, current_amount FROM income_statement WHERE ... AND period = ?",
				"monthlyBookSummary(fallback_journal): ComputeMonthlyFromJournal + ComputeIncomeStatement when income_statement missing required rows",
			},
			[]string{fmt.Sprintf("[期间汇总] single_period=%s source=%s", periods[0], source)},
			nil
	}

	var total monthlyBookView
	sourceCounts := map[string]int{}
	logs := make([]string, 0, len(periods)+2)
	for _, period := range periods {
		year, month := parsePeriod(period)
		book, source, err := e.monthlyBookSummary(year, month)
		if err != nil {
			return monthlyBookView{}, "", nil, nil, nil, err
		}
		sourceCounts[source]++
		total = sumMonthlyBookView(total, book)
		logs = append(logs, fmt.Sprintf("[期间汇总] period=%s source=%s revenue=%.2f cost=%.2f profit=%.2f", period, source, book.Revenue, book.TotalCost, book.Profit))
	}
	logs = append(logs, fmt.Sprintf("[期间汇总] aggregated_period=%s total_revenue=%.2f total_cost=%.2f total_profit=%.2f", displayPeriod(from, to), total.Revenue, total.TotalCost, total.Profit))

	validation, validationSQLs, validationLogs, err := e.validateIncomeStatementRangeTotals(from, to)
	if err != nil {
		return monthlyBookView{}, "", nil, nil, nil, err
	}
	return total, "range_monthly_book_summary(" + formatSourceCounts(sourceCounts) + ")", validation,
		append([]string{
			"range_book_summary: sum monthlyBookSummary over each period in selected range",
			"monthlyBookSummary(income_statement): SELECT item_name, current_amount FROM income_statement WHERE ... AND period = ?",
			"monthlyBookSummary(fallback_journal): ComputeMonthlyFromJournal + ComputeIncomeStatement when income_statement missing required rows",
		}, validationSQLs...),
		append(logs, validationLogs...),
		nil
}

func sumMonthlyBookView(base, add monthlyBookView) monthlyBookView {
	return monthlyBookView{
		Revenue:        round2(base.Revenue + add.Revenue),
		Cost:           round2(base.Cost + add.Cost),
		TaxSurcharge:   round2(base.TaxSurcharge + add.TaxSurcharge),
		SellingExpense: round2(base.SellingExpense + add.SellingExpense),
		AdminExpense:   round2(base.AdminExpense + add.AdminExpense),
		FinanceExpense: round2(base.FinanceExpense + add.FinanceExpense),
		TotalCost:      round2(base.TotalCost + add.TotalCost),
		Profit:         round2(base.Profit + add.Profit),
	}
}

func formatSourceCounts(sourceCounts map[string]int) string {
	if len(sourceCounts) == 0 {
		return "unknown"
	}
	keys := make([]string, 0, len(sourceCounts))
	for key := range sourceCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, sourceCounts[key]))
	}
	return strings.Join(parts, ",")
}

func periodsBetween(from, to string) ([]string, error) {
	start, err := time.Parse("2006-01", from)
	if err != nil {
		return nil, fmt.Errorf("parse start period %s: %w", from, err)
	}
	end, err := time.Parse("2006-01", to)
	if err != nil {
		return nil, fmt.Errorf("parse end period %s: %w", to, err)
	}
	if start.After(end) {
		return nil, fmt.Errorf("invalid period range %s~%s", from, to)
	}
	periods := make([]string, 0, 12)
	for current := start; !current.After(end); current = current.AddDate(0, 1, 0) {
		periods = append(periods, current.Format("2006-01"))
	}
	return periods, nil
}

type cumulativeValidationAccumulator struct {
	CurrentSum   float64
	PreviousCumu sql.NullFloat64
	LatestCumu   sql.NullFloat64
	PreviousAt   string
	LatestAt     string
}

func (e *Engine) validateIncomeStatementRangeTotals(from, to string) (map[string]any, []string, []string, error) {
	hasCumulative, err := e.tableHasColumn("income_statement", "cumulative_amount")
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("[区间校验] skipped: detect cumulative_amount failed: %v", err)}, nil
	}
	if !hasCumulative {
		return nil, nil, []string{"[区间校验] skipped: income_statement has no cumulative_amount column"}, nil
	}

	startYear, _ := parsePeriod(from)
	startBoundary := fmt.Sprintf("%04d-01", startYear)
	rows, err := e.db.Query(`
SELECT period, item_name, COALESCE(current_amount, 0), COALESCE(cumulative_amount, 0)
FROM income_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period BETWEEN ? AND ?
ORDER BY period, item_name
`, e.Company, e.Company, startBoundary, to)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	type matcher struct {
		key      string
		patterns []string
	}
	matchers := []matcher{
		{key: "revenue", patterns: []string{"营业收入", "主营业务收入", "营业总收入"}},
		{key: "cost", patterns: []string{"营业成本", "主营业务成本"}},
		{key: "selling_expense", patterns: []string{"销售费用"}},
		{key: "admin_expense", patterns: []string{"管理费用"}},
		{key: "finance_expense", patterns: []string{"财务费用"}},
		{key: "tax_surcharge", patterns: []string{"税金及附加", "营业税金及附加"}},
		{key: "profit", patterns: []string{"净利润", "利润总额"}},
	}
	accumulators := map[string]*cumulativeValidationAccumulator{}
	for _, matcher := range matchers {
		accumulators[matcher.key] = &cumulativeValidationAccumulator{}
	}

	matchedRows := 0
	for rows.Next() {
		var period string
		var itemName string
		var currentAmount float64
		var cumulativeAmount float64
		if err := rows.Scan(&period, &itemName, &currentAmount, &cumulativeAmount); err != nil {
			return nil, nil, nil, err
		}
		matchedKey := ""
		for _, matcher := range matchers {
			for _, pattern := range matcher.patterns {
				if strings.Contains(itemName, pattern) {
					matchedKey = matcher.key
					break
				}
			}
			if matchedKey != "" {
				break
			}
		}
		if matchedKey == "" {
			continue
		}
		matchedRows++
		acc := accumulators[matchedKey]
		if period >= from && period <= to {
			acc.CurrentSum = round2(acc.CurrentSum + currentAmount)
			if !acc.LatestCumu.Valid || period >= acc.LatestAt {
				acc.LatestCumu = sql.NullFloat64{Float64: cumulativeAmount, Valid: true}
				acc.LatestAt = period
			}
			continue
		}
		if period < from && (!acc.PreviousCumu.Valid || period >= acc.PreviousAt) {
			acc.PreviousCumu = sql.NullFloat64{Float64: cumulativeAmount, Valid: true}
			acc.PreviousAt = period
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if matchedRows == 0 {
		return nil, nil, []string{"[区间校验] skipped: no income_statement rows matched validation categories"}, nil
	}

	items := map[string]any{}
	passed := true
	logs := make([]string, 0, len(accumulators))
	for _, matcher := range matchers {
		acc := accumulators[matcher.key]
		if !acc.LatestCumu.Valid && acc.CurrentSum == 0 {
			continue
		}
		previous := 0.0
		if acc.PreviousCumu.Valid {
			previous = acc.PreviousCumu.Float64
		}
		cumulativeDelta := 0.0
		if acc.LatestCumu.Valid {
			cumulativeDelta = round2(acc.LatestCumu.Float64 - previous)
		}
		diff := round2(acc.CurrentSum - cumulativeDelta)
		itemPassed := math.Abs(diff) <= 1.00
		passed = passed && itemPassed
		items[matcher.key] = map[string]any{
			"current_sum":      round2(acc.CurrentSum),
			"cumulative_delta": cumulativeDelta,
			"diff":             diff,
			"passed":           itemPassed,
			"latest_period":    acc.LatestAt,
			"previous_period":  acc.PreviousAt,
		}
		logs = append(logs, fmt.Sprintf("[区间校验] item=%s current_sum=%.2f cumulative_delta=%.2f diff=%.2f passed=%t", matcher.key, acc.CurrentSum, cumulativeDelta, diff, itemPassed))
	}

	if len(items) == 0 {
		return nil, nil, []string{"[区间校验] skipped: no comparable cumulative rows in selected range"}, nil
	}
	return map[string]any{
			"basis":  "sum_current_amount_vs_cumulative_delta",
			"from":   from,
			"to":     to,
			"passed": passed,
			"items":  items,
		}, []string{
			"range_validation(income_statement): compare SUM(current_amount) with cumulative_amount delta over selected range",
		}, logs, nil
}

func (e *Engine) tableHasColumn(tableName, columnName string) (bool, error) {
	rows, err := e.db.Query(`
SELECT column_name
FROM information_schema.columns
WHERE table_name = ?
`, tableName)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return false, err
			}
			if strings.EqualFold(strings.TrimSpace(name), columnName) {
				return true, nil
			}
		}
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}

	sqliteRows, sqliteErr := e.db.Query(`PRAGMA table_info(` + tableName + `)`)
	if sqliteErr != nil {
		return false, sqliteErr
	}
	defer sqliteRows.Close()
	for sqliteRows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := sqliteRows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(strings.TrimSpace(name), columnName) {
			return true, nil
		}
	}
	return false, sqliteRows.Err()
}

func buildConsistencyGuard(raw accounting.AccrualPerspective, selected monthlyBookView, cash accounting.CashPerspective, selectedLabel string, validation map[string]any) map[string]any {
	revenueDelta := round2(selected.Revenue - raw.Revenue)
	costDelta := round2(selected.TotalCost - raw.TotalCost)
	profitDelta := round2(selected.Profit - raw.Profit)
	cashIdentityDelta := round2((cash.Income - cash.Expense) - cash.Net)
	accrualIdentityDelta := round2((selected.Revenue - selected.TotalCost) - selected.Profit)

	// 收入口径可能存在营业外收支/税费调整，允许 1 元误差。
	passed := math.Abs(cashIdentityDelta) <= 0.02 &&
		math.Abs(accrualIdentityDelta) <= 1.00
	if validationPassed, ok := validation["passed"].(bool); ok {
		passed = passed && validationPassed
	}

	guard := map[string]any{
		"passed":                 passed,
		"selected_accrual":       selectedLabel,
		"cash_identity_delta":    cashIdentityDelta,
		"accrual_identity_delta": accrualIdentityDelta,
		"source_drift": map[string]any{
			"revenue_delta": revenueDelta,
			"cost_delta":    costDelta,
			"profit_delta":  profitDelta,
		},
	}
	if validation != nil {
		guard["range_validation"] = validation
	}
	return guard
}

func buildBossDualPerspectiveMessage(period string, cash accounting.CashPerspective, accrual monthlyBookView, bridge *analysis.ProfitCashBridge) string {
	profitGap := round2(accrual.Profit - cash.Net)
	revenueTiming := round2(accrual.Revenue - cash.Income)
	costTiming := round2(cash.Expense - accrual.TotalCost)
	otherAdjustments := round2(profitGap - revenueTiming - costTiming)

	lines := []string{
		fmt.Sprintf("先说现金口径：%s 实际到账 %.2f 元，实际支出 %.2f 元，净增加 %.2f 元。", period, cash.Income, cash.Expense, cash.Net),
		fmt.Sprintf("再补经营口径：确认收入 %.2f 元，确认成本及费用 %.2f 元，利润 %.2f 元。", accrual.Revenue, accrual.TotalCost, accrual.Profit),
		fmt.Sprintf("两个口径之间，利润和净现金流相差 %.2f 元。", profitGap),
		"差异最大的3个原因：",
		fmt.Sprintf("1. 收入确认和回款时间差 %.2f 元（账上收入减去实际到账）。", revenueTiming),
		fmt.Sprintf("2. 付款和成本确认时间差 %.2f 元（实际支出减去账上成本及费用）。", costTiming),
		fmt.Sprintf("3. 其他调节项 %.2f 元（含税费/营业外收支/四舍五入等）。", otherAdjustments),
	}
	if bridge != nil {
		gapLabel := fmt.Sprintf("含税项调节后的利润桥和过滤后经营现金仍有 %.2f 元差额待继续拆分。", math.Abs(bridge.AdjustedOperatingCashGap))
		if bridge.AdjustedOperatingCashGap < 0 {
			gapLabel = fmt.Sprintf("含税项调节后的利润桥比过滤后的经营现金高 %.2f 元，说明还有营运资金或分类口径没补齐。", math.Abs(bridge.AdjustedOperatingCashGap))
		} else if bridge.AdjustedOperatingCashGap > 0 {
			gapLabel = fmt.Sprintf("过滤后的经营现金比含税项调节后的利润桥高 %.2f 元，说明还有现金分类或桥接项待核实。", math.Abs(bridge.AdjustedOperatingCashGap))
		}
		lines = append(lines,
			fmt.Sprintf("按利润调现金桥还原：净利润 %.2f + 折旧 %.2f - 应收净增加 %.2f - 预付净增加 %.2f - 其他应付款净增加 %.2f + 应付账款净增加 %.2f + 预收账款净增加 %.2f + 应付职工薪酬净增加 %.2f = 经营现金 %.2f 元。",
				bridge.NetProfit, bridge.Depreciation, bridge.ARIncrease, bridge.PrepaymentIncrease, bridge.OtherPayableIncrease, bridge.APIncrease, bridge.AdvanceReceiptIncrease, bridge.PayrollIncrease, bridge.EstimatedOperatingCash),
			fmt.Sprintf("再加税项时点调节 %.2f 元后，经营现金估算 %.2f 元。", bridge.TaxTimingAdjustment, bridge.AdjustedOperatingCashEstimate),
			fmt.Sprintf("按凭证同组科目过滤后，经营性现金净额 %.2f 元；已识别的非经营/混合现金净额 %.2f 元。", bridge.OperatingCashNet, bridge.ExcludedCashNet),
			gapLabel,
		)
		if bridge.OtherReceivableIncrease != 0 || bridge.TaxBalanceIncrease != 0 {
			lines = append(lines, fmt.Sprintf("补充披露：其他应收款净增加 %.2f 元、应交税费净变动 %.2f 元，这两项先单独列示，不直接并入经营现金桥，避免把内部往来或税项时差误当经营现金。", bridge.OtherReceivableIncrease, bridge.TaxBalanceIncrease))
		}
	}
	lines = append(lines, "建议动作：先盯未回款客户和大额支出项目，周内做一次回款与结算单对齐。")
	return strings.Join(lines, "\n")
}
