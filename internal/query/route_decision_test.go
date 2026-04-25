package query

import (
	"context"
	"strings"
	"testing"
)

func TestRouteDecisionQ1RevenueSelectsContractFundIncome(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "route-q1-revenue.sqlite")
	seedProbeContract(t, engine, "C001", "测试客户有限公司", "年度服务合同")
	for _, period := range []string{"2026-01", "2026-02", "2026-03"} {
		seedProbeFundIncome(t, engine, "C001", period, 100, 80, 100)
	}

	spec := BuildQuerySpec("2026年Q1收入是多少？", probeAnchor())
	routed, decision := engine.decideBossRoute(ctx, spec)

	if decision.SelectedSource != BossSourceContractAggregate {
		t.Fatalf("SelectedSource = %s, want %s", decision.SelectedSource, BossSourceContractAggregate)
	}
	if !routed.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = false, want true")
	}
	if routed.QueryFamily != QueryFamilyCoreMetric {
		t.Fatalf("QueryFamily = %s, want %s", routed.QueryFamily, QueryFamilyCoreMetric)
	}
	if !containsString(decision.PrimaryTables, "fin_fund_income") {
		t.Fatalf("PrimaryTables = %#v, want fin_fund_income", decision.PrimaryTables)
	}
}

func TestRouteDecisionSupplierCostSelectsCostSettlements(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "route-supplier-cost.sqlite")
	seedProbeContract(t, engine, "C-LY-001", "南京林悦智能科技有限公司", "林悦项目")
	seedProbeCostSettlement(t, engine, "C-LY-001", "2026-03", 21037.56, 0, 21037.56)

	spec := BuildQuerySpec("南京林悦智能科技有限公司2026年3月成本多少？", probeAnchor())
	routed, decision := engine.decideBossRoute(ctx, spec)

	if decision.SelectedSource != BossSourceContractAggregate {
		t.Fatalf("SelectedSource = %s, want %s", decision.SelectedSource, BossSourceContractAggregate)
	}
	if routed.QueryFamily != QueryFamilyContractDimension {
		t.Fatalf("QueryFamily = %s, want %s", routed.QueryFamily, QueryFamilyContractDimension)
	}
	if !routed.NeedsContractDimension {
		t.Fatalf("NeedsContractDimension = false, want true")
	}
	if !containsString(decision.PrimaryTables, "fin_cost_settlements") {
		t.Fatalf("PrimaryTables = %#v, want fin_cost_settlements", decision.PrimaryTables)
	}
}

func TestRouteDecisionContractMissingFallsBackWithReason(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "route-contract-missing.sqlite")
	seedProbeContract(t, engine, "C001", "测试客户有限公司", "年度服务合同")
	seedProbeFundIncome(t, engine, "C001", "2026-02", 100, 80, 100)

	spec := BuildQuerySpec("2026年3月收入是多少？", probeAnchor())
	routed, decision := engine.decideBossRoute(ctx, spec)

	if decision.SelectedSource != BossSourceContractAggregate {
		t.Fatalf("SelectedSource = %s, want %s", decision.SelectedSource, BossSourceContractAggregate)
	}
	if decision.FallbackReason == "" {
		t.Fatalf("FallbackReason missing: %+v", decision)
	}
	if !strings.Contains(decision.FallbackReason, "2026-03") {
		t.Fatalf("FallbackReason = %q, want requested period", decision.FallbackReason)
	}
	if !routed.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate should stay true so fallback is explicit")
	}
}

func TestRouteDecisionExplicitCashUsesBank(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "route-explicit-bank.sqlite")
	spec := BuildQuerySpec("2026年3月银行卡实际到账多少？", probeAnchor())
	routed, decision := engine.decideBossRoute(ctx, spec)

	if decision.SelectedSource != BossSourceBankStatement {
		t.Fatalf("SelectedSource = %s, want %s", decision.SelectedSource, BossSourceBankStatement)
	}
	if routed.PreferContractAggregate || routed.NeedsContractDimension {
		t.Fatalf("explicit bank route should not force contract route: %+v", routed)
	}
	if !containsString(decision.PrimaryTables, "bank_statement") {
		t.Fatalf("PrimaryTables = %#v, want bank_statement", decision.PrimaryTables)
	}
}

func seedProbeCostSettlement(t *testing.T, engine *Engine, contractID, period string, settlement, paid, invoice float64) {
	t.Helper()
	if _, err := engine.db.Exec(`
INSERT INTO fin_cost_settlements(contract_id, year_month, settlement_amount, paid_amount, invoice_amount)
VALUES (?, ?, ?, ?, ?)
`, contractID, period, settlement, paid, invoice); err != nil {
		t.Fatalf("seed cost settlement: %v", err)
	}
}
