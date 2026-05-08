package query

import "context"

type SupplierPaymentSourceAdapter struct {
	runtime SupplierPaymentSourceRuntime
}

func NewSupplierPaymentSourceAdapter(runtime SupplierPaymentSourceRuntime) *SupplierPaymentSourceAdapter {
	return &SupplierPaymentSourceAdapter{runtime: runtime}
}

func (a *SupplierPaymentSourceAdapter) Name() string {
	return "supplier_payments"
}

func (a *SupplierPaymentSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{SourceCapabilitySupplierPayments}
}

func (a *SupplierPaymentSourceAdapter) Fetch(_ context.Context, spec QuerySpec) (FactSet, error) {
	summary, err := a.runtime.collectSupplierPaymentSummary(spec.PeriodFrom, spec.PeriodTo)
	if err != nil {
		return FactSet{}, err
	}
	return buildSupplierPaymentFactSet(spec, summary), nil
}
