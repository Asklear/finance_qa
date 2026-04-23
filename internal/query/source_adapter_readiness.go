package query

import (
	"context"
	"fmt"
)

type ReadinessSourceAdapter struct {
	engine *Engine
}

type readinessSummary struct {
	Entity      string
	Period      string
	PeriodFrom  string
	PeriodTo    string
	HasData     bool
	Rows        int
	JournalRows int
	BankRows    int
	ExecutedSQL []string
	Logs        []string
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
	sqlJournal := `SELECT COUNT(*) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND summary LIKE ? AND voucher_date BETWEEN ? AND ?`
	sqlBank := `SELECT COUNT(*) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND counterparty_name LIKE ? AND transaction_date BETWEEN ? AND ?`
	var jCnt, bCnt int
	if err := e.db.QueryRow(sqlJournal, e.Company, e.Company, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&jCnt); err != nil {
		return readinessSummary{}, err
	}
	if err := e.db.QueryRow(sqlBank, e.Company, e.Company, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&bCnt); err != nil {
		return readinessSummary{}, err
	}
	total := jCnt + bCnt
	return readinessSummary{
		Entity:      entity,
		Period:      to,
		PeriodFrom:  from,
		PeriodTo:    to,
		HasData:     total > 0,
		Rows:        total,
		JournalRows: jCnt,
		BankRows:    bCnt,
		ExecutedSQL: []string{
			fmt.Sprintf("queryEntityDataReady(journal): %s [args: %s, %s, %s]", sqlJournal, e.Company, "%"+entity+"%", from+"-01"),
			fmt.Sprintf("queryEntityDataReady(bank): %s [args: %s, %s, %s]", sqlBank, e.Company, "%"+entity+"%", from+"-01"),
		},
		Logs: []string{
			fmt.Sprintf("[数据完备性] journal=%d, bank=%d, total=%d", jCnt, bCnt, total),
		},
	}, nil
}

func buildReadinessFactSet(spec QuerySpec, summary readinessSummary) FactSet {
	tracePayload := map[string]any{
		"entity":       summary.Entity,
		"period":       summary.Period,
		"has_data":     summary.HasData,
		"rows":         summary.Rows,
		"journal_rows": summary.JournalRows,
		"bank_rows":    summary.BankRows,
		"executed_sql": append([]string{}, summary.ExecutedSQL...),
		"logs":         append([]string{}, summary.Logs...),
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
		},
	}
}
