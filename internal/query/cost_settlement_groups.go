package query

import (
	"context"
	"strings"
)

type costSettlementTotals struct {
	Settlement    float64
	Paid          float64
	Invoice       float64
	Payable       float64
	InvoiceOpen   float64
	RowCount      int
	MonthCount    int
	ContractCount int
}

func (e *Engine) collectCostSettlementTotals(ctx context.Context, periodFrom, periodTo, like string) (costSettlementTotals, error) {
	var totals costSettlementTotals
	like = strings.TrimSpace(like)

	directSQL := `
SELECT COALESCE(SUM(cs.settlement_amount), 0),
       COALESCE(SUM(cs.paid_amount), 0),
       COALESCE(SUM(cs.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(cs.settlement_amount, 0) > COALESCE(cs.paid_amount, 0) THEN COALESCE(cs.settlement_amount, 0) - COALESCE(cs.paid_amount, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(cs.invoice_amount, 0) > COALESCE(cs.paid_amount, 0) THEN COALESCE(cs.invoice_amount, 0) - COALESCE(cs.paid_amount, 0) ELSE 0 END), 0),
       COUNT(*),
       COUNT(DISTINCT cs.year_month),
       COUNT(DISTINCT cs.contract_id)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?`
	directArgs := []any{periodFrom, periodTo}
	if like != "" {
		directSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		directArgs = append(directArgs, like, like)
	}
	if err := e.db.QueryRowContext(ctx, directSQL, directArgs...).Scan(
		&totals.Settlement,
		&totals.Paid,
		&totals.Invoice,
		&totals.Payable,
		&totals.InvoiceOpen,
		&totals.RowCount,
		&totals.MonthCount,
		&totals.ContractCount,
	); err != nil {
		return costSettlementTotals{}, err
	}

	if !e.hasCostSettlementGroupTables() {
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
       COALESCE(SUM(g.paid_amount), 0),
       COALESCE(SUM(g.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(g.settlement_amount, 0) > COALESCE(g.paid_amount, 0) THEN COALESCE(g.settlement_amount, 0) - COALESCE(g.paid_amount, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(g.invoice_amount, 0) > COALESCE(g.paid_amount, 0) THEN COALESCE(g.invoice_amount, 0) - COALESCE(g.paid_amount, 0) ELSE 0 END), 0),
       COUNT(*)
FROM fin_cost_settlement_groups g
WHERE ` + groupAmountFilter
	var groupSettlement, groupPaid, groupInvoice, groupPayable, groupInvoiceOpen float64
	var groupRows int
	if err := e.db.QueryRowContext(ctx, groupSQL, groupAmountArgs...).Scan(
		&groupSettlement,
		&groupPaid,
		&groupInvoice,
		&groupPayable,
		&groupInvoiceOpen,
		&groupRows,
	); err != nil {
		return costSettlementTotals{}, err
	}
	totals.Settlement += groupSettlement
	totals.Paid += groupPaid
	totals.Invoice += groupInvoice
	totals.Payable += groupPayable
	totals.InvoiceOpen += groupInvoiceOpen
	if like == "" {
		totals.RowCount += groupRows
	} else {
		groupCoverageRows, err := e.countCostSettlementGroupCoverageRows(ctx, periodFrom, periodTo, like)
		if err != nil {
			return costSettlementTotals{}, err
		}
		totals.RowCount += groupCoverageRows
	}
	monthCount, contractCount, err := e.countCostSettlementCoverage(ctx, periodFrom, periodTo, like)
	if err != nil {
		return costSettlementTotals{}, err
	}
	totals.MonthCount = monthCount
	totals.ContractCount = contractCount
	return totals, nil
}

func (e *Engine) countCostSettlementGroupCoverageRows(ctx context.Context, periodFrom, periodTo, like string) (int, error) {
	sqlText := `
SELECT COUNT(*)
FROM fin_cost_settlement_groups g
WHERE g.year_month BETWEEN ? AND ?
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
	var count int
	if err := e.db.QueryRowContext(ctx, sqlText, periodFrom, periodTo, like, like, like).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (e *Engine) countCostSettlementCoverage(ctx context.Context, periodFrom, periodTo, like string) (int, int, error) {
	like = strings.TrimSpace(like)
	directSQL := `
SELECT cs.year_month AS year_month, cs.contract_id AS contract_id
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?`
	args := []any{periodFrom, periodTo}
	if like != "" {
		directSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}

	groupSQL := `
SELECT g.year_month AS year_month, gm.contract_id AS contract_id
FROM fin_cost_settlement_groups g
JOIN fin_cost_settlement_group_members gm ON gm.group_id = g.id
JOIN fin_contracts c ON c.contract_id = gm.contract_id
WHERE g.year_month BETWEEN ? AND ?`
	args = append(args, periodFrom, periodTo)
	if like != "" {
		groupSQL += ` AND (g.customer_name LIKE ? OR c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like, like)
	}

	sqlText := `
SELECT COUNT(DISTINCT year_month), COUNT(DISTINCT contract_id)
FROM (` + directSQL + ` UNION ALL ` + groupSQL + `) cost_settlement_coverage`
	var monthCount, contractCount int
	if err := e.db.QueryRowContext(ctx, sqlText, args...).Scan(&monthCount, &contractCount); err != nil {
		return 0, 0, err
	}
	return monthCount, contractCount, nil
}

func (e *Engine) hasCostSettlementGroupTables() bool {
	groupCols := e.tableColumns("fin_cost_settlement_groups")
	memberCols := e.tableColumns("fin_cost_settlement_group_members")
	return groupCols["id"] &&
		groupCols["year_month"] &&
		groupCols["settlement_amount"] &&
		groupCols["paid_amount"] &&
		groupCols["invoice_amount"] &&
		memberCols["group_id"] &&
		memberCols["contract_id"]
}
