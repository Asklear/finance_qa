package query

import (
	"context"
	"fmt"
)

func (e *Engine) shouldUseOrchestrator(spec QuerySpec) bool {
	switch spec.QueryFamily {
	case QueryFamilyContractDimension, QueryFamilySupplierPayments, QueryFamilyReadiness, QueryFamilyCoreMetric:
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
	switch frame.Spec.QueryFamily {
	case QueryFamilyContractDimension:
		return composeContractResult(frame)
	case QueryFamilySupplierPayments:
		return composeSupplierPaymentResult(frame)
	case QueryFamilyReadiness:
		return composeReadinessResult(frame)
	case QueryFamilyCoreMetric:
		return composeCoreMetricResult(frame)
	case QueryFamilyARAP:
		return composeARAPResult(frame)
	default:
		return Result{}, fmt.Errorf("unsupported orchestrated query family %s", frame.Spec.QueryFamily)
	}
}

func shouldUseOrchestratorARAP(spec QuerySpec) bool {
	if containsAny(spec.NormalizedQuestion, []string{"项目"}) {
		return false
	}
	hasReceivable := containsAny(spec.NormalizedQuestion, []string{"应收"})
	hasPayable := containsAny(spec.NormalizedQuestion, []string{"应付"})
	return hasReceivable || hasPayable
}
