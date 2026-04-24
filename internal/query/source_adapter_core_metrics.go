package query

import (
	"context"
)

type CoreMetricsSourceAdapter struct {
	engine *Engine
}

func NewCoreMetricsSourceAdapter(engine *Engine) *CoreMetricsSourceAdapter {
	return &CoreMetricsSourceAdapter{engine: engine}
}

func (a *CoreMetricsSourceAdapter) Name() string {
	return "core_metrics"
}

func (a *CoreMetricsSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{
		SourceCapabilityCashReceipts,
		SourceCapabilityBankCashReceipts,
		SourceCapabilityCashPayments,
		SourceCapabilityAccrualRevenue,
		SourceCapabilityAccrualCost,
		SourceCapabilityAccrualProfit,
		SourceCapabilityCashBridge,
	}
}

func (a *CoreMetricsSourceAdapter) Fetch(_ context.Context, spec QuerySpec) (FactSet, error) {
	return a.fetchFactSet(spec)
}
