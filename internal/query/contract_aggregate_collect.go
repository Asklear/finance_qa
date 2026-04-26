package query

import (
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
	RevenueInvoiceOpenItems []contractAggregateOpenItem
	HasRevenueCoverage      bool
	HasCostCoverage         bool
	SourceTables            []string
	ExecutedSQL             []string
	CalculationLogs         []string
}

type contractAggregateOpenItem struct {
	CustomerName    string
	ContractContent string
	InvoiceAmount   float64
	ReceivedAmount  float64
	OpenAmount      float64
}

func (e *Engine) collectContractAggregateSummary(spec QuerySpec) (contractAggregateSummary, error) {
	entity := strings.TrimSpace(spec.Entity)
	if resolved := e.resolveContractSubject(spec.OriginalQuestion, entity); resolved != "" {
		entity = resolved
	}

	scope := "company"
	filterClause := ""
	args := []any{spec.PeriodFrom, spec.PeriodTo}
	if entity != "" {
		scope = "entity"
		filterClause = " AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)"
		like := "%" + entity + "%"
		args = append(args, like, like)
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
			"contract_aggregate(revenue): SELECT SUM(settlement_amount), SUM(received_amount), SUM(invoice_amount), SUM(unreceived), SUM(invoiced_unreceived), COUNT(DISTINCT contract_id) FROM fin_fund_income JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?",
		)
		revenueSQL := `
SELECT COALESCE(SUM(f.settlement_amount), 0),
       COALESCE(SUM(f.received_amount), 0),
       COALESCE(SUM(f.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(f.settlement_amount, 0) > COALESCE(f.received_amount, 0) THEN COALESCE(f.settlement_amount, 0) - COALESCE(f.received_amount, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(f.invoice_amount, 0) > COALESCE(f.received_amount, 0) THEN COALESCE(f.invoice_amount, 0) - COALESCE(f.received_amount, 0) ELSE 0 END), 0),
       COUNT(DISTINCT f.contract_id)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?` + filterClause
		if err := e.db.QueryRow(revenueSQL, args...).Scan(&summary.RevenueSettlement, &summary.RevenueReceived, &summary.RevenueInvoiced, &summary.RevenueReceivable, &summary.RevenueInvoiceOpen, &summary.ContractCount); err != nil {
			return contractAggregateSummary{}, err
		}
		if contractAggregateIncludesMetric(requestedMetrics, "已开票未回款") {
			items, err := e.collectRevenueInvoiceOpenItems(filterClause, args)
			if err != nil {
				return contractAggregateSummary{}, err
			}
			summary.RevenueInvoiceOpenItems = items
		}
	}

	var costContractCount int
	if needCost {
		summary.ExecutedSQL = append(summary.ExecutedSQL,
			"contract_aggregate(cost): SELECT SUM(settlement_amount), SUM(paid_amount), SUM(invoice_amount), SUM(payable), SUM(invoiced_unpaid), COUNT(DISTINCT contract_id) FROM fin_cost_settlements JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?",
		)
		costArgs := []any{spec.PeriodFrom, spec.PeriodTo}
		if entity != "" {
			like := "%" + entity + "%"
			costArgs = append(costArgs, like, like)
		}
		costSQL := `
SELECT COALESCE(SUM(cs.settlement_amount), 0),
       COALESCE(SUM(cs.paid_amount), 0),
       COALESCE(SUM(cs.invoice_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(cs.settlement_amount, 0) > COALESCE(cs.paid_amount, 0) THEN COALESCE(cs.settlement_amount, 0) - COALESCE(cs.paid_amount, 0) ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN COALESCE(cs.invoice_amount, 0) > COALESCE(cs.paid_amount, 0) THEN COALESCE(cs.invoice_amount, 0) - COALESCE(cs.paid_amount, 0) ELSE 0 END), 0),
       COUNT(DISTINCT cs.contract_id)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?` + filterClause
		if err := e.db.QueryRow(costSQL, costArgs...).Scan(&summary.CostSettlement, &summary.CostPaid, &summary.CostInvoiced, &summary.CostPayable, &summary.CostInvoiceOpen, &costContractCount); err != nil {
			return contractAggregateSummary{}, err
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

func (e *Engine) collectRevenueInvoiceOpenItems(filterClause string, args []any) ([]contractAggregateOpenItem, error) {
	sqlText := `
SELECT c.customer_name,
       c.contract_content,
       COALESCE(SUM(f.invoice_amount), 0),
       COALESCE(SUM(f.received_amount), 0),
       COALESCE(SUM(CASE WHEN COALESCE(f.invoice_amount, 0) > COALESCE(f.received_amount, 0) THEN COALESCE(f.invoice_amount, 0) - COALESCE(f.received_amount, 0) ELSE 0 END), 0)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?` + filterClause + `
GROUP BY c.customer_name, c.contract_content
HAVING COALESCE(SUM(CASE WHEN COALESCE(f.invoice_amount, 0) > COALESCE(f.received_amount, 0) THEN COALESCE(f.invoice_amount, 0) - COALESCE(f.received_amount, 0) ELSE 0 END), 0) > 0
ORDER BY 5 DESC, c.customer_name, c.contract_content`
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
		tables = append(tables, "fin_fund_income")
	}
	if contractAggregateNeedsCostData(requestedMetrics) {
		tables = append(tables, "fin_cost_settlements")
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
