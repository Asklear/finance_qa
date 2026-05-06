package query

import (
	"context"
	"fmt"
	"strings"
)

type contractFinanceTotals struct {
	Settlement     float64
	Movement       float64
	Invoice        float64
	SettlementOpen float64
	InvoiceOpen    float64
	RowCount       int
	MonthCount     int
	ContractCount  int
}

type contractFinanceTotalsSpec struct {
	DirectTable      string
	GroupTable       string
	GroupMemberTable string
	MovementColumn   string
}

func costSettlementTotalsSpec() contractFinanceTotalsSpec {
	return contractFinanceTotalsSpec{
		DirectTable:      "fin_cost_settlements",
		GroupTable:       "fin_cost_settlement_groups",
		GroupMemberTable: "fin_cost_settlement_group_members",
		MovementColumn:   "paid_amount",
	}
}

func fundIncomeTotalsSpec() contractFinanceTotalsSpec {
	return contractFinanceTotalsSpec{
		DirectTable:      "fin_fund_income",
		GroupTable:       "fin_fund_income_groups",
		GroupMemberTable: "fin_fund_income_group_members",
		MovementColumn:   "received_amount",
	}
}

func (spec contractFinanceTotalsSpec) validate() error {
	switch {
	case strings.TrimSpace(spec.DirectTable) == "":
		return fmt.Errorf("contract finance totals direct table is required")
	case strings.TrimSpace(spec.GroupTable) == "":
		return fmt.Errorf("contract finance totals group table is required")
	case strings.TrimSpace(spec.GroupMemberTable) == "":
		return fmt.Errorf("contract finance totals group member table is required")
	case strings.TrimSpace(spec.MovementColumn) == "":
		return fmt.Errorf("contract finance totals movement column is required")
	}
	return nil
}

func (e *Engine) collectContractFinanceTotals(ctx context.Context, spec contractFinanceTotalsSpec, periodFrom, periodTo, like string) (contractFinanceTotals, error) {
	if err := spec.validate(); err != nil {
		return contractFinanceTotals{}, err
	}
	like = strings.TrimSpace(like)

	var totals contractFinanceTotals
	directSQL := fmt.Sprintf(`
SELECT COALESCE(SUM(d.settlement_amount), 0),
       COALESCE(SUM(d.%[1]s), 0),
       COALESCE(SUM(d.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(d.settlement_amount, 0) > COALESCE(d.%[1]s, 0) THEN COALESCE(d.settlement_amount, 0) - COALESCE(d.%[1]s, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(d.invoice_amount, 0) > COALESCE(d.%[1]s, 0) THEN COALESCE(d.invoice_amount, 0) - COALESCE(d.%[1]s, 0) ELSE 0 END), 0),
       COUNT(*),
       COUNT(DISTINCT d.year_month),
       COUNT(DISTINCT d.contract_id)
FROM %[2]s d
JOIN fin_contracts c ON c.contract_id = d.contract_id
WHERE d.year_month BETWEEN ? AND ?`, spec.MovementColumn, spec.DirectTable)
	directArgs := []any{periodFrom, periodTo}
	if like != "" {
		directSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		directArgs = append(directArgs, like, like)
	}
	if err := e.db.QueryRowContext(ctx, directSQL, directArgs...).Scan(
		&totals.Settlement,
		&totals.Movement,
		&totals.Invoice,
		&totals.SettlementOpen,
		&totals.InvoiceOpen,
		&totals.RowCount,
		&totals.MonthCount,
		&totals.ContractCount,
	); err != nil {
		return contractFinanceTotals{}, err
	}

	if !e.hasContractFinanceGroupTables(spec) {
		return totals, nil
	}

	groupAmountFilter := `g.year_month BETWEEN ? AND ?`
	groupAmountArgs := []any{periodFrom, periodTo}
	if like != "" {
		groupAmountFilter += ` AND g.customer_name LIKE ?`
		groupAmountArgs = append(groupAmountArgs, like)
	}

	groupSQL := fmt.Sprintf(`
SELECT COALESCE(SUM(g.settlement_amount), 0),
       COALESCE(SUM(g.%[1]s), 0),
       COALESCE(SUM(g.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(g.settlement_amount, 0) > COALESCE(g.%[1]s, 0) THEN COALESCE(g.settlement_amount, 0) - COALESCE(g.%[1]s, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(g.invoice_amount, 0) > COALESCE(g.%[1]s, 0) THEN COALESCE(g.invoice_amount, 0) - COALESCE(g.%[1]s, 0) ELSE 0 END), 0),
       COUNT(*)
FROM %[2]s g
WHERE `+groupAmountFilter, spec.MovementColumn, spec.GroupTable)
	var groupSettlement, groupMovement, groupInvoice, groupSettlementOpen, groupInvoiceOpen float64
	var groupRows int
	if err := e.db.QueryRowContext(ctx, groupSQL, groupAmountArgs...).Scan(
		&groupSettlement,
		&groupMovement,
		&groupInvoice,
		&groupSettlementOpen,
		&groupInvoiceOpen,
		&groupRows,
	); err != nil {
		return contractFinanceTotals{}, err
	}
	totals.Settlement += groupSettlement
	totals.Movement += groupMovement
	totals.Invoice += groupInvoice
	totals.SettlementOpen += groupSettlementOpen
	totals.InvoiceOpen += groupInvoiceOpen
	if like == "" {
		totals.RowCount += groupRows
	} else {
		groupCoverageRows, err := e.countContractFinanceGroupCoverageRows(ctx, spec, periodFrom, periodTo, like)
		if err != nil {
			return contractFinanceTotals{}, err
		}
		totals.RowCount += groupCoverageRows
	}

	monthCount, contractCount, err := e.countContractFinanceCoverage(ctx, spec, periodFrom, periodTo, like)
	if err != nil {
		return contractFinanceTotals{}, err
	}
	totals.MonthCount = monthCount
	totals.ContractCount = contractCount
	return totals, nil
}

