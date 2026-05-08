package query

import "time"

func (e *Engine) queryContractDimension(question, entity string, anchor time.Time) Result {
	summary, err := e.collectContractDimensionSummary(question, entity, anchor)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}

	message := buildContractDimensionMessage(summary)
	factSet := buildContractDimensionFactSet(QuerySpec{
		QueryFamily: QueryFamilyContractDimension,
		MetricKind:  MetricKindUnknown,
		Entity:      summary.Entity,
		PeriodFrom:  summary.PeriodFrom,
		PeriodTo:    summary.PeriodTo,
		SubPeriod:   summary.SubPeriod,
	}, summary)
	summary.Data["fact_sets"] = []FactSet{factSet}

	return Result{
		Success:         true,
		Message:         message,
		Data:            summary.Data,
		ExecutedSQL:     summary.ExecutedSQL,
		CalculationLogs: summary.CalculationLog,
	}
}
