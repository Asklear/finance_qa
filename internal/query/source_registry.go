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

func NewDefaultSourceRegistry(engine *Engine) *SourceRegistry {
	registry := NewSourceRegistry()
	if engine == nil {
		return registry
	}
	registry.Register(NewContractSourceAdapter(engine))
	registry.Register(NewCoreMetricsSourceAdapter(engine))
	registry.Register(NewARAPSourceAdapter(engine))
	registry.Register(NewReadinessSourceAdapter(engine))
	registry.Register(NewSupplierPaymentSourceAdapter(engine))
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
