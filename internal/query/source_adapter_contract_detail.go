package query

import "context"

type ContractDetailSourceAdapter struct {
	runtime ContractDetailSourceRuntime
}

func NewContractDetailSourceAdapter(runtime ContractDetailSourceRuntime) *ContractDetailSourceAdapter {
	return &ContractDetailSourceAdapter{runtime: runtime}
}

func (a *ContractDetailSourceAdapter) Name() string {
	return "contract_detail"
}

func (a *ContractDetailSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{SourceCapabilityContractDetail}
}

func (a *ContractDetailSourceAdapter) Fetch(ctx context.Context, spec QuerySpec) (FactSet, error) {
	probe := a.runtime.ProbeContractDetailSources(spec)
	detail, err := a.runtime.collectContractDetail(ctx, spec, probe)
	if err != nil {
		return FactSet{}, err
	}
	return buildContractDetailFactSet(spec, detail), nil
}
