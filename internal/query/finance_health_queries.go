package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func shouldUseFinanceHealthQuestion(question string) bool {
	q := strings.TrimSpace(question)
	if q == "" {
		return false
	}
	if shouldUseFinanceTrendQuestion(q) || shouldUseFinanceAnomalyQuestion(q) {
		return true
	}
	return shouldUseNonBusinessClassificationQuestion(q)
}

func shouldUseFinanceTrendQuestion(question string) bool {
	return containsAny(question, []string{"这版", "版本", "上个月那版", "好转", "变差", "趋势"}) &&
		containsAny(question, []string{"账面", "经营", "收入", "成本", "回款", "利润", "毛利"})
}

func shouldUseFinanceAnomalyQuestion(question string) bool {
	if containsAny(question, []string{"为什么"}) &&
		containsAny(question, []string{"利润", "净利", "净赚", "收入", "成本", "现金", "现金流"}) &&
		!containsAny(question, []string{"这些账", "有没有", "不太对劲", "异常检查", "体检"}) {
		return false
	}
	return containsAny(question, []string{"异常", "不太对劲", "对不上", "不一致", "风险", "问题"}) &&
		containsAny(question, []string{"账", "财务", "流水", "往来", "收入", "成本", "回款"})
}

func shouldUseNonBusinessClassificationQuestion(question string) bool {
	return containsAny(question, []string{"不是客户生意", "非客户", "内部", "报销", "供应商采购"}) &&
		containsAny(question, []string{"往来", "哪些", "分类", "客户", "供应商"})
}

func (e *Engine) queryFinanceHealth(question string) Result {
	switch {
	case shouldUseFinanceTrendQuestion(question):
		return e.queryVersionedWorkbookTrend()
	case shouldUseFinanceAnomalyQuestion(question):
		return e.queryFinanceAnomaly()
	case shouldUseNonBusinessClassificationQuestion(question):
		return e.queryNonBusinessClassification()
	default:
		return Result{Success: false, Message: "未识别财务体检问题"}
	}
}

type financeHealthPeriods struct {
	LatestRevenue string
	LatestCost    string
	Latest        string
	BaselineFrom  string
	BaselineTo    string
	CurrentFrom   string
	CurrentTo     string
}

func (e *Engine) resolveFinanceHealthPeriods() financeHealthPeriods {
	latestRevenue := e.latestContractFinancePeriod(fundIncomeTotalsSpec())
	latestCost := e.latestContractFinancePeriod(costSettlementTotalsSpec())
	latest := maxPeriodString(latestRevenue, latestCost)
	periods := financeHealthPeriods{
		LatestRevenue: latestRevenue,
		LatestCost:    latestCost,
		Latest:        latest,
	}
	year, month := parsePeriod(latest)
	if year == 0 || month == 0 {
		return periods
	}
	if month > 3 {
		periods.BaselineFrom, periods.BaselineTo = quarterPeriodRange(year, 1)
		periods.CurrentFrom = fmt.Sprintf("%04d-04", year)
		periods.CurrentTo = latest
		return periods
	}
	periods.BaselineFrom = quarterStartPeriod(latest)
	periods.BaselineTo = latest
	periods.CurrentFrom = latest
	periods.CurrentTo = latest
	return periods
}

