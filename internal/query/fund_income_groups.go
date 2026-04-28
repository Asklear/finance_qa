package query

import (
	"context"
	"strings"
)

type fundIncomeTotals struct {
	Settlement    float64
	Received      float64
	Invoice       float64
	Receivable    float64
	InvoiceOpen   float64
	RowCount      int
	MonthCount    int
	ContractCount int
}

func (e *Engine) collectFundIncomeTotals(ctx context.Context, periodFrom, periodTo, like string) (fundIncomeTotals, error) {
	var totals fundIncomeTotals
	like = strings.TrimSpace(like)

	directSQL := `
SELECT COALESCE(SUM(f.settlement_amount), 0),
       COALESCE(SUM(f.received_amount), 0),
       COALESCE(SUM(f.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(f.settlement_amount, 0) > COALESCE(f.received_amount, 0) THEN COALESCE(f.settlement_amount, 0) - COALESCE(f.received_amount, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(f.invoice_amount, 0) > COALESCE(f.received_amount, 0) THEN COALESCE(f.invoice_amount, 0) - COALESCE(f.received_amount, 0) ELSE 0 END), 0),
       COUNT(*),
       COUNT(DISTINCT f.year_month),
       COUNT(DISTINCT f.contract_id)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?`
	directArgs := []any{periodFrom, periodTo}
	if like != "" {
		directSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		directArgs = append(directArgs, like, like)
	}
	if err := e.db.QueryRowContext(ctx, directSQL, directArgs...).Scan(
		&totals.Settlement,
		&totals.Received,
		&totals.Invoice,
		&totals.Receivable,
		&totals.InvoiceOpen,
		&totals.RowCount,
		&totals.MonthCount,
		&totals.ContractCount,
	); err != nil {
		return fundIncomeTotals{}, err
	}

	if !e.hasFundIncomeGroupTables() {
		return totals, nil
	}

	groupAmountFilter := `g.year_month BETWEEN ? AND ?`
	groupAmountArgs := []any{periodFrom, periodTo}
	if like != "" {
		groupAmountFilter += ` AND g.customer_name LIKE ?`
		groupAmountArgs = append(groupAmountArgs, like)
	}

	groupSQL := `
SELECT COALESCE(SUM(g.settlement_amount), 0),
       COALESCE(SUM(g.received_amount), 0),
       COALESCE(SUM(g.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(g.settlement_amount, 0) > COALESCE(g.received_amount, 0) THEN COALESCE(g.settlement_amount, 0) - COALESCE(g.received_amount, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(g.invoice_amount, 0) > COALESCE(g.received_amount, 0) THEN COALESCE(g.invoice_amount, 0) - COALESCE(g.received_amount, 0) ELSE 0 END), 0),
       COUNT(*)
FROM fin_fund_income_groups g
WHERE ` + groupAmountFilter
	var groupSettlement, groupReceived, groupInvoice, groupReceivable, groupInvoiceOpen float64
	var groupRows int
	if err := e.db.QueryRowContext(ctx, groupSQL, groupAmountArgs...).Scan(
		&groupSettlement,
		&groupReceived,
		&groupInvoice,
		&groupReceivable,
		&groupInvoiceOpen,
		&groupRows,
	); err != nil {
		return fundIncomeTotals{}, err
	}
	totals.Settlement += groupSettlement
	totals.Received += groupReceived
	totals.Invoice += groupInvoice
	totals.Receivable += groupReceivable
	totals.InvoiceOpen += groupInvoiceOpen
	if like == "" {
		totals.RowCount += groupRows
	} else {
		groupCoverageRows, err := e.countFundIncomeGroupCoverageRows(ctx, periodFrom, periodTo, like)
		if err != nil {
			return fundIncomeTotals{}, err
		}
		totals.RowCount += groupCoverageRows
	}
	monthCount, contractCount, err := e.countFundIncomeCoverage(ctx, periodFrom, periodTo, like)
	if err != nil {
		return fundIncomeTotals{}, err
	}
	totals.MonthCount = monthCount
	totals.ContractCount = contractCount
	return totals, nil
}

func (e *Engine) countFundIncomeGroupCoverageRows(ctx context.Context, periodFrom, periodTo, like string) (int, error) {
	sqlText := `
SELECT COUNT(*)
FROM fin_fund_income_groups g
WHERE g.year_month BETWEEN ? AND ?
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
	var count int
	if err := e.db.QueryRowContext(ctx, sqlText, periodFrom, periodTo, like, like, like).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (e *Engine) countFundIncomeCoverage(ctx context.Context, periodFrom, periodTo, like string) (int, int, error) {
	like = strings.TrimSpace(like)
	directSQL := `
SELECT f.year_month AS year_month, f.contract_id AS contract_id
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?`
	args := []any{periodFrom, periodTo}
	if like != "" {
		directSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}

	groupSQL := `
SELECT g.year_month AS year_month, gm.contract_id AS contract_id
FROM fin_fund_income_groups g
JOIN fin_fund_income_group_members gm ON gm.group_id = g.id
JOIN fin_contracts c ON c.contract_id = gm.contract_id
WHERE g.year_month BETWEEN ? AND ?`
	args = append(args, periodFrom, periodTo)
	if like != "" {
		groupSQL += ` AND (g.customer_name LIKE ? OR c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like, like)
	}

	sqlText := `
SELECT COUNT(DISTINCT year_month), COUNT(DISTINCT contract_id)
FROM (` + directSQL + ` UNION ALL ` + groupSQL + `) fund_income_coverage`
	var monthCount, contractCount int
	if err := e.db.QueryRowContext(ctx, sqlText, args...).Scan(&monthCount, &contractCount); err != nil {
		return 0, 0, err
	}
	return monthCount, contractCount, nil
}

func (e *Engine) hasFundIncomeGroupTables() bool {
	groupCols := e.tableColumns("fin_fund_income_groups")
	memberCols := e.tableColumns("fin_fund_income_group_members")
	return groupCols["id"] &&
		groupCols["year_month"] &&
		groupCols["settlement_amount"] &&
		groupCols["received_amount"] &&
		groupCols["invoice_amount"] &&
		memberCols["group_id"] &&
		memberCols["contract_id"]
}
