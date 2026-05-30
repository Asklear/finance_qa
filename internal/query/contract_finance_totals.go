package query

import (
	"context"
	"fmt"
	"strings"
)

type contractFinanceTotals struct {
	Settlement                   float64
	Movement                     float64
	Invoice                      float64
	UnattributedInvoice          float64
	UnattributedInvoiceContracts []contractDimensionRow
	SettlementOpen               float64
	InvoiceOpen                  float64
	RowCount                     int
	MonthCount                   int
	ContractCount                int
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
	directOffsetExpr := e.contractFinanceInvoiceOpenOffsetExpr("d", spec.DirectTable)
	directSQL := fmt.Sprintf(`
SELECT COALESCE(SUM(d.settlement_amount), 0),
       COALESCE(SUM(d.%[1]s), 0),
       COALESCE(SUM(d.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(d.settlement_amount, 0) > COALESCE(d.%[1]s, 0) THEN COALESCE(d.settlement_amount, 0) - COALESCE(d.%[1]s, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(d.invoice_amount, 0) > COALESCE(d.%[1]s, 0) + %[3]s THEN COALESCE(d.invoice_amount, 0) - COALESCE(d.%[1]s, 0) - %[3]s ELSE 0 END), 0),
       COUNT(*),
       COUNT(DISTINCT d.year_month),
       COUNT(DISTINCT d.contract_id)
FROM %[2]s d
JOIN fin_contracts c ON c.contract_id = d.contract_id
WHERE d.year_month BETWEEN ? AND ?`, spec.MovementColumn, spec.DirectTable, directOffsetExpr)
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

	groupAmountFilter, groupAmountArgs := e.contractFinanceGroupAmountFilter(spec, periodFrom, periodTo, like)

	groupInvoicePredicate, groupInvoiceArgs := e.contractFinanceGroupInvoiceAttributionPredicate(spec, like)
	groupSQL := fmt.Sprintf(`
SELECT COALESCE(SUM(g.settlement_amount), 0),
       COALESCE(SUM(g.%[1]s), 0),
       COALESCE(SUM(CASE WHEN %[3]s THEN g.invoice_amount ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN NOT (%[3]s) THEN g.invoice_amount ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(g.settlement_amount, 0) > COALESCE(g.%[1]s, 0) THEN COALESCE(g.settlement_amount, 0) - COALESCE(g.%[1]s, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN %[3]s AND COALESCE(g.invoice_amount, 0) > COALESCE(g.%[1]s, 0) THEN COALESCE(g.invoice_amount, 0) - COALESCE(g.%[1]s, 0) ELSE 0 END), 0),
       COUNT(*)
FROM %[2]s g
WHERE `+groupAmountFilter, spec.MovementColumn, spec.GroupTable, groupInvoicePredicate)
	groupQueryArgs := append([]any{}, groupInvoiceArgs...)
	groupQueryArgs = append(groupQueryArgs, groupInvoiceArgs...)
	groupQueryArgs = append(groupQueryArgs, groupInvoiceArgs...)
	groupQueryArgs = append(groupQueryArgs, groupAmountArgs...)
	var groupSettlement, groupMovement, groupInvoice, groupUnattributedInvoice, groupSettlementOpen, groupInvoiceOpen float64
	var groupRows int
	if err := e.db.QueryRowContext(ctx, groupSQL, groupQueryArgs...).Scan(
		&groupSettlement,
		&groupMovement,
		&groupInvoice,
		&groupUnattributedInvoice,
		&groupSettlementOpen,
		&groupInvoiceOpen,
		&groupRows,
	); err != nil {
		return contractFinanceTotals{}, err
	}
	totals.Settlement += groupSettlement
	totals.Movement += groupMovement
	totals.Invoice += groupInvoice
	totals.UnattributedInvoice += groupUnattributedInvoice
	if groupUnattributedInvoice > 0 {
		contracts, err := e.collectContractFinanceUnattributedInvoiceContracts(ctx, spec, periodFrom, periodTo, like)
		if err != nil {
			return contractFinanceTotals{}, err
		}
		totals.UnattributedInvoiceContracts = contracts
	}
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
	settlementOpen, invoiceOpen, err := e.collectContractFinanceOpenTotals(ctx, spec, periodFrom, periodTo, like)
	if err != nil {
		return contractFinanceTotals{}, err
	}
	totals.SettlementOpen = settlementOpen
	totals.InvoiceOpen = invoiceOpen
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

func (e *Engine) contractFinanceGroupOwnerPredicate(spec contractFinanceTotalsSpec) string {
	if e.hasContractFinanceGroupOwnerColumns(spec) {
		return "gm.source_row_number = g.source_start_row"
	}
	return "1=1"
}

func (e *Engine) hasContractFinanceGroupOwnerColumns(spec contractFinanceTotalsSpec) bool {
	groupCols := e.tableColumns(spec.GroupTable)
	memberCols := e.tableColumns(spec.GroupMemberTable)
	return groupCols["source_start_row"] && memberCols["source_row_number"]
}

func (e *Engine) contractFinanceGroupAmountFilter(spec contractFinanceTotalsSpec, periodFrom, periodTo, like string) (string, []any) {
	filter := `g.year_month BETWEEN ? AND ?`
	args := []any{periodFrom, periodTo}
	if strings.TrimSpace(like) == "" {
		return filter, args
	}

	filter += fmt.Sprintf(` AND (
    g.customer_name LIKE ?
    OR EXISTS (
      SELECT 1
      FROM %[1]s gm
      JOIN fin_contracts c ON c.contract_id = gm.contract_id
      WHERE gm.group_id = g.id
        AND %[2]s
        AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)
    )
  )`, spec.GroupMemberTable, e.contractFinanceGroupOwnerPredicate(spec))
	args = append(args, like, like, like)
	return filter, args
}

func (e *Engine) contractFinanceGroupInvoiceAttributionPredicate(spec contractFinanceTotalsSpec, like string) (string, []any) {
	if strings.TrimSpace(like) == "" {
		return "1=1", nil
	}
	return fmt.Sprintf(`(g.customer_name LIKE ? OR (SELECT COUNT(*) FROM %[1]s gm_attr WHERE gm_attr.group_id = g.id) <= 1)`, spec.GroupMemberTable), []any{like}
}

func (e *Engine) contractFinanceInvoiceOpenOffsetExpr(alias, tableName string) string {
	if e.tableColumns(tableName)["invoice_open_offset_amount"] {
		return fmt.Sprintf("COALESCE(%s.invoice_open_offset_amount, 0)", alias)
	}
	return "0"
}

func (e *Engine) collectContractFinanceOpenTotals(ctx context.Context, spec contractFinanceTotalsSpec, periodFrom, periodTo, like string) (float64, float64, error) {
	groupAmountFilter, groupAmountArgs := e.contractFinanceGroupAmountFilter(spec, periodFrom, periodTo, like)
	groupInvoicePredicate, groupInvoiceArgs := e.contractFinanceGroupInvoiceAttributionPredicate(spec, like)
	directOffsetExpr := e.contractFinanceInvoiceOpenOffsetExpr("d", spec.DirectTable)
	groupOffsetExpr := e.contractFinanceInvoiceOpenOffsetExpr("g", spec.GroupTable)
	directFilter := `d.year_month BETWEEN ? AND ?`
	directArgs := []any{periodFrom, periodTo}
	if strings.TrimSpace(like) != "" {
		directFilter += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		directArgs = append(directArgs, like, like)
	}
	sqlText := fmt.Sprintf(`
WITH selected_groups AS (
	SELECT g.id, g.year_month
	FROM %[2]s g
	WHERE `+groupAmountFilter+`
),
direct_rows AS (
	SELECT d.contract_id,
	       d.year_month,
	       COALESCE(d.settlement_amount, 0) AS settlement_amount,
	       COALESCE(d.%[1]s, 0) AS movement_amount,
	       COALESCE(d.invoice_amount, 0) AS invoice_amount,
	       %[6]s AS invoice_open_offset_amount
	FROM %[3]s d
	JOIN fin_contracts c ON c.contract_id = d.contract_id
	WHERE `+directFilter+`
),
uncovered_direct_rows AS (
	SELECT d.settlement_amount, d.movement_amount, d.invoice_amount, d.invoice_open_offset_amount
	FROM direct_rows d
	WHERE NOT EXISTS (
		SELECT 1
		FROM selected_groups sg
		JOIN %[4]s gm ON gm.group_id = sg.id
		WHERE gm.contract_id = d.contract_id
		  AND sg.year_month = d.year_month
	)
),
group_scope_rows AS (
	SELECT COALESCE(g.settlement_amount, 0) + COALESCE(SUM(d.settlement_amount), 0) AS settlement_amount,
	       COALESCE(g.%[1]s, 0) + COALESCE(SUM(d.movement_amount), 0) AS movement_amount,
	       CASE WHEN %[5]s THEN COALESCE(g.invoice_amount, 0) ELSE 0 END + COALESCE(SUM(d.invoice_amount), 0) AS invoice_amount,
	       %[7]s + COALESCE(SUM(d.invoice_open_offset_amount), 0) AS invoice_open_offset_amount
	FROM %[2]s g
	JOIN selected_groups sg ON sg.id = g.id
	LEFT JOIN %[4]s gm ON gm.group_id = g.id
	LEFT JOIN direct_rows d ON d.contract_id = gm.contract_id AND d.year_month = g.year_month
	GROUP BY g.id, g.settlement_amount, g.%[1]s, g.invoice_amount, g.year_month, g.customer_name
),
open_rows AS (
	SELECT settlement_amount, movement_amount, invoice_amount, invoice_open_offset_amount FROM uncovered_direct_rows
	UNION ALL
	SELECT settlement_amount, movement_amount, invoice_amount, invoice_open_offset_amount FROM group_scope_rows
)
SELECT COALESCE(SUM(CASE WHEN settlement_amount > movement_amount THEN settlement_amount - movement_amount ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN invoice_amount > movement_amount + invoice_open_offset_amount THEN invoice_amount - movement_amount - invoice_open_offset_amount ELSE 0 END), 0)
FROM open_rows`, spec.MovementColumn, spec.GroupTable, spec.DirectTable, spec.GroupMemberTable, groupInvoicePredicate, directOffsetExpr, groupOffsetExpr)
	args := append([]any{}, groupAmountArgs...)
	args = append(args, directArgs...)
	args = append(args, groupInvoiceArgs...)
	var settlementOpen, invoiceOpen float64
	if err := e.db.QueryRowContext(ctx, sqlText, args...).Scan(&settlementOpen, &invoiceOpen); err != nil {
		return 0, 0, err
	}
	return settlementOpen, invoiceOpen, nil
}

func (e *Engine) collectContractFinanceUnattributedInvoiceContracts(ctx context.Context, spec contractFinanceTotalsSpec, periodFrom, periodTo, like string) ([]contractDimensionRow, error) {
	like = strings.TrimSpace(like)
	if like == "" {
		return nil, nil
	}
	groupAmountFilter, groupAmountArgs := e.contractFinanceGroupAmountFilter(spec, periodFrom, periodTo, like)
	groupInvoicePredicate, groupInvoiceArgs := e.contractFinanceGroupInvoiceAttributionPredicate(spec, like)
	sourceRowExpr := "0"
	orderExpr := "source_row_order"
	if e.tableColumns(spec.GroupMemberTable)["source_row_number"] {
		sourceRowExpr = "MIN(COALESCE(gm.source_row_number, 0))"
	}
	sqlText := fmt.Sprintf(`
SELECT c.contract_id, c.customer_name, c.contract_content, %[3]s AS source_row_order
FROM %[1]s g
JOIN %[2]s gm ON gm.group_id = g.id
JOIN fin_contracts c ON c.contract_id = gm.contract_id
WHERE `+groupAmountFilter+`
  AND NOT (`+groupInvoicePredicate+`)
  AND COALESCE(g.invoice_amount, 0) > 0
GROUP BY c.contract_id, c.customer_name, c.contract_content
ORDER BY %[4]s, c.contract_id`, spec.GroupTable, spec.GroupMemberTable, sourceRowExpr, orderExpr)
	args := append([]any{}, groupAmountArgs...)
	args = append(args, groupInvoiceArgs...)
	rows, err := e.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]contractDimensionRow, 0)
	for rows.Next() {
		var row contractDimensionRow
		var sourceRowOrder int
		if err := rows.Scan(&row.ContractID, &row.CustomerName, &row.ContractContent, &sourceRowOrder); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *Engine) countContractFinanceGroupCoverageRows(ctx context.Context, spec contractFinanceTotalsSpec, periodFrom, periodTo, like string) (int, error) {
	groupAmountFilter, args := e.contractFinanceGroupAmountFilter(spec, periodFrom, periodTo, like)
	sqlText := fmt.Sprintf(`
SELECT COUNT(*)
FROM %[1]s g
WHERE `+groupAmountFilter, spec.GroupTable)
	var count int
	if err := e.db.QueryRowContext(ctx, sqlText, args...).Scan(&count); err != nil {
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
		groupSQL += fmt.Sprintf(` AND (g.customer_name LIKE ? OR (%s AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)))`, e.contractFinanceGroupOwnerPredicate(spec))
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