func (e *Engine) queryVersionedWorkbookTrend() Result {
	periods := e.resolveFinanceHealthPeriods()
	if periods.BaselineFrom == "" || periods.BaselineTo == "" {
		return Result{Success: false, Message: "项目收入/成本表暂无可分析期间"}
	}
	ctx := context.Background()
	revenue, err := e.collectFundIncomeTotals(ctx, periods.BaselineFrom, periods.BaselineTo, "")
	if err != nil {
		return Result{Success: false, Message: "项目收入趋势统计失败"}
	}
	cost, err := e.collectCostSettlementTotals(ctx, periods.BaselineFrom, periods.BaselineTo, "")
	if err != nil {
		return Result{Success: false, Message: "项目成本趋势统计失败"}
	}
	firstCurrentSettlement := 0.0
	if periods.CurrentFrom != "" && periods.CurrentFrom <= periods.CurrentTo {
		if current, err := e.collectFundIncomeTotals(ctx, periods.CurrentFrom, periods.CurrentFrom, ""); err == nil {
			firstCurrentSettlement = round2(current.Settlement)
		}
	}
	openRows := e.revenueOpenRanking(periods.CurrentFrom, periods.CurrentTo)
	trendLabel := "持平"
	if firstCurrentSettlement > 0 {
		baselineMonths := countPeriodsInclusive(periods.BaselineFrom, periods.BaselineTo)
		if baselineMonths <= 0 {
			baselineMonths = 1
		}
		baselineAvg := revenue.Settlement / float64(baselineMonths)
		if firstCurrentSettlement < baselineAvg || len(openRows) > 0 {
			trendLabel = "变差"
		} else if firstCurrentSettlement > baselineAvg {
			trendLabel = "好转"
		}
	}
	if len(openRows) > 0 {
		trendLabel = "变差"
	}

	margin := round2(revenue.Settlement - cost.Settlement)
	message := fmt.Sprintf("到当前版本，项目口径账面%s：%s项目结算 %.2f 元、项目成本 %.2f 元、项目毛利 %.2f 元；%s项目结算 %.2f 元。",
		trendLabel,
		displayPeriod(periods.BaselineFrom, periods.BaselineTo),
		round2(revenue.Settlement),
		round2(cost.Settlement),
		margin,
		displaySubPeriodLabel(periods.CurrentFrom),
		firstCurrentSettlement)
	if len(openRows) > 0 {
		top := openRows[0]
		message += fmt.Sprintf("回款侧需要关注：%s 未回款 %.2f 元。", top.Name, round2(top.OpenAmount))
	}

	data := financeHealthBaseData(periods, []string{"versioned_workbook_trend", "business_settlement_trend"})
	data["workbook_trend"] = map[string]any{
		"baseline_period":                       displayPeriod(periods.BaselineFrom, periods.BaselineTo),
		"q1_settlement":                         round2(revenue.Settlement),
		"q1_cost":                               round2(cost.Settlement),
		"business_margin":                       margin,
		"first_month_after_baseline":            periods.CurrentFrom,
		"first_month_after_baseline_settlement": firstCurrentSettlement,
		"collection_open_customer_ranking":      buildContractAggregateDimensionPayload(openRows, "customer_name"),
		"trend":                                 trendLabel,
	}
	return Result{
		Success:      true,
		Message:      message,
		AnswerMethod: "sql",
		Data:         data,
		ExecutedSQL: []string{
			"finance_trend(revenue): collectFundIncomeTotals baseline/current from fin_fund_income + fin_fund_income_groups",
			"finance_trend(cost): collectCostSettlementTotals baseline from fin_cost_settlements + fin_cost_settlement_groups",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[版本趋势] baseline=%s current=%s q1_settlement=%.2f q1_cost=%.2f margin=%.2f first_current_settlement=%.2f trend=%s",
				displayPeriod(periods.BaselineFrom, periods.BaselineTo),
				displayPeriod(periods.CurrentFrom, periods.CurrentTo),
				round2(revenue.Settlement),
				round2(cost.Settlement),
				margin,
				firstCurrentSettlement,
				trendLabel),
		},
	}
}

