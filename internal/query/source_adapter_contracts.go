package query

import "context"

type ContractSourceAdapter struct {
	runtime ContractSourceRuntime
}

func NewContractSourceAdapter(runtime ContractSourceRuntime) *ContractSourceAdapter {
	return &ContractSourceAdapter{runtime: runtime}
}

func (a *ContractSourceAdapter) Name() string {
	return "contracts"
}

func (a *ContractSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{SourceCapabilityContractLedger}
}

func (a *ContractSourceAdapter) Fetch(_ context.Context, spec QuerySpec) (FactSet, error) {
	if spec.QueryFamily == QueryFamilyCoreMetric && spec.PreferContractAggregate {
		summary, err := a.runtime.collectContractAggregateSummary(spec)
		if err != nil {
			return buildContractAggregateMissingFactSetWithConfig(spec, err.Error(), a.runtime.currentRuleConfig()), nil
		}
		return buildContractAggregateFactSet(spec, summary), nil
	}
	summary, err := a.runtime.collectContractDimensionSummaryForPeriod(spec.OriginalQuestion, spec.Entity, spec.PeriodFrom, spec.PeriodTo)
	if err != nil && (spec.PeriodFrom == "" || spec.PeriodTo == "") {
		summary, err = a.runtime.collectContractDimensionSummary(spec.OriginalQuestion, spec.Entity, a.runtime.getLatestContractPeriodAnchor())
	}
	if err != nil {
		return FactSet{}, err
	}
	return buildContractDimensionFactSet(spec, summary), nil
}