func (e *Engine) hasContractFinanceGroupTables(spec contractFinanceTotalsSpec) bool {
	groupCols := e.tableColumns(spec.GroupTable)
	memberCols := e.tableColumns(spec.GroupMemberTable)
	return groupCols["id"] &&
		groupCols["year_month"] &&
		groupCols["settlement_amount"] &&
		groupCols[spec.MovementColumn] &&
		groupCols["invoice_amount"] &&
		memberCols["group_id"] &&
		memberCols["contract_id"]
}

func (e *Engine) countContractFinanceGroupCoverageRows(ctx context.Context, spec contractFinanceTotalsSpec, periodFrom, periodTo, like string) (int, error) {
	sqlText := fmt.Sprintf(`
SELECT COUNT(*)
FROM %[1]s g
WHERE g.year_month BETWEEN ? AND ?
  AND (
    g.customer_name LIKE ?
    OR EXISTS (
      SELECT 1
      FROM %[2]s gm
      JOIN fin_contracts c ON c.contract_id = gm.contract_id
      WHERE gm.group_id = g.id
        AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)
    )
  )`, spec.GroupTable, spec.GroupMemberTable)
	var count int
	if err := e.db.QueryRowContext(ctx, sqlText, periodFrom, periodTo, like, like, like).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (e *Engine) countContractFinanceCoverage(ctx context.Context, spec contractFinanceTotalsSpec, periodFrom, periodTo, like string) (int, int, error) {
	like = strings.TrimSpace(like)
	directSQL := fmt.Sprintf(`
SELECT d.year_month AS year_month, d.contract_id AS contract_id
FROM %[1]s d
JOIN fin_contracts c ON c.contract_id = d.contract_id
WHERE d.year_month BETWEEN ? AND ?`, spec.DirectTable)
	args := []any{periodFrom, periodTo}
	if like != "" {
		directSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}

	groupSQL := fmt.Sprintf(`
SELECT g.year_month AS year_month, gm.contract_id AS contract_id
FROM %[1]s g
JOIN %[2]s gm ON gm.group_id = g.id
JOIN fin_contracts c ON c.contract_id = gm.contract_id
WHERE g.year_month BETWEEN ? AND ?`, spec.GroupTable, spec.GroupMemberTable)
	args = append(args, periodFrom, periodTo)
	if like != "" {
		groupSQL += ` AND (g.customer_name LIKE ? OR c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like, like)
	}

	sqlText := `
SELECT COUNT(DISTINCT year_month), COUNT(DISTINCT contract_id)
FROM (` + directSQL + ` UNION ALL ` + groupSQL + `) contract_finance_coverage`
	var monthCount, contractCount int
	if err := e.db.QueryRowContext(ctx, sqlText, args...).Scan(&monthCount, &contractCount); err != nil {
		return 0, 0, err
	}
	return monthCount, contractCount, nil
}
