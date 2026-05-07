package query

import (
	"context"
	"fmt"
)

func (e *Engine) shouldUseOrchestrator(spec QuerySpec) bool {
	return shouldUseOrchestratorForSpec(spec)
}

func shouldUseOrchestratorForSpec(spec QuerySpec) bool {
	switch spec.QueryFamily {
	case QueryFamilyContractDimension, QueryFamilyContractDetail, QueryFamilySupplierPayments, QueryFamilyReadiness, QueryFamilyCoreMetric:
		return true
	case QueryFamilyARAP:
		return shouldUseOrchestratorARAP(spec)
	default:
		return false
	}
}

func (e *Engine) tryOrchestratedQuery(spec QuerySpec) (Result, bool) {
	if !e.shouldUseOrchestrator(spec) {
		return Result{}, false
	}
	frame, err := NewOrchestrator(NewDefaultSourceRegistry(e)).Execute(context.Background(), spec)
	if err != nil {
		return Result{}, false
	}
	result, err := composeResultFromAnswerFrame(frame)
	if err != nil {
		return Result{}, false
	}
	return result, true
}

func composeResultFromAnswerFrame(frame AnswerFrame) (Result, error) {
	var result Result
	var err error

	switch frame.Spec.QueryFamily {
	case QueryFamilyContractDimension:
		result, err = composeContractResult(frame)
	case QueryFamilyContractDetail:
		result, err = composeContractDetailResult(frame)
	case QueryFamilySupplierPayments:
		result, err = composeSupplierPaymentResult(frame)
	case QueryFamilyReadiness:
		result, err = composeReadinessResult(frame)
	case QueryFamilyCoreMetric:
		result, err = composeCoreMetricResult(frame)
	case QueryFamilyARAP:
		result, err = composeARAPResult(frame)
	default:
		return Result{}, fmt.Errorf("unsupported orchestrated query family %s", frame.Spec.QueryFamily)
	}

	if err != nil {
		return result, err
	}

	// 统一添加 bridge 兼容字段（和 finalizeQueryResult 保持一致）
	if result.Data != nil {
		result.Data["final_answer"] = buildFinalAnswer(result)
		if hostSummary := buildHostSummaryContract(result.Data, frame.Spec.NormalizedQuestion); hostSummary != nil {
			result.Data["host_summary_contract"] = hostSummary
		}
	}

	return result, nil
}

func shouldUseOrchestratorARAP(spec QuerySpec) bool {
	if containsAny(spec.NormalizedQuestion, []string{"项目"}) {
		return false
	}
	hasReceivable := containsAny(spec.NormalizedQuestion, []string{"应收"})
	hasPayable := containsAny(spec.NormalizedQuestion, []string{"应付"})
	return hasReceivable || hasPayable
}
