package query

import (
	"context"
	"testing"
	"time"
)

type fakeSupplierPaymentRuntime struct {
	summary supplierPaymentSummary
	called  bool
}

func (r *fakeSupplierPaymentRuntime) collectSupplierPaymentSummary(from, to string) (supplierPaymentSummary, error) {
	r.called = true
	if r.summary.Period == "" {
		r.summary.Period = displayPeriod(from, to)
	}
	return r.summary, nil
}

func TestSupplierPaymentSourceAdapterUsesRuntimeInterface(t *testing.T) {
	runtime := &fakeSupplierPaymentRuntime{
		summary: supplierPaymentSummary{
			Period: "2026-03",
			Total:  123.45,
			Suppliers: []map[string]any{
				{"name": "测试供应商", "amount": 123.45},
			},
		},
	}
	adapter := NewSupplierPaymentSourceAdapter(runtime)

	factSet, err := adapter.Fetch(context.Background(), QuerySpec{
		Entity:     "测试供应商",
		PeriodFrom: "2026-03",
		PeriodTo:   "2026-03",
	})
	if err != nil {
		t.Fatalf("fetch supplier payment facts: %v", err)
	}
	if !runtime.called {
		t.Fatalf("expected adapter to call runtime")
	}
	if factSet.Source != "supplier_payments" {
		t.Fatalf("source = %q, want supplier_payments", factSet.Source)
	}
	if len(factSet.Facts) != 3 {
		t.Fatalf("facts = %d, want 3", len(factSet.Facts))
	}
}

type fakeDefaultSourceRuntime struct {
	ruleConfig RuleConfig

	contractDimensionSummary contractDimensionSummary
	contractDimensionCalled  bool

	contractDetailResult ContractDetailResult
	contractDetailCalled bool

	arapResult Result
	arapCalled bool
	arapType   string

	coreCoverage       coreMetricCoverage
	coreCoverageCalled bool
	coreComputeCalled  bool

	supplierSummary supplierPaymentSummary
	supplierCalled  bool

	readinessSummary readinessSummary
	readinessCalled  bool
}

func (r *fakeDefaultSourceRuntime) currentRuleConfig() RuleConfig {
	return r.ruleConfig
}

func (r *fakeDefaultSourceRuntime) collectContractAggregateSummary(spec QuerySpec) (contractAggregateSummary, error) {
	return contractAggregateSummary{
		Entity:           spec.Entity,
		Period:           displayPeriod(spec.PeriodFrom, spec.PeriodTo),
		PeriodFrom:       spec.PeriodFrom,
		PeriodTo:         spec.PeriodTo,
		RequestedMetrics: []string{"收入"},
	}, nil
}

func (r *fakeDefaultSourceRuntime) collectContractDimensionSummaryForPeriod(question, entity, from, to string) (contractDimensionSummary, error) {
	r.contractDimensionCalled = true
	if r.contractDimensionSummary.Entity == "" {
		r.contractDimensionSummary.Entity = entity
	}
	if r.contractDimensionSummary.Period == "" {
		r.contractDimensionSummary.Period = displayPeriod(from, to)
	}
	return r.contractDimensionSummary, nil
}

func (r *fakeDefaultSourceRuntime) collectContractDimensionSummary(question, entity string, anchor time.Time) (contractDimensionSummary, error) {
	r.contractDimensionCalled = true
	return r.contractDimensionSummary, nil
}

func (r *fakeDefaultSourceRuntime) getLatestContractPeriodAnchor() time.Time {
	return time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
}

func (r *fakeDefaultSourceRuntime) ProbeContractDetailSources(spec QuerySpec) ContractDetailProbeResult {
	return ContractDetailProbeResult{Intent: ContractDetailIntentField, HasStructuredAnswer: true, Confidence: 1}
}

func (r *fakeDefaultSourceRuntime) collectContractDetail(ctx context.Context, spec QuerySpec, probe ContractDetailProbeResult) (ContractDetailResult, error) {
	r.contractDetailCalled = true
	return r.contractDetailResult, nil
}

func (r *fakeDefaultSourceRuntime) queryAccountPayableReceivable(period, accountName, accountCodePrefix, typ, entity string) Result {
	r.arapCalled = true
	r.arapType = typ
	return r.arapResult
}

