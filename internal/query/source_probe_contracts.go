package query

import (
	"context"
	"fmt"
	"strings"
)

func (e *Engine) probeContractAggregate(ctx context.Context, rewrite BossQueryRewrite) SourceProbeResult {
	switch rewrite.Metric {
	case BossMetricARAP:
		revenue := e.probeContractAmount(ctx, rewrite, "fin_fund_income", "settlement_amount", "合同应收")
		cost := e.probeContractAmount(ctx, rewrite, "fin_cost_settlements", "settlement_amount", "合同应付")
		return combineContractARAPProbe(rewrite, revenue, cost)
	case BossMetricCost, BossMetricPayments:
		return e.probeContractAmount(ctx, rewrite, "fin_cost_settlements", "settlement_amount", "成本")
	case BossMetricProfit:
		revenue := e.probeContractAmount(ctx, rewrite, "fin_fund_income", "settlement_amount", "收入")
		cost := e.probeContractAmount(ctx, rewrite, "fin_cost_settlements", "settlement_amount", "成本")
		return combineContractProfitProbe(rewrite, revenue, cost)
	case BossMetricInvoice:
		revenue := e.probeContractAmount(ctx, rewrite, "fin_fund_income", "invoice_amount", "收入开票")
		if revenue.CanAnswer {
			return revenue
		}
		cost := e.probeContractAmount(ctx, rewrite, "fin_cost_settlements", "invoice_amount", "成本开票")
		if cost.CanAnswer {
			return cost
		}
		return combineContractInvoiceMissingProbe(rewrite, revenue, cost)
	default:
		return e.probeContractAmount(ctx, rewrite, "fin_fund_income", "settlement_amount", "收入")
	}
}

func combineContractARAPProbe(rewrite BossQueryRewrite, revenue, cost SourceProbeResult) SourceProbeResult {
	result := SourceProbeResult{
		Source:          BossSourceContractAggregate,
		SemanticMatch:   revenue.SemanticMatch || cost.SemanticMatch,
		Metric:          BossMetricARAP,
		PeriodFrom:      rewrite.PeriodFrom,
		PeriodTo:        rewrite.PeriodTo,
		PrimaryTables:   dedupeStrings(append(revenue.PrimaryTables, cost.PrimaryTables...)),
		SourceDocuments: combineProbeDocuments(revenue, cost),
		CoverageStatus:  CoverageMissing,
	}
	result.RowCount = revenue.RowCount + cost.RowCount
	if revenue.CanAnswer || cost.CanAnswer {
		result.CanAnswer = true
		if revenue.CoverageStatus == CoverageFull || cost.CoverageStatus == CoverageFull {
			result.CoverageStatus = CoverageFull
		} else {
			result.CoverageStatus = CoveragePartial
		}
		return result
	}
	result.MissingReason = "合同应收/应付口径在请求期间 " + displayPeriod(rewrite.PeriodFrom, rewrite.PeriodTo) + " 没有匹配记录"
	return result
}

func (e *Engine) probeContractAmount(ctx context.Context, rewrite BossQueryRewrite, tableName, amountColumn, label string) SourceProbeResult {
	primaryTables := []string{"fin_contracts", tableName}
	result := SourceProbeResult{
		Source:         BossSourceContractAggregate,
		SemanticMatch:  true,
		Metric:         rewrite.Metric,
		PeriodFrom:     rewrite.PeriodFrom,
		PeriodTo:       rewrite.PeriodTo,
		PrimaryTables:  primaryTables,
		CoverageStatus: CoverageMissing,
	}
	result.SourceDocuments = e.sourceDocumentsForBossProbe(ctx, rewrite, primaryTables)

	cols := e.tableColumns(tableName)
	required := []string{"year_month", amountColumn}
	if strings.TrimSpace(rewrite.Entity) != "" {
		required = append(required, "contract_id")
		if missing := missingColumns(e.tableColumns("fin_contracts"), []string{"contract_id", "customer_name", "contract_content"}); len(missing) > 0 {
			result.MissingReason = "合同信息表缺少字段：" + strings.Join(missing, "、")
			return result
		}
	}
	if missing := missingColumns(cols, required); len(missing) > 0 {
		result.MissingReason = label + "表缺少字段：" + strings.Join(missing, "、")
		return result
	}

	rowCount, monthCount, err := e.countContractAmountRows(ctx, rewrite, tableName, amountColumn)
	if err != nil {
		result.MissingReason = label + "合同口径探测失败：" + err.Error()
		return result
	}
	result.RowCount = rowCount
	if rowCount == 0 {
		result.MissingReason = label + "合同口径在请求期间 " + displayPeriod(rewrite.PeriodFrom, rewrite.PeriodTo) + " 没有匹配记录"
		return result
	}

	result.CanAnswer = true
	if expected := countPeriodsInclusive(rewrite.PeriodFrom, rewrite.PeriodTo); expected > 0 && monthCount < expected {
		result.CoverageStatus = CoveragePartial
		return result
	}
	result.CoverageStatus = CoverageFull
	return result
}

