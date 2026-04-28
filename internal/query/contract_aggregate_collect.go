package query

import (
	"context"
	"fmt"
	"strings"
)

const contractAggregateRole = "aggregate_summary"

type contractAggregateSummary struct {
	Entity                  string
	Scope                   string
	Period                  string
	PeriodFrom              string
	PeriodTo                string
	RequestedMetrics        []string
	ContractCount           int
	RevenueSettlement       float64
	RevenueReceived         float64
	RevenueInvoiced         float64
	RevenueReceivable       float64
	RevenueInvoiceOpen      float64
	CostSettlement          float64
	CostPaid                float64
	CostInvoiced            float64
	CostPayable             float64
	CostInvoiceOpen         float64
	Profit                  float64
	RevenueItems            []contractAggregateOpenItem
	RevenueInvoiceOpenItems []contractAggregateOpenItem
	CostItems               []contractAggregateOpenItem
	CostInvoiceOpenItems    []contractAggregateOpenItem
	HasRevenueCoverage      bool
	HasCostCoverage         bool
	SourceTables            []string
	ExecutedSQL             []string
	CalculationLogs         []string
}

type contractAggregateOpenItem struct {
	CustomerName     string
	ContractContent  string
	SettlementAmount float64
	InvoiceAmount    float64
	ReceivedAmount   float64
	OpenAmount       float64
}