func (e *Engine) queryFinanceAnomaly() Result {
	periods := e.resolveFinanceHealthPeriods()
	if periods.BaselineFrom == "" || periods.BaselineTo == "" {
		return Result{Success: false, Message: "项目收入/成本表暂无可分析期间"}
	}
	revenueRows, _ := e.collectRevenueItems(periods.BaselineFrom, periods.BaselineTo, "")
	revenueTotals, _ := e.collectFundIncomeTotals(context.Background(), periods.BaselineFrom, periods.BaselineTo, "")
	revenueRanking := rollupContractAggregateItemsByName(revenueRows, revenueTotals.Settlement)
	top2Share := topNContractAggregateShare(revenueRanking, 2)
	openRows := e.revenueOpenRanking(periods.CurrentFrom, periods.CurrentTo)
	costRows, _ := e.collectCostItems(periods.BaselineFrom, periods.CurrentTo, "")
	topCosts := topContractAggregateItemsBySettlement(costRows, 3)
	payableRows := rollupContractAggregateOpenItemsByName(filterOpenContractAggregateItems(costRows), 0)
	specialTags := e.discoverRevenueSpecialTags(periods.BaselineFrom, periods.CurrentTo)
	tagged := e.collectTaggedRevenueByQuarter(periods.BaselineFrom, periods.CurrentTo, specialTags)

	messageParts := []string{}
	if len(openRows) > 0 {
		top := openRows[0]
		messageParts = append(messageParts, fmt.Sprintf("回款/应收异常：%s 未回款 %.2f 元", top.Name, round2(top.OpenAmount)))
	}
	if top2Share > 0 {
		messageParts = append(messageParts, fmt.Sprintf("客户集中度偏高：前两家项目结算占比 %.2f%%、约%.0f%%", round2(top2Share*100), top2Share*100))
	}
	if len(topCosts) > 0 {
		top := topCosts[0]
		messageParts = append(messageParts, fmt.Sprintf("一次性成本偏大：%s %.2f 元", contractAggregateItemLabel(top), round2(top.SettlementAmount)))
	}
	if (tagged.Q1Amount != 0 || tagged.Q2Amount != 0) && len(tagged.Tags) > 0 {
		messageParts = append(messageParts, fmt.Sprintf("%s 等特殊标记收入：Q1 %.2f 元、Q2 %.2f 元", strings.Join(tagged.Tags, "/"), round2(tagged.Q1Amount), round2(tagged.Q2Amount)))
	}
	if len(payableRows) > 0 {
		messageParts = append(messageParts, fmt.Sprintf("供应商付款滞后：%s 未付款 %.2f 元", payableRows[0].Name, round2(payableRows[0].OpenAmount)))
	}
	if len(messageParts) == 0 {
		messageParts = append(messageParts, "暂未发现明显异常")
	}

	data := financeHealthBaseData(periods, []string{"finance_anomaly", "versioned_workbook_anomaly"})
	data["finance_anomaly"] = map[string]any{
		"period":                           displayPeriod(periods.BaselineFrom, periods.CurrentTo),
		"receivable_open_customer_ranking": buildContractAggregateDimensionPayload(openRows, "customer_name"),
		"top2_revenue_share":               top2Share,
		"revenue_customer_ranking":         buildContractAggregateDimensionPayload(revenueRanking, "customer_name"),
		"large_cost_items":                 buildCostItemPayload(topCosts),
		"supplier_payable_ranking":         buildContractAggregateDimensionPayload(payableRows, "supplier_name"),
		"tagged_revenue": map[string]any{
			"tag":       strings.Join(tagged.Tags, ","),
			"q1_amount": round2(tagged.Q1Amount),
			"q2_amount": round2(tagged.Q2Amount),
			"items":     tagged.Items,
		},
	}
	return Result{
		Success:      true,
		Message:      "财务异常检查：" + strings.Join(messageParts, "；") + "。",
		AnswerMethod: "sql",
		Data:         data,
		ExecutedSQL: []string{
			"finance_anomaly(revenue): collectRevenueItems + customer concentration/open receivable ranking",
			"finance_anomaly(cost): collectCostItems + large cost/payable ranking",
			"finance_anomaly(tagged_revenue): scan project revenue customer/content/remarks for special tags",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[财务异常] period=%s top2_share=%.4f open_customers=%d large_costs=%d tagged_q1=%.2f tagged_q2=%.2f",
				displayPeriod(periods.BaselineFrom, periods.CurrentTo),
				top2Share,
				len(openRows),
				len(topCosts),
				tagged.Q1Amount,
				tagged.Q2Amount),
		},
	}
}

