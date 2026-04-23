package query

import "context"

type ContractSourceAdapter struct {
	engine *Engine
}

func NewContractSourceAdapter(engine *Engine) *ContractSourceAdapter {
	return &ContractSourceAdapter{engine: engine}
}

func (a *ContractSourceAdapter) Name() string {
	return "contracts"
}

func (a *ContractSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{SourceCapabilityContractLedger}
}

func (a *ContractSourceAdapter) Fetch(_ context.Context, spec QuerySpec) (FactSet, error) {
	if spec.QueryFamily == QueryFamilyCoreMetric && spec.PreferContractAggregate {
		summary, err := a.engine.collectContractAggregateSummary(spec)
		if err != nil {
			return buildContractAggregateMissingFactSet(spec, err.Error()), nil
		}
		return buildContractAggregateFactSet(spec, summary), nil
	}
	summary, err := a.engine.collectContractDimensionSummary(spec.OriginalQuestion, spec.Entity, a.engine.getLatestPeriodAnchor())
	if err != nil {
		return FactSet{}, err
	}
	return buildContractDimensionFactSet(spec, summary), nil
}