func (e *Engine) countContractAmountRows(ctx context.Context, rewrite BossQueryRewrite, tableName, amountColumn string) (int, int, error) {
	if tableName == "fin_fund_income" && e.hasFundIncomeGroupTables() && isFundIncomeAmountColumn(amountColumn) {
		return e.countFundIncomeAmountRows(ctx, rewrite, amountColumn)
	}
	if tableName == "fin_cost_settlements" && e.hasCostSettlementGroupTables() && isCostSettlementAmountColumn(amountColumn) {
		return e.countCostSettlementAmountRows(ctx, rewrite, amountColumn)
	}

	entity := strings.TrimSpace(rewrite.Entity)
	if entity == "" {
		sqlText := fmt.Sprintf(`
SELECT COUNT(*), COUNT(DISTINCT year_month)
FROM %s
WHERE year_month BETWEEN ? AND ?
  AND %s IS NOT NULL
`, tableName, amountColumn)
		var rows, months int
		err := e.db.QueryRowContext(ctx, sqlText, rewrite.PeriodFrom, rewrite.PeriodTo).Scan(&rows, &months)
		return rows, months, err
	}

	sqlText := fmt.Sprintf(`
SELECT COUNT(*), COUNT(DISTINCT f.year_month)
FROM %s f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?
  AND f.%s IS NOT NULL
  AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)
`, tableName, amountColumn)
	like := "%" + entity + "%"
	var rows, months int
	err := e.db.QueryRowContext(ctx, sqlText, rewrite.PeriodFrom, rewrite.PeriodTo, like, like).Scan(&rows, &months)
	return rows, months, err
}

func isCostSettlementAmountColumn(column string) bool {
	switch strings.TrimSpace(column) {
	case "settlement_amount", "paid_amount", "invoice_amount":
		return true
	default:
		return false
	}
}

func (e *Engine) countCostSettlementAmountRows(ctx context.Context, rewrite BossQueryRewrite, amountColumn string) (int, int, error) {
	entity := strings.TrimSpace(rewrite.Entity)
	directSQL := fmt.Sprintf(`
SELECT cs.year_month AS year_month
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?
  AND cs.%s IS NOT NULL`, amountColumn)
	args := []any{rewrite.PeriodFrom, rewrite.PeriodTo}
	if entity != "" {
		directSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		like := "%" + entity + "%"
		args = append(args, like, like)
	}

	groupSQL := fmt.Sprintf(`
SELECT g.year_month AS year_month
FROM fin_cost_settlement_groups g
WHERE g.year_month BETWEEN ? AND ?
  AND g.%s IS NOT NULL`, amountColumn)
	args = append(args, rewrite.PeriodFrom, rewrite.PeriodTo)
	if entity != "" {
		like := "%" + entity + "%"
		groupSQL += `
  AND (
    g.customer_name LIKE ?
    OR EXISTS (
      SELECT 1
      FROM fin_cost_settlement_group_members gm
      JOIN fin_contracts c ON c.contract_id = gm.contract_id
      WHERE gm.group_id = g.id
        AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)
    )
  )`
		args = append(args, like, like, like)
	}

	sqlText := `
SELECT COUNT(*), COUNT(DISTINCT year_month)
FROM (` + directSQL + ` UNION ALL ` + groupSQL + `) cost_settlement_amount_rows`
	var rows, months int
	err := e.db.QueryRowContext(ctx, sqlText, args...).Scan(&rows, &months)
	return rows, months, err
}

func isFundIncomeAmountColumn(column string) bool {
	switch strings.TrimSpace(column) {
	case "settlement_amount", "received_amount", "invoice_amount":
		return true
	default:
		return false
	}
}