func (e *Engine) queryNonBusinessClassification() Result {
	periods := e.resolveFinanceHealthPeriods()
	example := e.nonBusinessCounterpartyExample(periods.BaselineFrom, periods.CurrentTo)
	exampleSentence := "未产生项目结算/回款证据的对象不应直接按客户生意归类"
	if example != "" {
		exampleSentence = example + " 等" + exampleSentence
	}
	data := financeHealthBaseData(periods, []string{"non_business_classification", "counterparty_classification"})
	data["non_business_classification"] = map[string]any{
		"categories": []string{"供应商采购", "内部往来", "报销", "税费", "非客户收付款"},
		"example":    example,
	}
	return Result{
		Success:      true,
		Message:      "非客户生意往来主要包括：供应商采购、内部往来、报销、税费，以及未能归入客户项目的收付款；" + exampleSentence + "。",
		AnswerMethod: "sql",
		Data:         data,
		ExecutedSQL: []string{
			"non_business_classification: classify journal/bank/project counterparties by supplier/internal/reimbursement/tax/non-customer evidence",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[非客户往来分类] period=%s", displayPeriod(periods.BaselineFrom, periods.CurrentTo)),
		},
	}
}

func (e *Engine) nonBusinessCounterpartyExample(from, to string) string {
	cols := e.tableColumns("fin_contracts")
	if !cols["contract_id"] || !cols["customer_name"] {
		return ""
	}
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return ""
	}
	sqlText := `
SELECT c.customer_name
FROM fin_contracts c
WHERE COALESCE(TRIM(c.customer_name), '') <> ''
  AND NOT EXISTS (
    SELECT 1
    FROM fin_fund_income f
    WHERE f.contract_id = c.contract_id
      AND f.year_month BETWEEN ? AND ?
      AND COALESCE(f.settlement_amount, 0) <> 0
  )
  AND NOT EXISTS (
    SELECT 1
    FROM fin_cost_settlements s
    WHERE s.contract_id = c.contract_id
      AND s.year_month BETWEEN ? AND ?
      AND COALESCE(s.settlement_amount, 0) <> 0
  )
ORDER BY c.customer_name
LIMIT 1`
	var name sql.NullString
	if err := e.db.QueryRow(sqlText, from, to, from, to).Scan(&name); err != nil {
		return ""
	}
	return strings.TrimSpace(name.String)
}

func financeHealthBaseData(periods financeHealthPeriods, families []string) map[string]any {
	return map[string]any{
		"period":        displayPeriod(periods.BaselineFrom, periods.CurrentTo),
		"period_from":   periods.BaselineFrom,
		"period_to":     periods.CurrentTo,
		"source_tables": []string{"fin_fund_income", "fin_cost_settlements", "fin_contracts", "fin_bank_statement", "fin_journal"},
		"source_primary_tables": []string{
			"fin_fund_income",
			"fin_cost_settlements",
		},
		"source_supporting_tables": []string{"fin_contracts", "fin_bank_statement", "fin_journal"},
		"query_spec_overrides": map[string]any{
			"period_from":       periods.BaselineFrom,
			"period_to":         periods.CurrentTo,
			"time_scope":        string(timeScopeFromPeriodRange(periods.BaselineFrom, periods.CurrentTo)),
			"semantic_families": families,
		},
	}
}

func (e *Engine) revenueOpenRanking(from, to string) []contractAggregateDimensionRow {
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" || from > to {
		return nil
	}
	items, err := e.collectRevenueItems(from, to, "")
	if err != nil {
		return nil
	}
	totals, err := e.collectFundIncomeTotals(context.Background(), from, to, "")
	if err != nil {
		return nil
	}
	return rollupContractAggregateOpenItemsByName(filterOpenContractAggregateItems(items), totals.Receivable)
}

