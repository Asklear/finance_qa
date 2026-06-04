package query

import "time"

type QueryFamily string

const (
	QueryFamilyGeneral           QueryFamily = "general"
	QueryFamilyCoreMetric        QueryFamily = "core_metric"
	QueryFamilyCounterparty      QueryFamily = "counterparty"
	QueryFamilyContractDimension QueryFamily = "contract_dimension"
	QueryFamilyContractDetail    QueryFamily = "contract_detail"
	QueryFamilyARAP              QueryFamily = "arap"
	QueryFamilySupplierPayments  QueryFamily = "supplier_payments"
	QueryFamilyReadiness         QueryFamily = "readiness"
	QueryFamilyReconciliation    QueryFamily = "reconciliation"
	QueryFamilyHRCost            QueryFamily = "hr_cost"
)

type MetricKind string

const (
	MetricKindUnknown  MetricKind = "unknown"
	MetricKindRevenue  MetricKind = "revenue"
	MetricKindCost     MetricKind = "cost"
	MetricKindProfit   MetricKind = "profit"
	MetricKindReceipts MetricKind = "receipts"
)

type TimeScope string

const (
	TimeScopeMonth      TimeScope = "month"
	TimeScopeQuarter    TimeScope = "quarter"
	TimeScopeHalfYear   TimeScope = "half_year"
	TimeScopeYearFull   TimeScope = "year_full"
	TimeScopeYearToDate TimeScope = "year_to_date"
	TimeScopeCustom     TimeScope = "custom_range"
)

type PerspectivePolicy string

const (
	PerspectiveUnknown              PerspectivePolicy = "unknown"
	PerspectiveCashThenAccrual      PerspectivePolicy = "cash_then_accrual"
	PerspectiveAccrualOnly          PerspectivePolicy = "accrual_only"
	PerspectiveOfficialThenEvidence PerspectivePolicy = "official_then_evidence"
)

type QuerySpec struct {
	OriginalQuestion            string
	NormalizedQuestion          string
	Intent                      Intent
	QueryFamily                 QueryFamily
	MetricKind                  MetricKind
	Entity                      string
	PeriodFrom                  string
	PeriodTo                    string
	SubPeriod                   string
	TimeScope                   TimeScope
	PerspectivePolicy           PerspectivePolicy
	NeedsContractDimension      bool
	PreferContractAggregate     bool
	ReadinessCheckRequired      bool
	AuthoritativeSourceRequired bool
	OpeningPeriodAware          bool
	BossRewrite                 BossQueryRewrite
	SourceConstraint            string
	RouteDecision               RouteDecision
	LexiconProfile              string
	AsOf                        string
	SemanticFamilies            []string
}

func BuildQuerySpec(question string, anchor time.Time) QuerySpec {
	return buildQuerySpec(question, anchor, getRuleConfig())
}
