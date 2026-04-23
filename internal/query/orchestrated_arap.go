package query

import "fmt"

func composeARAPResult(frame AnswerFrame) (Result, error) {
	factSet, ok := findFactSetBySource(frame.FactSets, "arap")
	if !ok || len(factSet.Facts) == 0 {
		return Result{}, fmt.Errorf("arap fact set missing")
	}
	scopes := detectARAPScopes(frame.Spec)
	if frame.Spec.Entity != "" && len(scopes) > 1 {
		return composeEntityARAPResult(frame, factSet)
	}
	if frame.Spec.Entity != "" && len(scopes) == 1 {
		return composeEntitySingleScopeARAPResult(frame, factSet, scopes[0])
	}

	scopePrefix := "official_arap"
	accountName := "应收账款"
	if containsAny(frame.Spec.NormalizedQuestion, []string{"应付"}) {
		accountName = "应付账款"
	}

	resultData := extractARAPResultData(factSet, scopePrefix+"_total")
	if resultData == nil {
		resultData = extractARAPResultData(factSet, "openitem_closing_total")
	}
	if resultData == nil {
		return Result{}, fmt.Errorf("arap result data missing")
	}

	data := cloneMap(resultData)
	if _, ok := data["account"]; !ok {
		data["account"] = accountName
	}
	total, _ := findFactValue(factSet, scopePrefix+"_total")
	if total == 0 {
		total, _ = findFactValue(factSet, "openitem_closing_total")
	}
	period := anyToString(data["period"])
	if period == "" {
		period = frame.Spec.PeriodTo
		data["period"] = period
	}
	message := fmt.Sprintf("%s %s期末余额 %.2f 元", period, anyToString(data["account"]), total)
	if anyToString(data["source"]) == "journal_open_items" {
		message = buildARAPOpenItemMessage(data)
	}
	data["fact_sets"] = frame.FactSets
	data["query_pipeline"] = "orchestrator"
	data["source_plan"] = sourceCapabilitiesToStrings(frame.Plan.Capabilities)

	return Result{
		Success:         true,
		Message:         message,
		Data:            data,
		ExecutedSQL:     extractFrameTraceStrings(frame.FactSets, "executed_sql"),
		CalculationLogs: extractFrameTraceStrings(frame.FactSets, "logs"),
	}, nil
}

func composeEntitySingleScopeARAPResult(frame AnswerFrame, factSet FactSet, scope arapScope) (Result, error) {
	resultData := extractARAPResultData(factSet, "official_arap_total")
	if resultData == nil {
		resultData = extractARAPResultData(factSet, "openitem_closing_total")
	}
	if resultData == nil {
		return Result{}, fmt.Errorf("entity arap result data missing")
	}

	data := cloneMap(resultData)
	data["entity"] = frame.Spec.Entity
	if anyToString(data["account"]) == "" {
		data["account"] = scope.accountName
	}
	if anyToString(data["period"]) == "" {
		data["period"] = frame.Spec.PeriodTo
	}
	message := buildEntitySingleScopeARAPMessage(data)
	if message == "" {
		message = fmt.Sprintf("%s %s %s期末余额 %.2f 元", anyToString(data["period"]), frame.Spec.Entity, anyToString(data["account"]), anyToFloat64(data["total"]))
	}
	data["fact_sets"] = frame.FactSets
	data["query_pipeline"] = "orchestrator"
	data["source_plan"] = sourceCapabilitiesToStrings(frame.Plan.Capabilities)

	return Result{
		Success:         true,
		Message:         message,
		Data:            data,
		ExecutedSQL:     extractFrameTraceStrings(frame.FactSets, "executed_sql"),
		CalculationLogs: extractFrameTraceStrings(frame.FactSets, "logs"),
	}, nil
}

