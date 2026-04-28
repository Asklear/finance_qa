package query

import (
	"context"
	"fmt"
	"strings"
)

type ReadinessSourceAdapter struct {
	engine *Engine
}

type readinessSummary struct {
	Entity           string
	Period           string
	PeriodFrom       string
	PeriodTo         string
	HasData          bool
	Rows             int
	JournalRows      int
	BankRows         int
	ContractRows     int
	ContractFundRows int
	ContractCostRows int
	ExecutedSQL      []string
	Logs             []string
}

func NewReadinessSourceAdapter(engine *Engine) *ReadinessSourceAdapter {
	return &ReadinessSourceAdapter{engine: engine}
}

func (a *ReadinessSourceAdapter) Name() string {
	return "data_readiness"
}

func (a *ReadinessSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{SourceCapabilityDataReadiness}
}

func (a *ReadinessSourceAdapter) Fetch(_ context.Context, spec QuerySpec) (FactSet, error) {
	summary, err := a.engine.collectEntityDataReadiness(spec.Entity, spec.PeriodFrom, spec.PeriodTo)
	if err != nil {
		return FactSet{}, err
	}
	return buildReadinessFactSet(spec, summary), nil
}

func (e *Engine) collectEntityDataReadiness(entity, from, to string) (readinessSummary, error) {
	sqlJournal := `SELECT COUNT(*) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND (summary LIKE ? OR counterparty LIKE ?) AND voucher_date BETWEEN ? AND ?`
	sqlBank := `SELECT COUNT(*) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND counterparty_name LIKE ? AND transaction_date BETWEEN ? AND ?`
	var jCnt, bCnt int
	entityLike := "%" + entity + "%"
	startDate := from + "-01"
	endDate := monthEndDay(to)
	if err := e.db.QueryRow(sqlJournal, e.Company, e.Company, entityLike, entityLike, startDate, endDate).Scan(&jCnt); err != nil {
		return readinessSummary{}, err
	}
	if err := e.db.QueryRow(sqlBank, e.Company, e.Company, entityLike, startDate, endDate).Scan(&bCnt); err != nil {
		return readinessSummary{}, err
	}

	contractRows, contractFundRows, contractCostRows, contractSQLs, contractLogs, err := e.collectContractDataReadiness(entity, from, to)
	if err != nil {
		return readinessSummary{}, err
	}
	total := jCnt + bCnt + contractFundRows + contractCostRows
	return readinessSummary{
		Entity:           entity,
		Period:           to,
		PeriodFrom:       from,
		PeriodTo:         to,
		HasData:          total > 0,
		Rows:             total,
		JournalRows:      jCnt,
		BankRows:         bCnt,
		ContractRows:     contractRows,
		ContractFundRows: contractFundRows,
		ContractCostRows: contractCostRows,
		ExecutedSQL: append([]string{
			fmt.Sprintf("queryEntityDataReady(journal): %s [args: %s, %s, %s]", sqlJournal, e.Company, entityLike, from+"-01"),
			fmt.Sprintf("queryEntityDataReady(bank): %s [args: %s, %s, %s]", sqlBank, e.Company, entityLike, from+"-01"),
		}, contractSQLs...),
		Logs: append([]string{
			fmt.Sprintf("[数据完备性] journal=%d, bank=%d, contract_fund=%d, contract_cost=%d, total=%d", jCnt, bCnt, contractFundRows, contractCostRows, total),
		}, contractLogs...),
	}, nil
}