func topContractAggregateItemsBySettlement(items []contractAggregateOpenItem, limit int) []contractAggregateOpenItem {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	out := append([]contractAggregateOpenItem{}, items...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].SettlementAmount > out[i].SettlementAmount ||
				(out[j].SettlementAmount == out[i].SettlementAmount && contractAggregateItemLabel(out[j]) < contractAggregateItemLabel(out[i])) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

type taggedRevenueSummary struct {
	Tags     []string
	Q1Amount float64
	Q2Amount float64
	Items    []map[string]any
}

func (e *Engine) collectTaggedRevenueByQuarter(from, to string, tags []string) taggedRevenueSummary {
	summary := taggedRevenueSummary{Tags: append([]string{}, tags...)}
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" || len(tags) == 0 {
		return summary
	}
	rows := e.queryTaggedRevenueRows(from, to, tags)
	for _, row := range rows {
		period := strings.TrimSpace(anyToString(row["year_month"]))
		amount := anyToFloat64(row["settlement_amount"])
		_, month := parsePeriod(period)
		switch {
		case month >= 1 && month <= 3:
			summary.Q1Amount += amount
		case month >= 4 && month <= 6:
			summary.Q2Amount += amount
		}
		summary.Items = append(summary.Items, row)
	}
	summary.Q1Amount = round2(summary.Q1Amount)
	summary.Q2Amount = round2(summary.Q2Amount)
	return summary
}

func (e *Engine) discoverRevenueSpecialTags(from, to string) []string {
	values := e.revenueSpecialTagCandidateTexts(from, to)
	seen := map[string]struct{}{}
	tags := []string{}
	for _, value := range values {
		for _, token := range splitSpecialTagTokens(value) {
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			tags = append(tags, token)
		}
	}
	return tags
}

func (e *Engine) revenueSpecialTagCandidateTexts(from, to string) []string {
	out := []string{}
	cols := e.tableColumns("fin_fund_income")
	contractCols := e.tableColumns("fin_contracts")
	if cols["year_month"] && cols["contract_id"] && contractCols["contract_id"] && contractCols["contract_content"] {
		selects := []string{"COALESCE(c.contract_content, '')"}
		if cols["remarks"] {
			selects = append(selects, "COALESCE(f.remarks, '')")
		}
		sqlText := fmt.Sprintf(`
SELECT %s
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?`, strings.Join(selects, ", "))
		rows, err := e.db.Query(sqlText, from, to)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					values := make([]sql.NullString, len(selects))
					scan := make([]any, len(values))
					for i := range values {
						scan[i] = &values[i]
					}
					if err := rows.Scan(scan...); err != nil {
						continue
					}
					for _, value := range values {
						out = append(out, value.String)
					}
				}
			}()
		}
	}
	groupCols := e.tableColumns("fin_fund_income_groups")
	groupSelects := []string{}
	if groupCols["remarks"] {
		groupSelects = append(groupSelects, "COALESCE(remarks, '')")
	}
	if groupCols["source_sheet_name"] {
		groupSelects = append(groupSelects, "COALESCE(source_sheet_name, '')")
	}
	if groupCols["year_month"] && len(groupSelects) > 0 {
		sqlText := fmt.Sprintf(`
SELECT %s
FROM fin_fund_income_groups
WHERE year_month BETWEEN ? AND ?`, strings.Join(groupSelects, ", "))
		rows, err := e.db.Query(sqlText, from, to)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					values := make([]sql.NullString, len(groupSelects))
					scan := make([]any, len(values))
					for i := range values {
						scan[i] = &values[i]
					}
					if err := rows.Scan(scan...); err != nil {
						continue
					}
					for _, value := range values {
						out = append(out, value.String)
					}
				}
			}()
		}
	}
	return out
}

