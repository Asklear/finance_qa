package query

import "fmt"

func composeContractResult(frame AnswerFrame) (Result, error) {
	factSet, ok := findFactSetBySource(frame.FactSets, "contracts")
	if !ok || len(factSet.Facts) == 0 {
		return Result{}, fmt.Errorf("contracts fact set missing")
	}
	trace := factSet.Facts[0].TracePayload
	data, _ := trace["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	summary := contractDimensionSummary{
		Entity:    anyToString(trace["entity"]),
		Role:      anyToString(trace["role"]),
		Period:    anyToString(trace["period"]),
		Contracts: anyToMapSlice(trace["contracts"]),
		Data:      cloneMap(data),
	}
	if subPeriod := anyToString(summary.Data["sub_period"]); subPeriod != "" {
		summary.SubPeriod = subPeriod
	}
	summary.Data["fact_sets"] = frame.FactSets
	summary.Data["query_pipeline"] = "orchestrator"
	summary.Data["source_plan"] = sourceCapabilitiesToStrings(frame.Plan.Capabilities)
	return Result{
		Success:         true,
		Message:         buildContractDimensionMessage(summary),
		Data:            summary.Data,
		ExecutedSQL:     extractFrameTraceStrings(frame.FactSets, "executed_sql"),
		CalculationLogs: extractFrameTraceStrings(frame.FactSets, "logs"),
	}, nil
}

func composeContractAggregateCoreMetricResult(frame AnswerFrame) (Result, bool, error) {
	factSet, ok := findFactSetBySource(frame.FactSets, "contract_aggregate")
	if !ok || len(factSet.Facts) == 0 {
		return Result{}, false, nil
	}
	trace := factSet.Facts[0].TracePayload
	if canAnswer, _ := trace["can_answer"].(bool); canAnswer {
		data, _ := trace["result_data"].(map[string]any)
		message, _ := trace["result_message"].(string)
		if data == nil || message == "" {
			return Result{}, true, fmt.Errorf("contract aggregate result snapshot missing")
		}
		resultData := cloneMap(data)
		resultData["fact_sets"] = frame.FactSets
		resultData["query_pipeline"] = "orchestrator"
		resultData["source_plan"] = sourceCapabilitiesToStrings(frame.Plan.Capabilities)
		return Result{
			Success:         true,
			Message:         message,
			Data:            resultData,
			ExecutedSQL:     extractFrameTraceStrings(frame.FactSets, "executed_sql"),
			CalculationLogs: extractFrameTraceStrings(frame.FactSets, "calculation_logs"),
		}, true, nil
	}
	if shouldUseStrictContractSourceForSpec(frame.Spec) {
		return buildStrictContractMissingResultForSpec(
			frame.Spec,
			anyToString(trace["fallback_reason"]),
			anySourceStringSlice(trace["source_tables"]),
			extractFrameTraceStrings(frame.FactSets, "executed_sql"),
			extractFrameTraceStrings(frame.FactSets, "calculation_logs"),
		), true, nil
	}
	return Result{}, false, nil
}
