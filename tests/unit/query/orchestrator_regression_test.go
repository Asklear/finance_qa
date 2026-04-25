package query_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"financeqa/internal/query"
)

type stubSourceAdapter struct {
	name         string
	capabilities []query.SourceCapability
	calls        *[]string
}

func (s stubSourceAdapter) Name() string {
	return s.name
}

func (s stubSourceAdapter) Capabilities() []query.SourceCapability {
	return append([]query.SourceCapability{}, s.capabilities...)
}

func (s stubSourceAdapter) Fetch(_ context.Context, spec query.QuerySpec) (query.FactSet, error) {
	if s.calls != nil {
		*s.calls = append(*s.calls, s.name)
	}
	return query.FactSet{
		Source: s.name,
		Facts: []query.Fact{
			{
				Source:         s.name,
				MetricKey:      string(spec.MetricKind),
				Entity:         spec.Entity,
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				AuthorityLevel: query.AuthoritySupporting,
				CoverageStatus: query.CoverageFull,
				Confidence:     1,
			},
		},
	}, nil
}

func TestSourceRegistryResolvesAdapterByCapability(t *testing.T) {
	registry := query.NewSourceRegistry()
	registry.Register(stubSourceAdapter{
		name:         "core-metrics",
		capabilities: []query.SourceCapability{query.SourceCapabilityCashReceipts, query.SourceCapabilityAccrualRevenue},
	})

	adapter, ok := registry.Resolve(query.SourceCapabilityAccrualRevenue)
	if !ok {
		t.Fatalf("expected adapter for accrual revenue")
	}
	if adapter.Name() != "core-metrics" {
		t.Fatalf("adapter name = %s, want core-metrics", adapter.Name())
	}
}

func TestOrchestratorExecutesPlannedAdaptersInOrder(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("2026年3月收入是多少？", now)

	calls := []string{}
	registry := query.NewSourceRegistry()
	registry.Register(stubSourceAdapter{
		name:         "contract-adapter",
		capabilities: []query.SourceCapability{query.SourceCapabilityContractLedger},
		calls:        &calls,
	})
	registry.Register(stubSourceAdapter{
		name:         "cash-adapter",
		capabilities: []query.SourceCapability{query.SourceCapabilityCashReceipts},
		calls:        &calls,
	})
	registry.Register(stubSourceAdapter{
		name:         "book-adapter",
		capabilities: []query.SourceCapability{query.SourceCapabilityAccrualRevenue},
		calls:        &calls,
	})

	orchestrator := query.NewOrchestrator(registry)
	result, err := orchestrator.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(result.FactSets) != 3 {
		t.Fatalf("FactSets len = %d, want 3", len(result.FactSets))
	}
	if len(calls) != 3 || calls[0] != "contract-adapter" || calls[1] != "cash-adapter" || calls[2] != "book-adapter" {
		t.Fatalf("adapter calls = %#v, want [contract-adapter cash-adapter book-adapter]", calls)
	}
	if result.Plan.QueryFamily != query.QueryFamilyCoreMetric {
		t.Fatalf("result plan family = %s, want %s", result.Plan.QueryFamily, query.QueryFamilyCoreMetric)
	}
}

func TestOrchestratorReturnsStructuredErrorWhenCapabilityMissing(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("2026年3月应收账款多少？", now)

	orchestrator := query.NewOrchestrator(query.NewSourceRegistry())
	_, err := orchestrator.Execute(context.Background(), spec)
	if err == nil {
		t.Fatalf("expected missing capability error")
	}
	var missing *query.MissingCapabilityError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %T, want *MissingCapabilityError", err)
	}
	if missing.Capability != query.SourceCapabilityContractLedger {
		t.Fatalf("missing capability = %s, want %s", missing.Capability, query.SourceCapabilityContractLedger)
	}
}

func TestDefaultSourceRegistryRegistersMigratedAdapters(t *testing.T) {
	dbPath := buildProfitBridgeQueryDB(t)
	engine, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	registry := query.NewDefaultSourceRegistry(engine)

	cases := []struct {
		capability query.SourceCapability
		wantName   string
	}{
		{capability: query.SourceCapabilityAccrualRevenue, wantName: "core_metrics"},
		{capability: query.SourceCapabilityOfficialARAP, wantName: "arap"},
		{capability: query.SourceCapabilityContractLedger, wantName: "contracts"},
		{capability: query.SourceCapabilityDataReadiness, wantName: "data_readiness"},
		{capability: query.SourceCapabilitySupplierPayments, wantName: "supplier_payments"},
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
