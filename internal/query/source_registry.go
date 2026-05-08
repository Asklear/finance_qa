package query

import "context"

type SourceAdapter interface {
	Name() string
	Capabilities() []SourceCapability
	Fetch(ctx context.Context, spec QuerySpec) (FactSet, error)
}

type SourceRegistry struct {
	adapters []SourceAdapter
}

func NewSourceRegistry() *SourceRegistry {
	return &SourceRegistry{adapters: make([]SourceAdapter, 0, 8)}
}

func NewDefaultSourceRegistry(runtime DefaultSourceRuntime) *SourceRegistry {
	registry := NewSourceRegistry()
	if runtime == nil {
		return registry
	}
	registry.Register(NewContractDetailSourceAdapter(runtime))
	registry.Register(NewContractSourceAdapter(runtime))
	registry.Register(NewCoreMetricsSourceAdapter(runtime))
	registry.Register(NewARAPSourceAdapter(runtime))
	registry.Register(NewReadinessSourceAdapter(runtime))
	registry.Register(NewSupplierPaymentSourceAdapter(runtime))
	return registry
}

func (r *SourceRegistry) Register(adapter SourceAdapter) {
	if adapter == nil {
		return
	}
	r.adapters = append(r.adapters, adapter)
}

func (r *SourceRegistry) Resolve(capability SourceCapability) (SourceAdapter, bool) {
	for _, adapter := range r.adapters {
		for _, supported := range adapter.Capabilities() {
			if supported == capability {
				return adapter, true
			}
		}
	}
	return nil, false
}
