package query

import "fmt"

func composeSupplierPaymentResult(frame AnswerFrame) (Result, error) {
	factSet, ok := findFactSetBySource(frame.FactSets, "supplier_payments")
	if !ok || len(factSet.Facts) == 0 {
		return Result{}, fmt.Errorf("supplier fact set missing")
	}
	trace := factSet.Facts[0].TracePayload
	suppliers := anyToMapSlice(trace["suppliers"])
	excluded := anyToMapSlice(trace["excluded"])
	period := anyToString(trace["period"])
	total, _ := findFactValue(factSet, "supplier_payment_total")
	count, _ := findFactValue(factSet, "supplier_payment_count")
	msg := fmt.Sprintf("%s 发生付款的外部供应商共 %d 家，合计 %.2f 元。", period, int(count), total)
	if len(suppliers) == 0 {
		msg = fmt.Sprintf("%s 暂未识别到外部供应商付款。", period)
	}
	return Result{
		Success: true,
		Message: msg,
		Data: map[string]any{
			"period":                  period,
			"count":                   int(count),
			"total":                   total,
			"suppliers":               suppliers,
			"excluded_counterparties": excluded,
			"fact_sets":               frame.FactSets,
			"query_pipeline":          "orchestrator",
			"source_plan":             sourceCapabilitiesToStrings(frame.Plan.Capabilities),
		},
		ExecutedSQL:     extractFrameTraceStrings(frame.FactSets, "executed_sql"),
		CalculationLogs: extractFrameTraceStrings(frame.FactSets, "logs"),
	}, nil
}

func composeReadinessResult(frame AnswerFrame) (Result, error) {
	factSet, ok := findFactSetBySource(frame.FactSets, "data_readiness")
	if !ok || len(factSet.Facts) == 0 {
		return Result{}, fmt.Errorf("readiness fact set missing")
	}
	trace := factSet.Facts[0].TracePayload
	entity := anyToString(trace["entity"])
	period := anyToString(trace["period"])
	hasData, _ := findFactValue(factSet, "readiness_has_data")
	rows, _ := findFactValue(factSet, "readiness_row_count")
	message := fmt.Sprintf("%s 在 %s 暂无数据", entity, period)
	if hasData > 0 {
		message = fmt.Sprintf("%s 在 %s 有 %d 条数据", entity, period, int(rows))
	}
	return Result{
		Success: true,
		Message: message,
		Data: map[string]any{
			"entity":         entity,
			"period":         period,
			"has_data":       hasData > 0,
			"rows":           int(rows),
			"fact_sets":      frame.FactSets,
			"query_pipeline": "orchestrator",
			"source_plan":    sourceCapabilitiesToStrings(frame.Plan.Capabilities),
		},
		ExecutedSQL:     extractFrameTraceStrings(frame.FactSets, "executed_sql"),
		CalculationLogs: extractFrameTraceStrings(frame.FactSets, "logs"),
	}, nil
}
