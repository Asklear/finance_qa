package query

type SourceCapability string

const (
	SourceCapabilityCashReceipts     SourceCapability = "cash_receipts"
	SourceCapabilityBankCashReceipts SourceCapability = "bank_cash_receipts"
	SourceCapabilityCashPayments     SourceCapability = "cash_payments"
	SourceCapabilityAccrualRevenue   SourceCapability = "accrual_revenue"
	SourceCapabilityAccrualCost      SourceCapability = "accrual_cost"
	SourceCapabilityAccrualProfit    SourceCapability = "accrual_profit"
	SourceCapabilityCashBridge       SourceCapability = "cash_bridge"
	SourceCapabilityOfficialARAP     SourceCapability = "official_arap"
	SourceCapabilityOpenItemEvidence SourceCapability = "openitem_evidence"
	SourceCapabilityContractLedger   SourceCapability = "contract_ledger"
	SourceCapabilityContractDetail   SourceCapability = "contract_detail"
	SourceCapabilitySupplierPayments SourceCapability = "supplier_payment_fact"
	SourceCapabilityDataReadiness    SourceCapability = "data_readiness_fact"
)

type QueryPlan struct {
	QueryFamily  QueryFamily
	Capabilities []SourceCapability
}

func (p QueryPlan) Requires(capability SourceCapability) bool {
	for _, existing := range p.Capabilities {
		if existing == capability {
			return true
		}
	}
	return false
}

func PlanQuerySpec(spec QuerySpec) QueryPlan {
	plan := QueryPlan{QueryFamily: spec.QueryFamily}

	add := func(capability SourceCapability) {
		if !plan.Requires(capability) {
			plan.Capabilities = append(plan.Capabilities, capability)
		}
	}

	switch spec.QueryFamily {
	case QueryFamilyContractDetail:
		add(SourceCapabilityContractDetail)
	case QueryFamilyContractDimension:
		add(SourceCapabilityContractLedger)
		add(SourceCapabilityBankCashReceipts)
		if spec.MetricKind == MetricKindCost {
			add(SourceCapabilityCashPayments)
		}
	case QueryFamilyARAP:
		add(SourceCapabilityOfficialARAP)
		add(SourceCapabilityOpenItemEvidence)
	case QueryFamilySupplierPayments:
		add(SourceCapabilitySupplierPayments)
	case QueryFamilyReadiness:
		add(SourceCapabilityDataReadiness)
	case QueryFamilyCounterparty:
		add(SourceCapabilityBankCashReceipts)
		if spec.PerspectivePolicy == PerspectiveCashThenAccrual {
			add(SourceCapabilityAccrualRevenue)
		}
	case QueryFamilyCoreMetric:
		if spec.PreferContractAggregate {
			add(SourceCapabilityContractLedger)
		}
		switch spec.MetricKind {
		case MetricKindCost:
			add(SourceCapabilityCashPayments)
			add(SourceCapabilityAccrualCost)
		case MetricKindProfit:
			add(SourceCapabilityAccrualProfit)
			add(SourceCapabilityCashBridge)
		default:
			add(SourceCapabilityCashReceipts)
			add(SourceCapabilityAccrualRevenue)
		}
	case QueryFamilyHRCost:
		add(SourceCapabilityCashPayments)
		add(SourceCapabilityAccrualCost)
	}

	return plan
}