func (r *fakeDefaultSourceRuntime) resolveCoreMetricCoverageForRequest(from, to string, request coreMetricRequest) coreMetricCoverage {
	r.coreCoverageCalled = true
	return r.coreCoverage
}

func (r *fakeDefaultSourceRuntime) computeUnifiedCoreMetrics(from, to string) (*unifiedCoreMetrics, []string, []string, error) {
	r.coreComputeCalled = true
	return &unifiedCoreMetrics{Period: displayPeriod(from, to)}, nil, nil, nil
}

func (r *fakeDefaultSourceRuntime) collectSupplierPaymentSummary(from, to string) (supplierPaymentSummary, error) {
	r.supplierCalled = true
	if r.supplierSummary.Period == "" {
		r.supplierSummary.Period = displayPeriod(from, to)
	}
	return r.supplierSummary, nil
}

func (r *fakeDefaultSourceRuntime) collectEntityDataReadiness(entity, from, to string) (readinessSummary, error) {
	r.readinessCalled = true
	if r.readinessSummary.Entity == "" {
		r.readinessSummary.Entity = entity
	}
	if r.readinessSummary.Period == "" {
		r.readinessSummary.Period = displayPeriod(from, to)
	}
	return r.readinessSummary, nil
}

func TestDefaultSourceRegistryAcceptsRuntimeInterface(t *testing.T) {
	runtime := &fakeDefaultSourceRuntime{}
	registry := NewDefaultSourceRegistry(runtime)

	cases := []struct {
		capability SourceCapability
		wantName   string
	}{
		{capability: SourceCapabilityContractDetail, wantName: "contract_detail"},
		{capability: SourceCapabilityContractLedger, wantName: "contracts"},
		{capability: SourceCapabilityAccrualRevenue, wantName: "core_metrics"},
		{capability: SourceCapabilityOfficialARAP, wantName: "arap"},
		{capability: SourceCapabilityDataReadiness, wantName: "data_readiness"},
		{capability: SourceCapabilitySupplierPayments, wantName: "supplier_payments"},
	}

	for _, tc := range cases {
		adapter, ok := registry.Resolve(tc.capability)
		if !ok {
			t.Fatalf("expected adapter for capability %s", tc.capability)
		}
		if adapter.Name() != tc.wantName {
			t.Fatalf("adapter for %s = %s, want %s", tc.capability, adapter.Name(), tc.wantName)
		}
	}
}

func TestContractSourceAdapterUsesRuntimeInterfaceForDimensionFacts(t *testing.T) {
	runtime := &fakeDefaultSourceRuntime{
		contractDimensionSummary: contractDimensionSummary{
			Entity:    "测试客户",
			Role:      "customer_contract",
			Period:    "2026-03",
			Contracts: []map[string]any{{"contract_id": "C-001"}},
			Data: map[string]any{
				"book_view": map[string]any{
					"settlement_amount": 100.0,
					"invoice_amount":    90.0,
				},
				"cash_view": map[string]any{
					"received_amount": 80.0,
				},
			},
		},
	}
	adapter := NewContractSourceAdapter(runtime)

	factSet, err := adapter.Fetch(context.Background(), QuerySpec{
		OriginalQuestion: "测试客户3月合同收入多少？",
		Entity:           "测试客户",
		PeriodFrom:       "2026-03",
		PeriodTo:         "2026-03",
	})
	if err != nil {
		t.Fatalf("fetch contract facts: %v", err)
	}
	if !runtime.contractDimensionCalled {
		t.Fatalf("expected adapter to call runtime")
	}
	assertInternalFactValue(t, factSet, "contract_book_settlement", 100)
	assertInternalFactValue(t, factSet, "contract_cash_received", 80)
}