func (e *Engine) collectContractDataReadiness(entity, from, to string) (int, int, int, []string, []string, error) {
	entity = strings.TrimSpace(entity)
	if entity == "" || len(e.tableColumns("fin_contracts")) == 0 {
		return 0, 0, 0, nil, nil, nil
	}

	matchedEntity := e.resolveContractSubject("", entity)
	if strings.TrimSpace(matchedEntity) == "" {
		matchedEntity = entity
	}
	like := "%" + matchedEntity + "%"
	sqls := []string{
		"queryEntityDataReady(contracts): SELECT COUNT(DISTINCT contract_id) FROM fin_contracts WHERE customer_name LIKE ? OR contract_content LIKE ?",
	}
	logs := []string{fmt.Sprintf("[数据完备性-合同] entity=%s", matchedEntity)}

	var contractRows int
	if err := e.db.QueryRow(`
SELECT COUNT(DISTINCT contract_id)
FROM fin_contracts
WHERE customer_name LIKE ? OR contract_content LIKE ?
`, like, like).Scan(&contractRows); err != nil {
		return 0, 0, 0, nil, nil, err
	}
	if contractRows == 0 {
		logs = append(logs, "[数据完备性-合同] no matched contracts")
		return 0, 0, 0, sqls, logs, nil
	}

	var fundRows, costRows int
	if len(e.tableColumns("fin_fund_income")) > 0 {
		sqls = append(sqls, "queryEntityDataReady(contract_fund_income): SELECT COUNT(*) FROM fin_fund_income JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?")
		if err := e.db.QueryRow(`
SELECT COUNT(*)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND f.year_month BETWEEN ? AND ?
`, like, like, from, to).Scan(&fundRows); err != nil {
			return 0, 0, 0, nil, nil, err
		}
	}
	if e.hasFundIncomeGroupTables() {
		sqls = append(sqls, "queryEntityDataReady(contract_fund_income_groups): SELECT COUNT(*) FROM fin_fund_income_groups WHERE customer/member matches ... AND year_month BETWEEN ? AND ?")
		var groupRows int
		if err := e.db.QueryRow(`
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
  )
`, from, to, like, like, like).Scan(&groupRows); err != nil {
			return 0, 0, 0, nil, nil, err
		}
		fundRows += groupRows
	}
	if len(e.tableColumns("fin_cost_settlements")) > 0 {
		sqls = append(sqls, "queryEntityDataReady(contract_cost_settlements): SELECT COUNT(*) FROM fin_cost_settlements JOIN fin_contracts ... WHERE year_month BETWEEN ? AND ?")
		if err := e.db.QueryRow(`
SELECT COUNT(*)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND cs.year_month BETWEEN ? AND ?
`, like, like, from, to).Scan(&costRows); err != nil {
			return 0, 0, 0, nil, nil, err
		}
	}
	if e.hasCostSettlementGroupTables() {
		sqls = append(sqls, "queryEntityDataReady(contract_cost_settlement_groups): SELECT COUNT(*) FROM fin_cost_settlement_groups WHERE customer/member matches ... AND year_month BETWEEN ? AND ?")
		var groupRows int
		if err := e.db.QueryRow(`
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
  )
`, from, to, like, like, like).Scan(&groupRows); err != nil {
			return 0, 0, 0, nil, nil, err
		}
		costRows += groupRows
	}
	logs = append(logs, fmt.Sprintf("[数据完备性-合同] contracts=%d fund=%d cost=%d", contractRows, fundRows, costRows))
	return contractRows, fundRows, costRows, sqls, logs, nil
}

func buildReadinessFactSet(spec QuerySpec, summary readinessSummary) FactSet {
	tracePayload := map[string]any{
		"entity":             summary.Entity,
		"period":             summary.Period,
		"has_data":           summary.HasData,
		"rows":               summary.Rows,
		"journal_rows":       summary.JournalRows,
		"bank_rows":          summary.BankRows,
		"contract_rows":      summary.ContractRows,
		"contract_fund_rows": summary.ContractFundRows,
		"contract_cost_rows": summary.ContractCostRows,
		"executed_sql":       append([]string{}, summary.ExecutedSQL...),
		"logs":               append([]string{}, summary.Logs...),
	}
	asFloat := func(v int) float64 { return float64(v) }
	hasDataValue := 0.0
	if summary.HasData {
		hasDataValue = 1
	}

	return FactSet{
		Source: "data_readiness",
		Facts: []Fact{
			{
				Source:         "data_readiness",
				MetricKey:      "readiness_has_data",
				Entity:         summary.Entity,
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				Value:          hasDataValue,
				AuthorityLevel: AuthoritySupporting,
				CoverageStatus: CoverageFull,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
			{
				Source:         "data_readiness",
				MetricKey:      "readiness_row_count",
				Entity:         summary.Entity,
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				Value:          asFloat(summary.Rows),
				AuthorityLevel: AuthoritySupporting,
				CoverageStatus: CoverageFull,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
			{
				Source:         "data_readiness",
				MetricKey:      "readiness_journal_rows",
				Entity:         summary.Entity,
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				Value:          asFloat(summary.JournalRows),
				AuthorityLevel: AuthoritySupporting,
				CoverageStatus: CoverageFull,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
			{
				Source:         "data_readiness",
				MetricKey:      "readiness_bank_rows",
				Entity:         summary.Entity,
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				Value:          asFloat(summary.BankRows),
				AuthorityLevel: AuthoritySupporting,
				CoverageStatus: CoverageFull,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
			{
				Source:         "data_readiness",
				MetricKey:      "readiness_contract_fund_rows",
				Entity:         summary.Entity,
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				Value:          asFloat(summary.ContractFundRows),
				AuthorityLevel: AuthoritySupporting,
				CoverageStatus: CoverageFull,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
			{
				Source:         "data_readiness",
				MetricKey:      "readiness_contract_cost_rows",
				Entity:         summary.Entity,
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				Value:          asFloat(summary.ContractCostRows),
				AuthorityLevel: AuthoritySupporting,
				CoverageStatus: CoverageFull,
				Confidence:     1,
				TracePayload:   tracePayload,
			},
		},
	}
}