func (e *Engine) collectContractAggregateSummary(spec QuerySpec) (contractAggregateSummary, error) {
	entity := strings.TrimSpace(spec.Entity)
	if entity != "" || !shouldUseCompanyScopeContractAggregate(spec.OriginalQuestion) {
		if resolved := e.resolveContractSubject(spec.OriginalQuestion, entity); resolved != "" {
			entity = resolved
		}
	}

	scope := "company"
	entityLike := ""
	if entity != "" {
		scope = "entity"
		entityLike = "%" + entity + "%"
	}

	requestedMetrics := detectRequestedMetrics(spec.OriginalQuestion)
	needRevenue := contractAggregateNeedsRevenueData(requestedMetrics)
	needCost := contractAggregateNeedsCostData(requestedMetrics)

	summary := contractAggregateSummary{
		Entity:           entity,
		Scope:            scope,
		Period:           displayPeriod(spec.PeriodFrom, spec.PeriodTo),
		PeriodFrom:       spec.PeriodFrom,
		PeriodTo:         spec.PeriodTo,
		RequestedMetrics: requestedMetrics,
		SourceTables:     contractAggregateSourceTablesForMetrics(requestedMetrics),
	}

	if needRevenue {
		summary.ExecutedSQL = append(summary.ExecutedSQL,
			"contract_aggregate(revenue): SELECT SUM(settlement_amount), SUM(received_amount), SUM(invoice_amount), SUM(unreceived), SUM(invoiced_unreceived), COUNT(DISTINCT contract_id) FROM fin_fund_income + fin_fund_income_groups ... WHERE year_month BETWEEN ? AND ?",
		)
		revenueTotals, err := e.collectFundIncomeTotals(context.Background(), spec.PeriodFrom, spec.PeriodTo, entityLike)
		if err != nil {
			return contractAggregateSummary{}, err
		}
		summary.RevenueSettlement = revenueTotals.Settlement
		summary.RevenueReceived = revenueTotals.Received
		summary.RevenueInvoiced = revenueTotals.Invoice
		summary.RevenueReceivable = revenueTotals.Receivable
		summary.RevenueInvoiceOpen = revenueTotals.InvoiceOpen
		summary.ContractCount = revenueTotals.ContractCount
		if contractAggregateWantsDetailItems(spec.OriginalQuestion) {
			items, err := e.collectRevenueItems(spec.PeriodFrom, spec.PeriodTo, entityLike)
			if err != nil {
				return contractAggregateSummary{}, err
			}
			summary.RevenueItems = items
		}
		if contractAggregateIncludesMetric(requestedMetrics, "已开票未回款") {
			items, err := e.collectRevenueInvoiceOpenItems(spec.PeriodFrom, spec.PeriodTo, entityLike)
			if err != nil {
				return contractAggregateSummary{}, err
			}
			summary.RevenueInvoiceOpenItems = items
		}
	}

	var costContractCount int
	if needCost {
		summary.ExecutedSQL = append(summary.ExecutedSQL,
			"contract_aggregate(cost): SELECT SUM(settlement_amount), SUM(paid_amount), SUM(invoice_amount), SUM(payable), SUM(invoiced_unpaid), COUNT(DISTINCT contract_id) FROM fin_cost_settlements + fin_cost_settlement_groups ... WHERE year_month BETWEEN ? AND ?",
		)
		costTotals, err := e.collectCostSettlementTotals(context.Background(), spec.PeriodFrom, spec.PeriodTo, entityLike)
		if err != nil {
			return contractAggregateSummary{}, err
		}
		summary.CostSettlement = costTotals.Settlement
		summary.CostPaid = costTotals.Paid
		summary.CostInvoiced = costTotals.Invoice
		summary.CostPayable = costTotals.Payable
		summary.CostInvoiceOpen = costTotals.InvoiceOpen
		costContractCount = costTotals.ContractCount
		if contractAggregateWantsDetailItems(spec.OriginalQuestion) {
			items, err := e.collectCostItems(spec.PeriodFrom, spec.PeriodTo, entityLike)
			if err != nil {
				return contractAggregateSummary{}, err
			}
			summary.CostItems = items
		}
		if contractAggregateIncludesMetric(requestedMetrics, "已收票未付款") {
			items, err := e.collectCostInvoiceOpenItems(spec.PeriodFrom, spec.PeriodTo, entityLike)
			if err != nil {
				return contractAggregateSummary{}, err
			}
			summary.CostInvoiceOpenItems = items
		}
	}

	summary.RevenueSettlement = round2(summary.RevenueSettlement)
	summary.RevenueReceived = round2(summary.RevenueReceived)
	summary.RevenueInvoiced = round2(summary.RevenueInvoiced)
	summary.RevenueReceivable = round2(summary.RevenueReceivable)
	summary.RevenueInvoiceOpen = round2(summary.RevenueInvoiceOpen)
	summary.CostSettlement = round2(summary.CostSettlement)
	summary.CostPaid = round2(summary.CostPaid)
	summary.CostInvoiced = round2(summary.CostInvoiced)
	summary.CostPayable = round2(summary.CostPayable)
	summary.CostInvoiceOpen = round2(summary.CostInvoiceOpen)
	summary.Profit = round2(summary.RevenueSettlement - summary.CostSettlement)
	summary.HasRevenueCoverage = summary.ContractCount > 0
	summary.HasCostCoverage = costContractCount > 0
	if summary.ContractCount == 0 && costContractCount > 0 {
		summary.ContractCount = costContractCount
	}

	summary.CalculationLogs = append(summary.CalculationLogs,
		fmt.Sprintf("[合同汇总优先] scope=%s entity=%s period=%s revenue=%.2f received=%.2f invoice=%.2f receivable=%.2f cost=%.2f paid=%.2f payable=%.2f profit=%.2f", summary.Scope, summary.Entity, summary.Period, summary.RevenueSettlement, summary.RevenueReceived, summary.RevenueInvoiced, summary.RevenueReceivable, summary.CostSettlement, summary.CostPaid, summary.CostPayable, summary.Profit),
	)

	return summary, nil
}

func contractAggregateWantsDetailItems(question string) bool {
	return containsAny(question, []string{"明细", "列表", "有哪些", "哪些", "分别", "拆", "拆分", "构成"})
}

func (e *Engine) mergedGroupContractContentSQL(memberTable, groupAlias string) string {
	if e.usesPostgresSQL() {
		return fmt.Sprintf(`COALESCE((
	SELECT '合并行合计（覆盖合同/项目：' || STRING_AGG(COALESCE(NULLIF(TRIM(mc.contract_content), ''), mc.contract_id), '、' ORDER BY gm.source_row_number, mc.contract_id) || '）'
	FROM %s gm
	JOIN fin_contracts mc ON mc.contract_id = gm.contract_id
	WHERE gm.group_id = %s.id
), '合并行合计')`, memberTable, groupAlias)
	}
	return fmt.Sprintf(`COALESCE((
	SELECT '合并行合计（覆盖合同/项目：' || GROUP_CONCAT(COALESCE(NULLIF(TRIM(mc.contract_content), ''), mc.contract_id), '、') || '）'
	FROM %s gm
	JOIN fin_contracts mc ON mc.contract_id = gm.contract_id
	WHERE gm.group_id = %s.id
), '合并行合计')`, memberTable, groupAlias)
}

