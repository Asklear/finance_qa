package query

import (
	"context"
	"time"
)

type ContractSourceRuntime interface {
	currentRuleConfig() RuleConfig
	collectContractAggregateSummary(spec QuerySpec) (contractAggregateSummary, error)
	collectContractDimensionSummaryForPeriod(question, entity, from, to string) (contractDimensionSummary, error)
	collectContractDimensionSummary(question, entity string, anchor time.Time) (contractDimensionSummary, error)
	getLatestContractPeriodAnchor() time.Time
}

type ContractDetailSourceRuntime interface {
	ProbeContractDetailSources(spec QuerySpec) ContractDetailProbeResult
	collectContractDetail(ctx context.Context, spec QuerySpec, probe ContractDetailProbeResult) (ContractDetailResult, error)
}

type ARAPSourceRuntime interface {
	queryAccountPayableReceivable(period, accountName, accountCodePrefix, typ, entity string) Result
}

type CoreMetricsSourceRuntime interface {
	currentRuleConfig() RuleConfig
	resolveCoreMetricCoverageForRequest(from, to string, request coreMetricRequest) coreMetricCoverage
	computeUnifiedCoreMetrics(from, to string) (*unifiedCoreMetrics, []string, []string, error)
}

type SupplierPaymentSourceRuntime interface {
	collectSupplierPaymentSummary(from, to string) (supplierPaymentSummary, error)
}

type ReadinessSourceRuntime interface {
	collectEntityDataReadiness(entity, from, to string) (readinessSummary, error)
}

// DefaultSourceRuntime is the runtime surface required to wire built-in source adapters.
type DefaultSourceRuntime interface {
	ContractSourceRuntime
	ContractDetailSourceRuntime
	ARAPSourceRuntime
	CoreMetricsSourceRuntime
	SupplierPaymentSourceRuntime
	ReadinessSourceRuntime
}

var _ DefaultSourceRuntime = (*Engine)(nil)
