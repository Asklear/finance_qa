package query

import (
	"fmt"
	"strings"
)

func composeCoreMetricResult(frame AnswerFrame) (Result, error) {
	if contractResult, ok, err := composeContractAggregateCoreMetricResult(frame); ok || err != nil {
		return contractResult, err
	}
	factSet, ok := findFactSetBySource(frame.FactSets, "core_metrics")
	if !ok || len(factSet.Facts) == 0 {
		return Result{}, fmt.Errorf("core_metrics fact set missing")
	}
	trace := factSet.Facts[0].TracePayload
	data, _ := trace["result_data"].(map[string]any)
	message, _ := trace["result_message"].(string)
	if data == nil || message == "" {
		return Result{}, fmt.Errorf("core_metrics result snapshot missing")
	}
	resultData := cloneMap(data)
	if contractFactSet, ok := findFactSetBySource(frame.FactSets, "contract_aggregate"); ok && len(contractFactSet.Facts) > 0 {
		if fallbackReason := anyToString(contractFactSet.Facts[0].TracePayload["fallback_reason"]); fallbackReason != "" {
			message = fallbackReason + "。\n" + message
			resultData["contract_fallback_reason"] = fallbackReason
		}
	}
	resultData["fact_sets"] = frame.FactSets
	resultData["query_pipeline"] = "orchestrator"
	resultData["source_plan"] = sourceCapabilitiesToStrings(frame.Plan.Capabilities)
	result := Result{
		Success:         true,
		Message:         message,
		Data:            resultData,
		ExecutedSQL:     extractFrameTraceStrings(frame.FactSets, "executed_sql"),
		CalculationLogs: extractFrameTraceStrings(frame.FactSets, "calculation_logs"),
	}
	return annotateJournalTaxDisclosure(result, strings.Contains(anyToString(trace["accrual_source"]), "journal")), nil
}