func (e *Engine) countFundIncomeAmountRows(ctx context.Context, rewrite BossQueryRewrite, amountColumn string) (int, int, error) {
	entity := strings.TrimSpace(rewrite.Entity)
	directSQL := fmt.Sprintf(`
SELECT f.year_month AS year_month
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?
  AND f.%s IS NOT NULL`, amountColumn)
	args := []any{rewrite.PeriodFrom, rewrite.PeriodTo}
	if entity != "" {
		directSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		like := "%" + entity + "%"
		args = append(args, like, like)
	}

	groupSQL := fmt.Sprintf(`
SELECT g.year_month AS year_month
FROM fin_fund_income_groups g
WHERE g.year_month BETWEEN ? AND ?
  AND g.%s IS NOT NULL`, amountColumn)
	args = append(args, rewrite.PeriodFrom, rewrite.PeriodTo)
	if entity != "" {
		like := "%" + entity + "%"
		groupSQL += `
  AND (
    g.customer_name LIKE ?
    OR EXISTS (
      SELECT 1
      FROM fin_fund_income_group_members gm
      JOIN fin_contracts c ON c.contract_id = gm.contract_id
      WHERE gm.group_id = g.id
        AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)
    )
  )`
		args = append(args, like, like, like)
	}

	sqlText := `
SELECT COUNT(*), COUNT(DISTINCT year_month)
FROM (` + directSQL + ` UNION ALL ` + groupSQL + `) fund_income_amount_rows`
	var rows, months int
	err := e.db.QueryRowContext(ctx, sqlText, args...).Scan(&rows, &months)
	return rows, months, err
}

func combineContractProfitProbe(rewrite BossQueryRewrite, revenue, cost SourceProbeResult) SourceProbeResult {
	result := SourceProbeResult{
		Source:           BossSourceContractAggregate,
		SemanticMatch:    revenue.SemanticMatch || cost.SemanticMatch,
		Metric:           BossMetricProfit,
		PeriodFrom:       rewrite.PeriodFrom,
		PeriodTo:         rewrite.PeriodTo,
		PrimaryTables:    dedupeStrings(append(revenue.PrimaryTables, cost.PrimaryTables...)),
		SourceDocuments:  combineProbeDocuments(revenue, cost),
		CoverageStatus:   CoverageMissing,
		SupportingTables: nil,
	}
	result.RowCount = revenue.RowCount + cost.RowCount
	missing := []string{}
	if !revenue.CanAnswer {
		missing = append(missing, "收入")
	}
	if !cost.CanAnswer {
		missing = append(missing, "成本")
	}
	if len(missing) > 0 {
		result.MissingReason = "合同利润需要同时具备" + strings.Join(missing, "和") + "覆盖"
		return result
	}
	result.CanAnswer = true
	if revenue.CoverageStatus == CoverageFull && cost.CoverageStatus == CoverageFull {
		result.CoverageStatus = CoverageFull
	} else {
		result.CoverageStatus = CoveragePartial
	}
	return result
}

func combineContractInvoiceMissingProbe(rewrite BossQueryRewrite, revenue, cost SourceProbeResult) SourceProbeResult {
	return SourceProbeResult{
		Source:          BossSourceContractAggregate,
		SemanticMatch:   true,
		CanAnswer:       false,
		CoverageStatus:  CoverageMissing,
		Metric:          rewrite.Metric,
		PeriodFrom:      rewrite.PeriodFrom,
		PeriodTo:        rewrite.PeriodTo,
		RowCount:        revenue.RowCount + cost.RowCount,
		MissingReason:   "合同开票口径在请求期间 " + displayPeriod(rewrite.PeriodFrom, rewrite.PeriodTo) + " 没有匹配记录",
		PrimaryTables:   dedupeStrings(append(revenue.PrimaryTables, cost.PrimaryTables...)),
		SourceDocuments: combineProbeDocuments(revenue, cost),
	}
}

func countPeriodsInclusive(from, to string) int {
	fromYear, fromMonth := parsePeriod(from)
	toYear, toMonth := parsePeriod(to)
	if fromYear == 0 || fromMonth == 0 || toYear == 0 || toMonth == 0 {
		return 0
	}
	return (toYear-fromYear)*12 + (toMonth - fromMonth) + 1
}

func (e *Engine) hasAnyContractLedgerRows(ctx context.Context, tables []string) bool {
	for _, tableName := range tables {
		base := baseSourceTableName(tableName)
		switch base {
		case "fin_contracts", "fin_fund_income", "fin_cost_settlements":
		default:
			continue
		}
		if len(e.tableColumns(base)) == 0 {
			continue
		}
		var count int
		if err := e.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", base)).Scan(&count); err == nil && count > 0 {
			return true
		}
	}
	return false
}