func TestContractDetailSourceAdapterUsesRuntimeInterface(t *testing.T) {
	runtime := &fakeDefaultSourceRuntime{
		contractDetailResult: ContractDetailResult{
			MatchedContracts: []ContractDetailMatch{
				{
					ContractTitle: "测试合同",
					PaymentTerms:  "按月结算",
				},
			},
			SourceTables: []string{"contract_master"},
			Probe: ContractDetailProbeResult{
				Intent:              ContractDetailIntentField,
				HasStructuredAnswer: true,
				Confidence:          0.9,
			},
		},
	}
	adapter := NewContractDetailSourceAdapter(runtime)

	factSet, err := adapter.Fetch(context.Background(), QuerySpec{
		OriginalQuestion: "测试合同付款条款是什么？",
	})
	if err != nil {
		t.Fatalf("fetch contract detail facts: %v", err)
	}
	if !runtime.contractDetailCalled {
		t.Fatalf("expected adapter to call runtime")
	}
	if factSet.Source != "contract_detail" || len(factSet.Facts) != 1 {
		t.Fatalf("factSet = %#v, want one contract detail fact", factSet)
	}
	if factSet.Facts[0].CoverageStatus != CoverageFull {
		t.Fatalf("coverage = %s, want %s", factSet.Facts[0].CoverageStatus, CoverageFull)
	}
}

func TestCoreMetricsSourceAdapterUsesRuntimeInterfaceForMissingCoverage(t *testing.T) {
	runtime := &fakeDefaultSourceRuntime{
		coreCoverage: coreMetricCoverage{
			RequestedFrom: "2026-05",
			RequestedTo:   "2026-05",
			AvailableTo:   "2026-04",
			HasData:       false,
		},
	}
	adapter := NewCoreMetricsSourceAdapter(runtime)

	factSet, err := adapter.Fetch(context.Background(), QuerySpec{
		OriginalQuestion: "2026年5月收入是多少？",
		MetricKind:       MetricKindRevenue,
		PeriodFrom:       "2026-05",
		PeriodTo:         "2026-05",
	})
	if err != nil {
		t.Fatalf("fetch core metric facts: %v", err)
	}
	if !runtime.coreCoverageCalled {
		t.Fatalf("expected adapter to check coverage through runtime")
	}
	if runtime.coreComputeCalled {
		t.Fatalf("expected adapter to skip core metric computation when coverage is missing")
	}
	if factSet.Source != "core_metrics" || len(factSet.Facts) != 1 {
		t.Fatalf("factSet = %#v, want one core_metrics missing fact", factSet)
	}
	if factSet.Facts[0].CoverageStatus != CoverageMissing {
		t.Fatalf("coverage = %s, want %s", factSet.Facts[0].CoverageStatus, CoverageMissing)
	}
}

func TestARAPSourceAdapterUsesRuntimeInterfaceForOfficialFact(t *testing.T) {
	runtime := &fakeDefaultSourceRuntime{
		arapResult: Result{
			Success: true,
			Data: map[string]any{
				"source":          "balance_sheet",
				"total":           456.0,
				"opening_balance": 123.0,
			},
		},
	}
	adapter := NewARAPSourceAdapter(runtime)

	factSet, err := adapter.Fetch(context.Background(), QuerySpec{
		NormalizedQuestion: "2026年3月应收账款余额",
		Entity:             "测试客户",
		PeriodTo:           "2026-03",
	})
	if err != nil {
		t.Fatalf("fetch arap facts: %v", err)
	}
	if !runtime.arapCalled || runtime.arapType != "receivable" {
		t.Fatalf("runtime call = %t type=%s, want receivable call", runtime.arapCalled, runtime.arapType)
	}
	assertInternalFactValue(t, factSet, "official_arap_total", 456)
	assertInternalFactValue(t, factSet, "official_arap_opening_balance", 123)
}

func TestReadinessSourceAdapterUsesRuntimeInterface(t *testing.T) {
	runtime := &fakeDefaultSourceRuntime{
		readinessSummary: readinessSummary{
			Entity:      "测试客户",
			Period:      "2026-03",
			HasData:     true,
			Rows:        3,
			JournalRows: 1,
			BankRows:    2,
		},
	}
	adapter := NewReadinessSourceAdapter(runtime)

	factSet, err := adapter.Fetch(context.Background(), QuerySpec{
		Entity:     "测试客户",
		PeriodFrom: "2026-03",
		PeriodTo:   "2026-03",
	})
	if err != nil {
		t.Fatalf("fetch readiness facts: %v", err)
	}
	if !runtime.readinessCalled {
		t.Fatalf("expected adapter to call runtime")
	}
	assertInternalFactValue(t, factSet, "readiness_has_data", 1)
	assertInternalFactValue(t, factSet, "readiness_row_count", 3)
}
