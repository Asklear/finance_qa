package query

import "context"

type SupplierPaymentSourceAdapter struct {
	engine *Engine
}

func NewSupplierPaymentSourceAdapter(engine *Engine) *SupplierPaymentSourceAdapter {
	return &SupplierPaymentSourceAdapter{engine: engine}
}

func (a *SupplierPaymentSourceAdapter) Name() string {
	return "supplier_payments"
}

func (a *SupplierPaymentSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{SourceCapabilitySupplierPayments}
}

func (a *SupplierPaymentSourceAdapter) Fetch(_ context.Context, spec QuerySpec) (FactSet, error) {
	summary, err := a.engine.collectSupplierPaymentSummary(spec.PeriodFrom, spec.PeriodTo)
	if err != nil {
		return FactSet{}, err
	}
	return buildSupplierPaymentFactSet(spec, summary), nil
}