func (e *Engine) usesPostgresSQL() bool {
	dbPath := strings.ToLower(strings.TrimSpace(e.dbPath))
	return strings.Contains(dbPath, "host=") ||
		strings.HasPrefix(dbPath, "postgres://") ||
		strings.HasPrefix(dbPath, "postgresql://")
}

func (e *Engine) collectRevenueItems(periodFrom, periodTo, like string) ([]contractAggregateOpenItem, error) {
	unionSQL := `
SELECT c.customer_name,
       c.contract_content,
       COALESCE(f.settlement_amount, 0) AS settlement_amount,
       COALESCE(f.invoice_amount, 0) AS invoice_amount,
       COALESCE(f.received_amount, 0) AS received_amount,
       CASE WHEN COALESCE(f.settlement_amount, 0) > COALESCE(f.received_amount, 0) THEN COALESCE(f.settlement_amount, 0) - COALESCE(f.received_amount, 0) ELSE 0 END AS open_amount
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?`
	args := []any{periodFrom, periodTo}
	if strings.TrimSpace(like) != "" {
		unionSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}
	if e.hasFundIncomeGroupTables() {
		groupContent := e.mergedGroupContractContentSQL("fin_fund_income_group_members", "g")
		unionSQL += `
UNION ALL
SELECT g.customer_name,
       ` + groupContent + ` AS contract_content,
       COALESCE(g.settlement_amount, 0) AS settlement_amount,
       COALESCE(g.invoice_amount, 0) AS invoice_amount,
       COALESCE(g.received_amount, 0) AS received_amount,
       CASE WHEN COALESCE(g.settlement_amount, 0) > COALESCE(g.received_amount, 0) THEN COALESCE(g.settlement_amount, 0) - COALESCE(g.received_amount, 0) ELSE 0 END AS open_amount
FROM fin_fund_income_groups g
WHERE g.year_month BETWEEN ? AND ?`
		args = append(args, periodFrom, periodTo)
		if strings.TrimSpace(like) != "" {
			unionSQL += ` AND g.customer_name LIKE ?`
			args = append(args, like)
		}
	}
	sqlText := `
SELECT customer_name,
       contract_content,
       COALESCE(SUM(settlement_amount), 0),
       COALESCE(SUM(invoice_amount), 0),
       COALESCE(SUM(received_amount), 0),
       COALESCE(SUM(open_amount), 0)
FROM (` + unionSQL + `) revenue_items
GROUP BY customer_name, contract_content
ORDER BY 3 DESC, customer_name, contract_content`
	return e.collectContractAggregateItems(sqlText, args)
}

func (e *Engine) collectCostItems(periodFrom, periodTo, like string) ([]contractAggregateOpenItem, error) {
	unionSQL := `
SELECT c.customer_name,
       c.contract_content,
       COALESCE(cs.settlement_amount, 0) AS settlement_amount,
       COALESCE(cs.invoice_amount, 0) AS invoice_amount,
       COALESCE(cs.paid_amount, 0) AS paid_amount,
       CASE WHEN COALESCE(cs.settlement_amount, 0) > COALESCE(cs.paid_amount, 0) THEN COALESCE(cs.settlement_amount, 0) - COALESCE(cs.paid_amount, 0) ELSE 0 END AS open_amount
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?`
	args := []any{periodFrom, periodTo}
	if strings.TrimSpace(like) != "" {
		unionSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}
	if e.hasCostSettlementGroupTables() {
		groupContent := e.mergedGroupContractContentSQL("fin_cost_settlement_group_members", "g")
		unionSQL += `
UNION ALL
SELECT g.customer_name,
       ` + groupContent + ` AS contract_content,
       COALESCE(g.settlement_amount, 0) AS settlement_amount,
       COALESCE(g.invoice_amount, 0) AS invoice_amount,
       COALESCE(g.paid_amount, 0) AS paid_amount,
       CASE WHEN COALESCE(g.settlement_amount, 0) > COALESCE(g.paid_amount, 0) THEN COALESCE(g.settlement_amount, 0) - COALESCE(g.paid_amount, 0) ELSE 0 END AS open_amount
FROM fin_cost_settlement_groups g
WHERE g.year_month BETWEEN ? AND ?`
		args = append(args, periodFrom, periodTo)
		if strings.TrimSpace(like) != "" {
			unionSQL += ` AND g.customer_name LIKE ?`
			args = append(args, like)
		}
	}
	sqlText := `
SELECT customer_name,
       contract_content,
       COALESCE(SUM(settlement_amount), 0),
       COALESCE(SUM(invoice_amount), 0),
       COALESCE(SUM(paid_amount), 0),
       COALESCE(SUM(open_amount), 0)
FROM (` + unionSQL + `) cost_items
GROUP BY customer_name, contract_content
ORDER BY 3 DESC, customer_name, contract_content`
	return e.collectContractAggregateItems(sqlText, args)
}

