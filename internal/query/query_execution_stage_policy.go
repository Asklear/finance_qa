package query

type executionStagePlanBuilder struct {
	stages []executionStage
}

func newExecutionStagePlanBuilder(capacity int) *executionStagePlanBuilder {
	if capacity < 1 {
		capacity = 1
	}
	return &executionStagePlanBuilder{stages: make([]executionStage, 0, capacity)}
}

func (b *executionStagePlanBuilder) add(stage executionStage) {
	for _, existing := range b.stages {
		if existing == stage {
			return
		}
	}
	b.stages = append(b.stages, stage)
}

func (b *executionStagePlanBuilder) addAll(stages ...executionStage) {
	for _, stage := range stages {
		b.add(stage)
	}
}

func (b *executionStagePlanBuilder) stagesOrEmpty() []executionStage {
	return append([]executionStage{}, b.stages...)
}

func resolveOperationalExecutionStages(ctx queryExecutionContext) []executionStage {
	stages := make([]executionStage, 0, 2)
	if ctx.spec.QueryFamily == QueryFamilyHRCost || shouldUseHRBreakdown(ctx.q, ctx.cfg) {
		stages = append(stages, executionStageHRBreakdown)
	}
	return stages
}

func resolveSourceExecutionStages(ctx queryExecutionContext) []executionStage {
	builder := newExecutionStagePlanBuilder(4)
	if shouldUseOrchestratorForSpec(ctx.spec) {
		builder.add(executionStageOrchestrator)
	}
	builder.addAll(resolveLegacySourceFallbackStages(ctx)...)
	return builder.stagesOrEmpty()
}

func resolveLegacySourceFallbackStages(ctx queryExecutionContext) []executionStage {
	builder := newExecutionStagePlanBuilder(4)
	if ctx.spec.NeedsContractDimension || ctx.spec.QueryFamily == QueryFamilyContractDimension || shouldUseContractDimension(ctx.q) {
		builder.add(executionStageDirectContractDimension)
	}
	if ctx.spec.QueryFamily == QueryFamilyReconciliation || shouldUseReconciliation(ctx.q) {
		builder.add(executionStageDirectReconciliation)
	}
	if isIntervalCoreMetricQuestion(ctx.q, ctx.entity, ctx.hasRealEntity, ctx.from, ctx.to) ||
		shouldPreferCoreMetricSummary(ctx.q, ctx.entity, ctx.hasRealEntity, ctx.from, ctx.to) {
		builder.add(executionStageDirectCoreMetricRange)
	}
	if ctx.spec.QueryFamily == QueryFamilySupplierPayments || shouldUseSupplierPaymentStats(ctx.q) {
		builder.add(executionStageDirectSupplierPayments)
	}
	return builder.stagesOrEmpty()
}

func resolveCounterpartyExecutionStages(ctx queryExecutionContext) []executionStage {
	if ctx.spec.NeedsContractDimension || ctx.spec.QueryFamily == QueryFamilyContractDimension {
		return nil
	}
	builder := newExecutionStagePlanBuilder(2)
	if ctx.hasRealEntity && isCounterpartyClassificationQuestion(ctx.q) {
		builder.add(executionStageCounterpartyClassification)
	}
	if ctx.engine != nil && ctx.engine.shouldUseCounterpartyAuditFallback(ctx) {
		builder.add(executionStageCounterpartyAuditFallback)
	}
	return builder.stagesOrEmpty()
}
