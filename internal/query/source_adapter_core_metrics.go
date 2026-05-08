package query

import (
	"context"
)

type CoreMetricsSourceAdapter struct {
	runtime CoreMetricsSourceRuntime
}

func NewCoreMetricsSourceAdapter(runtime CoreMetricsSourceRuntime) *CoreMetricsSourceAdapter {
	return &CoreMetricsSourceAdapter{runtime: runtime}
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
