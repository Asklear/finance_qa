package query

import "fmt"

func (e *Engine) queryEntityDataReady(entity, from, to string) Result {
	summary, err := e.collectEntityDataReadiness(entity, from, to)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	spec := QuerySpec{
		QueryFamily: QueryFamilyReadiness,
		Entity:      entity,
		PeriodFrom:  from,
		PeriodTo:    to,
	}
	factSet := buildReadinessFactSet(spec, summary)
	if summary.HasData {
		return Result{
			Success: true,
			Message: fmt.Sprintf("%s 在 %s 有 %d 条数据", entity, to, summary.Rows),
			Data: map[string]any{
				"entity":    entity,
				"period":    to,
				"has_data":  true,
				"rows":      summary.Rows,
				"fact_sets": []FactSet{factSet},
			},
			ExecutedSQL:     summary.ExecutedSQL,
			CalculationLogs: summary.Logs,
		}
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 在 %s 暂无数据", entity, to),
		Data: map[string]any{
			"entity":    entity,
			"period":    to,
			"has_data":  false,
			"rows":      0,
			"fact_sets": []FactSet{factSet},
		},
		ExecutedSQL:     summary.ExecutedSQL,
		CalculationLogs: summary.Logs,
	}
}
