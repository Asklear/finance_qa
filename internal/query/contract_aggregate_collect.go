package query

import (
	"fmt"
	"strings"
)

const contractAggregateRole = "aggregate_summary"

type contractAggregateSummary struct {
	Entity             string
	Scope              string
	Period             string
	PeriodFrom         string
	PeriodTo           string
	RequestedMetrics   []string
	ContractCount      int
	RevenueSettlement  float64
	RevenueReceived    float64
	RevenueInvoiced    float64
	CostSettlement     float64
	CostPaid           float64
	Profit             float64
	HasRevenueCoverage bool
	HasCostCoverage    bool
	SourceTables       []string
	ExecutedSQL        []string
	CalculationLogs    []string
}

func (e *Engine) collectContractAggregateSummary(spec QuerySpec) (contractAggregateSummary, error) {
	entity := strings.TrimSpace(spec.Entity)
	if entity == "" {
		entity = e.matchContractSubjectByName(spec.OriginalQuestion)
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
			"contract_aggregate(revenue): SELECT SUM(settlement_amount), SUM(received_amount), SUM(invoice_amount), COUNT(DISTINCT contract_id) FROM fin_fund_income JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?",
		)
		revenueSQL := `
SELECT COALESCE(SUM(f.settlement_amount), 0),
       COALESCE(SUM(f.received_amount), 0),
       COALESCE(SUM(f.invoice_amount), 0),
       COUNT(DISTINCT f.contract_id)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?` + filterClause
		if err := e.db.QueryRow(revenueSQL, args...).Scan(&summary.RevenueSettlement, &summary.RevenueReceived, &summary.RevenueInvoiced, &summary.ContractCount); err != nil {
			return contractAggregateSummary{}, err
		}
	}

	var costContractCount int
	if needCost {
		summary.ExecutedSQL = append(summary.ExecutedSQL,
			"contract_aggregate(cost): SELECT SUM(settlement_amount), SUM(paid_amount), COUNT(DISTINCT contract_id) FROM fin_cost_settlements JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?",
		)
		costArgs := []any{spec.PeriodFrom, spec.PeriodTo}
		if entity != "" {
			like := "%" + entity + "%"
			costArgs = append(costArgs, like, like)
		}
		costSQL := `
SELECT COALESCE(SUM(cs.settlement_amount), 0),
       COALESCE(SUM(cs.paid_amount), 0),
       COUNT(DISTINCT cs.contract_id)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?` + filterClause
		if err := e.db.QueryRow(costSQL, costArgs...).Scan(&summary.CostSettlement, &summary.CostPaid, &costContractCount); err != nil {
			return contractAggregateSummary{}, err
		}
	}

	summary.RevenueSettlement = round2(summary.RevenueSettlement)
	summary.RevenueReceived = round2(summary.RevenueReceived)
	summary.RevenueInvoiced = round2(summary.RevenueInvoiced)
	summary.CostSettlement = round2(summary.CostSettlement)
	summary.CostPaid = round2(summary.CostPaid)
	summary.Profit = round2(summary.RevenueSettlement - summary.CostSettlement)
	summary.HasRevenueCoverage = summary.ContractCount > 0
	summary.HasCostCoverage = costContractCount > 0
	if summary.ContractCount == 0 && costContractCount > 0 {
		summary.ContractCount = costContractCount
	}

	summary.CalculationLogs = append(summary.CalculationLogs,
		fmt.Sprintf("[合同汇总优先] scope=%s entity=%s period=%s revenue=%.2f received=%.2f invoice=%.2f cost=%.2f paid=%.2f profit=%.2f", summary.Scope, summary.Entity, summary.Period, summary.RevenueSettlement, summary.RevenueReceived, summary.RevenueInvoiced, summary.CostSettlement, summary.CostPaid, summary.Profit),
	)

	return summary, nil
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
		case "收入", "利润":
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
		case "成本", "利润":
			return true
		}
	}
	return false
}
