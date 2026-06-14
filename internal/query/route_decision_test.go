package query

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRouteDecisionContractARAPSelectsRevenueCostTables(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "route-contract-arap.sqlite")
	seedProbeContract(t, engine, "C-REV-001", "测试客户有限公司", "客户项目")
	seedProbeContract(t, engine, "C-COST-001", "测试供应商有限公司", "供应商项目")
	seedProbeFundIncome(t, engine, "C-REV-001", "2026-03", 1000, 600, 800)
	seedProbeCostSettlement(t, engine, "C-COST-001", "2026-03", 700, 200, 500)

	spec := BuildQuerySpec("2026年3月合同应收应付分别是多少？", probeAnchor())
	routed, decision := engine.decideBossRoute(ctx, spec)

	if decision.SelectedSource != BossSourceContractAggregate {
		t.Fatalf("SelectedSource = %s, want %s", decision.SelectedSource, BossSourceContractAggregate)
	}
	if !routed.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = false, want true")
	}
	for _, want := range []string{"fin_fund_income", "fin_cost_settlements"} {
		if !containsString(decision.PrimaryTables, want) {
			t.Fatalf("PrimaryTables = %#v, want %s", decision.PrimaryTables, want)
		}
	}
}

func TestRouteDecisionProjectMarginSelectsRevenueCostTables(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "route-project-margin.sqlite")
	seedProbeContract(t, engine, "C-REV-001", "测试客户有限公司", "客户项目")
	seedProbeContract(t, engine, "C-COST-001", "测试供应商有限公司", "供应商项目")
	seedProbeFundIncome(t, engine, "C-REV-001", "2026-03", 1000, 600, 800)
	seedProbeCostSettlement(t, engine, "C-COST-001", "2026-03", 700, 200, 500)

	spec := BuildQuerySpec("2026年3月项目毛利是多少？", probeAnchor())
	routed, decision := engine.decideBossRoute(ctx, spec)

	if decision.SelectedSource != BossSourceContractAggregate {
		t.Fatalf("SelectedSource = %s, want %s", decision.SelectedSource, BossSourceContractAggregate)
	}
	if !routed.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = false, want true")
	}
	for _, want := range []string{"fin_fund_income", "fin_cost_settlements"} {
		if !containsString(decision.PrimaryTables, want) {
			t.Fatalf("PrimaryTables = %#v, want %s", decision.PrimaryTables, want)
		}
	}
}

func TestRouteDecisionRelativeIncomeCostDefaultsToContractAggregate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "route-relative-income-cost.sqlite")
	seedProbeContract(t, engine, "C-REV-001", "测试客户有限公司", "客户项目")
	seedProbeContract(t, engine, "C-COST-001", "测试供应商有限公司", "供应商项目")
	seedProbeFundIncome(t, engine, "C-REV-001", "2026-05", 1000, 600, 800)
	seedProbeCostSettlement(t, engine, "C-COST-001", "2026-05", 700, 200, 500)

	spec := BuildQuerySpec("上个月收入和成本是多少？", time.Date(2026, time.June, 14, 0, 0, 0, 0, time.UTC))
	routed, decision := engine.decideBossRoute(ctx, spec)

	if decision.SelectedSource != BossSourceContractAggregate {
		t.Fatalf("SelectedSource = %s, want %s", decision.SelectedSource, BossSourceContractAggregate)
	}
	if routed.QueryFamily != QueryFamilyCoreMetric {
		t.Fatalf("QueryFamily = %s, want %s (rewrite=%+v entity=%q needs=%t)", routed.QueryFamily, QueryFamilyCoreMetric, routed.BossRewrite, routed.Entity, routed.NeedsContractDimension)
	}
	if routed.NeedsContractDimension {
		t.Fatalf("NeedsContractDimension = true, want false")
	}
	if !routed.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = false, want true")
	}
	if routed.PeriodFrom != "2026-05" || routed.PeriodTo != "2026-05" {
		t.Fatalf("period = %s~%s, want 2026-05~2026-05", routed.PeriodFrom, routed.PeriodTo)
	}
	if !containsString(decision.PrimaryTables, "fin_cost_settlements") {
		t.Fatalf("PrimaryTables = %#v, want fin_cost_settlements", decision.PrimaryTables)
	}
}

func TestQueryRelativeIncomeCostUsesContractAggregate(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "query-relative-income-cost.sqlite")
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 14, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	seedProbeContract(t, engine, "C-REV-001", "测试客户有限公司", "客户项目")
	seedProbeContract(t, engine, "C-COST-001", "测试供应商有限公司", "供应商项目")
	seedProbeFundIncome(t, engine, "C-REV-001", "2026-05", 1000, 600, 800)
	seedProbeCostSettlement(t, engine, "C-COST-001", "2026-05", 700, 200, 500)

	result := engine.Query("上个月收入和成本是多少？")
	if !result.Success {
		t.Fatalf("query failed: %s data=%+v", result.Message, result.Data)
	}
	if strings.Contains(result.Message, "账务数据仅到") {
		t.Fatalf("query should not be answered by core metric coverage guard: %s", result.Message)
	}
	for _, want := range []string{"2026-05", "1000.00", "700.00"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message = %q, want %q", result.Message, want)
		}
	}
	if got := result.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; data=%+v", got, result.Data)
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
