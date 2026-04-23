package query

import (
	"context"
	"fmt"
	"math"
	"strings"
)

func (e *Engine) queryARAP(question, entity, from, to string) Result {
	period := to
	var result Result
	if entity != "" && strings.Contains(question, "项目") {
		result = e.queryProjectARAP(entity, from, to)
	} else if entity != "" && e.isRealBusinessEntity(question, entity) && strings.Contains(question, "应付") && !strings.Contains(question, "应收/应付") && !strings.Contains(question, "应收") {
		result = e.queryAccountPayableReceivable(period, "应付账款", "2202", "payable", entity)
	} else if entity != "" && e.isRealBusinessEntity(question, entity) && strings.Contains(question, "应收") && !strings.Contains(question, "应收/应付") && !strings.Contains(question, "应付") {
		result = e.queryAccountPayableReceivable(period, "应收账款", "1122", "receivable", entity)
	} else if entity != "" && e.isRealBusinessEntity(question, entity) && containsAny(question, []string{"应收", "应付"}) {
		result = e.queryEntityARAP(entity, period)
	} else if strings.Contains(question, "应收") {
		result = e.queryAccountPayableReceivable(period, "应收账款", "1122", "receivable", "")
	} else if strings.Contains(question, "应付") {
		result = e.queryAccountPayableReceivable(period, "应付账款", "2202", "payable", "")
	} else if entity != "" {
		result = e.queryCounterpartyAmountFallback(question, entity, from, to)
	} else {
		result = Result{
			Success: false,
			Message: "未识别应收/应付对象",
			CalculationLogs: []string{
				"[AR/AP分流] 问题未命中应收/应付且未识别实体",
			},
		}
	}

	if result.Success {
		spec := BuildQuerySpec(question, e.getLatestPeriodAnchor())
		if strings.TrimSpace(entity) != "" && !looksLikeAccountFragment(entity) && e.isRealBusinessEntity(question, entity) {
			spec.Entity = entity
		} else {
			spec.Entity = ""
		}
		spec.PeriodFrom = from
		spec.PeriodTo = to
		if factSet, ok := buildARAPFactSetFromQueryResult(spec, result); ok && len(factSet.Facts) > 0 {
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["fact_sets"] = []FactSet{factSet}
		} else if factSet, err := NewARAPSourceAdapter(e).Fetch(context.Background(), spec); err == nil && len(factSet.Facts) > 0 {
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["fact_sets"] = []FactSet{factSet}
		}
	}
	return result
}

func (e *Engine) queryEntityARAP(entity, period string) Result {
	receivable := e.queryAccountPayableReceivable(period, "应收账款", "1122", "receivable", entity)
	payable := e.queryAccountPayableReceivable(period, "应付账款", "2202", "payable", entity)
	if !receivable.Success && !payable.Success {
		return Result{Success: false, Message: fmt.Sprintf("[%s] 未找到应收/应付余额", entity)}
	}

	receivableTotal := 0.0
	receivableDetails := []map[string]any{}
	if receivable.Success {
		receivableTotal = anyToFloat64(receivable.Data["total"])
		receivableDetails = mapsFromAnySlice(receivable.Data["details"])
	}
	payableTotal := 0.0
	payableDetails := []map[string]any{}
	if payable.Success {
		payableTotal = anyToFloat64(payable.Data["total"])
		payableDetails = mapsFromAnySlice(payable.Data["details"])
	}
	inferencePrefix := ""
	if resultUsesInferredOpenItemSettlement(receivable) || resultUsesInferredOpenItemSettlement(payable) {
		inferencePrefix = "按开放项推断："
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("[%s] %s %s应收 %.2f 元，应付 %.2f 元", entity, period, inferencePrefix, receivableTotal, payableTotal),
		Data: map[string]any{
			"entity":           entity,
			"period":           period,
			"receivable_total": round2(receivableTotal),
			"payable_total":    round2(payableTotal),
			"receivable":       receivable.Data,
			"payable":          payable.Data,
			"details": map[string]any{
				"receivable": receivableDetails,
				"payable":    payableDetails,
			},
		},
		ExecutedSQL:     append(append([]string{}, receivable.ExecutedSQL...), payable.ExecutedSQL...),
		CalculationLogs: append(append([]string{}, receivable.CalculationLogs...), payable.CalculationLogs...),
	}
}

func (e *Engine) queryProjectARAP(entity, from, to string) Result {
	sqlTxt := `SELECT COALESCE(SUM(credit_amount), 0), COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND counterparty_name LIKE ? AND transaction_date BETWEEN ? AND ?`
	var inAmt, outAmt float64
	e.db.QueryRow(sqlTxt, e.Company, e.Company, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&inAmt, &outAmt)
	receivable := math.Max(inAmt-outAmt, 0)
	payable := math.Max(outAmt-inAmt, 0)
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 项目应收 %.2f 元，应付 %.2f 元", to, entity, receivable, payable),
		Data: map[string]any{
			"entity": entity, "period": to, "receivable": receivable, "payable": payable,
		},
		ExecutedSQL: []string{
			fmt.Sprintf("queryProjectARAP: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[项目应收应付] 收入=%.2f, 成本=%.2f, 应收=%.2f, 应付=%.2f", inAmt, outAmt, receivable, payable),
		},
	}
}
