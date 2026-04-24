package query

import "testing"

func TestBuildExecutionPlanPrefersOrchestratorBeforeDirectContractFallback(t *testing.T) {
	ctx := queryExecutionContext{
		q:    "飞未云科2026年累计销售额多少？",
		spec: QuerySpec{QueryFamily: QueryFamilyContractDimension, NeedsContractDimension: true},
		cfg:  getRuleConfig(),
	}

	plan := buildExecutionPlan(ctx)
	if len(plan) < 2 {
		t.Fatalf("plan too short: %+v", plan)
	}
	if plan[0] != executionStageOrchestrator {
		t.Fatalf("plan[0] = %s, want %s", plan[0], executionStageOrchestrator)
	}
	if plan[1] != executionStageDirectContractDimension {
		t.Fatalf("plan[1] = %s, want %s", plan[1], executionStageDirectContractDimension)
	}
}

func TestBuildExecutionPlanKeepsReconciliationAheadOfIntentRoute(t *testing.T) {
	ctx := queryExecutionContext{
		q:    "为什么2026年3月银行卡上看和账上看的利润不一样？",
		spec: QuerySpec{QueryFamily: QueryFamilyReconciliation},
		cfg:  getRuleConfig(),
	}

	plan := buildExecutionPlan(ctx)
	if len(plan) < 2 {
		t.Fatalf("plan too short: %+v", plan)
	}
	if plan[0] != executionStageDirectReconciliation {
		t.Fatalf("plan[0] = %s, want %s", plan[0], executionStageDirectReconciliation)
	}
	if plan[len(plan)-1] != executionStageIntentRoute {
		t.Fatalf("last stage = %s, want %s", plan[len(plan)-1], executionStageIntentRoute)
	}
}

func TestBuildExecutionPlanPlacesClassificationBeforeAuditFallback(t *testing.T) {
	ctx := queryExecutionContext{
		q:             "林悦在2026年2月这笔是成本还是收入？",
		spec:          QuerySpec{QueryFamily: QueryFamilyCounterparty},
		hasRealEntity: true,
		entity:        "南京林悦智能科技有限公司",
		cfg:           getRuleConfig(),
	}

	plan := buildExecutionPlan(ctx)
	classificationIndex := -1
	for idx, stage := range plan {
		if stage == executionStageCounterpartyClassification {
			classificationIndex = idx
		}
	}
	if classificationIndex == -1 {
		t.Fatalf("classification stage missing: %+v", plan)
	}
	for _, stage := range plan {
		if stage == executionStageCounterpartyAuditFallback {
			t.Fatalf("classification-only question should not inject audit fallback: %+v", plan)
		}
	}
}
