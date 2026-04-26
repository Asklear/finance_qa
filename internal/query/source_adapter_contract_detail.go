package query

import "context"

type ContractDetailSourceAdapter struct {
	engine *Engine
}

func NewContractDetailSourceAdapter(engine *Engine) *ContractDetailSourceAdapter {
	return &ContractDetailSourceAdapter{engine: engine}
}

func (a *ContractDetailSourceAdapter) Name() string {
	return "contract_detail"
}

func (a *ContractDetailSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{SourceCapabilityContractDetail}
}

func (a *ContractDetailSourceAdapter) Fetch(ctx context.Context, spec QuerySpec) (FactSet, error) {
	probe := a.engine.ProbeContractDetailSources(spec)
	detail, err := a.engine.collectContractDetail(ctx, spec, probe)
	if err != nil {
		return FactSet{}, err
	}
	return buildContractDetailFactSet(spec, detail), nil
}