func composeEntityARAPResult(frame AnswerFrame, factSet FactSet) (Result, error) {
	receivable := extractARAPResultData(factSet, "official_receivable_total")
	if receivable == nil {
		receivable = extractARAPResultData(factSet, "openitem_receivable_closing_total")
	}
	payable := extractARAPResultData(factSet, "official_payable_total")
	if payable == nil {
		payable = extractARAPResultData(factSet, "openitem_payable_closing_total")
	}
	if receivable == nil {
		receivable = map[string]any{"total": float64(0), "details": []map[string]any{}}
	}
	if payable == nil {
		payable = map[string]any{"total": float64(0), "details": []map[string]any{}}
	}
	receivableTotal := anyToFloat64(receivable["total"])
	payableTotal := anyToFloat64(payable["total"])
	inferencePrefix := ""
	if resultDataUsesInferredOpenItemSettlement(receivable) || resultDataUsesInferredOpenItemSettlement(payable) {
		inferencePrefix = "按开放项推断："
	}
	data := map[string]any{
		"entity":           frame.Spec.Entity,
		"period":           frame.Spec.PeriodTo,
		"receivable_total": round2(receivableTotal),
		"payable_total":    round2(payableTotal),
		"receivable":       cloneMap(receivable),
		"payable":          cloneMap(payable),
		"details": map[string]any{
			"receivable": mapsFromAnySlice(receivable["details"]),
			"payable":    mapsFromAnySlice(payable["details"]),
		},
		"fact_sets":      frame.FactSets,
		"query_pipeline": "orchestrator",
		"source_plan":    sourceCapabilitiesToStrings(frame.Plan.Capabilities),
	}
	return Result{
		Success:         true,
		Message:         fmt.Sprintf("[%s] %s %s应收 %.2f 元，应付 %.2f 元", frame.Spec.Entity, frame.Spec.PeriodTo, inferencePrefix, receivableTotal, payableTotal),
		Data:            data,
		ExecutedSQL:     extractFrameTraceStrings(frame.FactSets, "executed_sql"),
		CalculationLogs: extractFrameTraceStrings(frame.FactSets, "logs"),
	}, nil
}

func buildEntitySingleScopeARAPMessage(data map[string]any) string {
	period := anyToString(data["period"])
	entity := anyToString(data["entity"])
	account := anyToString(data["account"])
	source := anyToString(data["source"])
	if source == "journal_open_items" {
		return buildARAPOpenItemMessage(data)
	}
	if source != "journal_entity_rollforward" {
		return ""
	}
	actionLabel := "回款/冲减"
	if anyToString(data["type"]) == "payable" {
		actionLabel = "付款/冲减"
	}
	return fmt.Sprintf("%s %s %s期末余额 %.2f 元（期初 %.2f，本期新增 %.2f，本期%s %.2f，开放项残余 %.2f）。",
		period,
		entity,
		account,
		anyToFloat64(data["total"]),
		anyToFloat64(data["opening_balance"]),
		anyToFloat64(data["current_increase"]),
		actionLabel,
		anyToFloat64(data["current_decrease"]),
		anyToFloat64(data["open_item_closing_total"]))
}

func extractARAPResultData(factSet FactSet, metricKey string) map[string]any {
	for _, fact := range factSet.Facts {
		if fact.MetricKey != metricKey {
			continue
		}
		if data, ok := fact.TracePayload["result_data"].(map[string]any); ok {
			return cloneMap(data)
		}
	}
	return nil
}

func resultDataUsesInferredOpenItemSettlement(data map[string]any) bool {
	if openItem, ok := data["open_item_analysis"].(map[string]any); ok && resultDataUsesInferredOpenItemSettlement(openItem) {
		return true
	}
	if anyToString(data["source"]) != "journal_open_items" {
		return false
	}
	raw, ok := data["settlement_confidence"].(map[string]any)
	if !ok {
		return false
	}
	return anyToFloat64(raw["probable_historical_settlement"])+
		anyToFloat64(raw["probable_current_settlement"])+
		anyToFloat64(raw["unmatched_decrease"]) > 0
}