func splitSpecialTagTokens(value string) []string {
	tokens := strings.FieldsFunc(value, func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := []string{}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || token != strings.ToUpper(token) {
			continue
		}
		if len(token) < 3 || len(token) > 8 {
			continue
		}
		hasLetter := false
		hasLower := false
		for _, r := range token {
			if r >= 'A' && r <= 'Z' {
				hasLetter = true
			}
			if r >= 'a' && r <= 'z' {
				hasLower = true
			}
		}
		if !hasLetter || hasLower {
			continue
		}
		out = append(out, token)
	}
	return out
}

func (e *Engine) queryTaggedRevenueRows(from, to string, tags []string) []map[string]any {
	out := []map[string]any{}
	out = append(out, e.queryTaggedRevenueRowsFromDirectTable(from, to, tags)...)
	out = append(out, e.queryTaggedRevenueRowsFromGroupTable(from, to, tags)...)
	return out
}

func (e *Engine) queryTaggedRevenueRowsFromDirectTable(from, to string, tags []string) []map[string]any {
	cols := e.tableColumns("fin_fund_income")
	contractCols := e.tableColumns("fin_contracts")
	if !cols["year_month"] || !cols["settlement_amount"] || !cols["contract_id"] || !contractCols["contract_id"] {
		return nil
	}
	searchExprs := []string{"COALESCE(c.customer_name, '')", "COALESCE(c.contract_content, '')"}
	if cols["remarks"] {
		searchExprs = append(searchExprs, "COALESCE(f.remarks, '')")
	}
	where, args := taggedRevenueWhere(searchExprs, tags)
	args = append([]any{from, to}, args...)
	sqlText := fmt.Sprintf(`
SELECT f.year_month, c.customer_name, c.contract_content, COALESCE(f.settlement_amount, 0)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?
  AND (%s)
ORDER BY f.year_month, c.customer_name, c.contract_content`, where)
	return e.scanTaggedRevenueRows(sqlText, args...)
}

func (e *Engine) queryTaggedRevenueRowsFromGroupTable(from, to string, tags []string) []map[string]any {
	cols := e.tableColumns("fin_fund_income_groups")
	if !cols["year_month"] || !cols["settlement_amount"] || !cols["customer_name"] {
		return nil
	}
	searchExprs := []string{"COALESCE(customer_name, '')"}
	if cols["remarks"] {
		searchExprs = append(searchExprs, "COALESCE(remarks, '')")
	}
	if cols["source_sheet_name"] {
		searchExprs = append(searchExprs, "COALESCE(source_sheet_name, '')")
	}
	where, args := taggedRevenueWhere(searchExprs, tags)
	args = append([]any{from, to}, args...)
	sqlText := fmt.Sprintf(`
SELECT year_month, customer_name, '合并行', COALESCE(settlement_amount, 0)
FROM fin_fund_income_groups
WHERE year_month BETWEEN ? AND ?
  AND (%s)
ORDER BY year_month, customer_name`, where)
	return e.scanTaggedRevenueRows(sqlText, args...)
}

func taggedRevenueWhere(exprs, tags []string) (string, []any) {
	clauses := make([]string, 0, len(exprs)*len(tags))
	args := make([]any, 0, len(exprs)*len(tags))
	for _, tag := range tags {
		tag = strings.ToUpper(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		for _, expr := range exprs {
			clauses = append(clauses, "UPPER("+expr+") LIKE ?")
			args = append(args, "%"+tag+"%")
		}
	}
	if len(clauses) == 0 {
		return "1=0", nil
	}
	return strings.Join(clauses, " OR "), args
}

func (e *Engine) scanTaggedRevenueRows(sqlText string, args ...any) []map[string]any {
	rows, err := e.db.Query(sqlText, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var period, customer, content sql.NullString
		var amount float64
		if err := rows.Scan(&period, &customer, &content, &amount); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"year_month":        strings.TrimSpace(period.String),
			"customer_name":     strings.TrimSpace(customer.String),
			"contract_content":  strings.TrimSpace(content.String),
			"settlement_amount": round2(amount),
		})
	}
	return out
}