func (e *Engine) collectContractAggregateItems(sqlText string, args []any) ([]contractAggregateOpenItem, error) {
	rows, err := e.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []contractAggregateOpenItem{}
	for rows.Next() {
		var item contractAggregateOpenItem
		if err := rows.Scan(&item.CustomerName, &item.ContractContent, &item.SettlementAmount, &item.InvoiceAmount, &item.ReceivedAmount, &item.OpenAmount); err != nil {
			return nil, err
		}
		item.SettlementAmount = round2(item.SettlementAmount)
		item.InvoiceAmount = round2(item.InvoiceAmount)
		item.ReceivedAmount = round2(item.ReceivedAmount)
		item.OpenAmount = round2(item.OpenAmount)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (e *Engine) collectRevenueInvoiceOpenItems(periodFrom, periodTo, like string) ([]contractAggregateOpenItem, error) {
	unionSQL := `
SELECT c.customer_name,
       c.contract_content,
       COALESCE(f.invoice_amount, 0) AS invoice_amount,
       COALESCE(f.received_amount, 0) AS received_amount,
       CASE WHEN COALESCE(f.invoice_amount, 0) > COALESCE(f.received_amount, 0) THEN COALESCE(f.invoice_amount, 0) - COALESCE(f.received_amount, 0) ELSE 0 END AS open_amount
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?`
	args := []any{periodFrom, periodTo}
	if strings.TrimSpace(like) != "" {
		unionSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}
	if e.hasFundIncomeGroupTables() {
		groupContent := e.mergedGroupContractContentSQL("fin_fund_income_group_members", "g")
		unionSQL += `
UNION ALL
SELECT g.customer_name,
       ` + groupContent + ` AS contract_content,
       COALESCE(g.invoice_amount, 0) AS invoice_amount,
       COALESCE(g.received_amount, 0) AS received_amount,
       CASE WHEN COALESCE(g.invoice_amount, 0) > COALESCE(g.received_amount, 0) THEN COALESCE(g.invoice_amount, 0) - COALESCE(g.received_amount, 0) ELSE 0 END AS open_amount
FROM fin_fund_income_groups g
WHERE g.year_month BETWEEN ? AND ?`
		args = append(args, periodFrom, periodTo)
		if strings.TrimSpace(like) != "" {
			unionSQL += ` AND g.customer_name LIKE ?`
			args = append(args, like)
		}
	}
	sqlText := `
SELECT customer_name,
       contract_content,
       COALESCE(SUM(invoice_amount), 0),
       COALESCE(SUM(received_amount), 0),
       COALESCE(SUM(open_amount), 0)
FROM (` + unionSQL + `) revenue_invoice_open_items
GROUP BY customer_name, contract_content
HAVING COALESCE(SUM(open_amount), 0) > 0
ORDER BY 5 DESC, customer_name, contract_content`
	rows, err := e.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []contractAggregateOpenItem{}
	for rows.Next() {
		var item contractAggregateOpenItem
		if err := rows.Scan(&item.CustomerName, &item.ContractContent, &item.InvoiceAmount, &item.ReceivedAmount, &item.OpenAmount); err != nil {
			return nil, err
		}
		item.InvoiceAmount = round2(item.InvoiceAmount)
		item.ReceivedAmount = round2(item.ReceivedAmount)
		item.OpenAmount = round2(item.OpenAmount)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (e *Engine) collectCostInvoiceOpenItems(periodFrom, periodTo, like string) ([]contractAggregateOpenItem, error) {
	unionSQL := `
SELECT c.customer_name,
       c.contract_content,
       COALESCE(cs.invoice_amount, 0) AS invoice_amount,
       COALESCE(cs.paid_amount, 0) AS paid_amount,
       CASE WHEN COALESCE(cs.invoice_amount, 0) > COALESCE(cs.paid_amount, 0) THEN COALESCE(cs.invoice_amount, 0) - COALESCE(cs.paid_amount, 0) ELSE 0 END AS open_amount
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?`
	args := []any{periodFrom, periodTo}
	if strings.TrimSpace(like) != "" {
		unionSQL += ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}
	if e.hasCostSettlementGroupTables() {
		groupContent := e.mergedGroupContractContentSQL("fin_cost_settlement_group_members", "g")
		unionSQL += `
UNION ALL
SELECT g.customer_name,
       ` + groupContent + ` AS contract_content,
       COALESCE(g.invoice_amount, 0) AS invoice_amount,
       COALESCE(g.paid_amount, 0) AS paid_amount,
       CASE WHEN COALESCE(g.invoice_amount, 0) > COALESCE(g.paid_amount, 0) THEN COALESCE(g.invoice_amount, 0) - COALESCE(g.paid_amount, 0) ELSE 0 END AS open_amount
FROM fin_cost_settlement_groups g
WHERE g.year_month BETWEEN ? AND ?`
		args = append(args, periodFrom, periodTo)
		if strings.TrimSpace(like) != "" {
			unionSQL += ` AND g.customer_name LIKE ?`
			args = append(args, like)
		}
	}
	sqlText := `
SELECT customer_name,
       contract_content,
       COALESCE(SUM(invoice_amount), 0),
       COALESCE(SUM(paid_amount), 0),
       COALESCE(SUM(open_amount), 0)
FROM (` + unionSQL + `) cost_invoice_open_items
GROUP BY customer_name, contract_content
HAVING COALESCE(SUM(open_amount), 0) > 0
ORDER BY 5 DESC, customer_name, contract_content`
	rows, err := e.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []contractAggregateOpenItem{}
	for rows.Next() {
		var item contractAggregateOpenItem
		if err := rows.Scan(&item.CustomerName, &item.ContractContent, &item.InvoiceAmount, &item.ReceivedAmount, &item.OpenAmount); err != nil {
			return nil, err
		}
		item.InvoiceAmount = round2(item.InvoiceAmount)
		item.ReceivedAmount = round2(item.ReceivedAmount)
		item.OpenAmount = round2(item.OpenAmount)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func contractAggregateSourceTablesForMetrics(requestedMetrics []string) []string {
	configured := getRuleConfig().ContractSourceTables(contractAggregateRole)
	tables := []string{"fin_contracts"}
	if contractAggregateNeedsRevenueData(requestedMetrics) {
		tables = append(tables, "fin_fund_income", "fin_fund_income_groups", "fin_fund_income_group_members")
	}
	if contractAggregateNeedsCostData(requestedMetrics) {
		tables = append(tables, "fin_cost_settlements", "fin_cost_settlement_groups", "fin_cost_settlement_group_members")
	}
	return filterSourceTables(configured, tables...)
}

func contractAggregateNeedsRevenueData(requestedMetrics []string) bool {
	if len(requestedMetrics) == 0 {
		return true
	}
	for _, metric := range requestedMetrics {
		switch strings.TrimSpace(metric) {
		case "收入", "利润", "应收", "已开票未回款":
			return true
		}
	}
	return false
}

func contractAggregateNeedsCostData(requestedMetrics []string) bool {
	if len(requestedMetrics) == 0 {
		return true
	}
	for _, metric := range requestedMetrics {
		switch strings.TrimSpace(metric) {
		case "成本", "利润", "应付", "已收票未付款":
			return true
		}
	}
	return false
}
