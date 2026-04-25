package query

import (
	"testing"
	"time"
)

func TestRewriteBossQueryQ1Revenue(t *testing.T) {
	anchor := time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC)

	got := RewriteBossQuery("2026年Q1收入是多少？", anchor)

	if got.Metric != BossMetricRevenue {
		t.Fatalf("Metric = %s, want %s", got.Metric, BossMetricRevenue)
	}
	if got.Scope != BossScopeCompany {
		t.Fatalf("Scope = %s, want %s", got.Scope, BossScopeCompany)
	}
	if got.PeriodFrom != "2026-01" || got.PeriodTo != "2026-03" {
		t.Fatalf("period = %s~%s, want 2026-01~2026-03", got.PeriodFrom, got.PeriodTo)
	}
	if got.Granularity != BossGranularityAggregate {
		t.Fatalf("Granularity = %s, want %s", got.Granularity, BossGranularityAggregate)
	}
	if got.Perspective != BossPerspectiveContractFirst {
		t.Fatalf("Perspective = %s, want %s", got.Perspective, BossPerspectiveContractFirst)
	}
	if got.SourceConstraint != "" {
		t.Fatalf("SourceConstraint = %q, want empty", got.SourceConstraint)
	}
	if !got.RequiresSourceProbe {
		t.Fatalf("RequiresSourceProbe = false, want true")
	}
}

func TestRewriteBossQuerySupplierCost(t *testing.T) {
	anchor := time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC)

	got := RewriteBossQuery("南京林悦智能科技有限公司3月成本多少？", anchor)

	if got.Metric != BossMetricCost {
		t.Fatalf("Metric = %s, want %s", got.Metric, BossMetricCost)
	}
	if got.Scope != BossScopeEntity {
		t.Fatalf("Scope = %s, want %s", got.Scope, BossScopeEntity)
	}
	if got.Entity != "南京林悦智能科技有限公司" {
		t.Fatalf("Entity = %q", got.Entity)
	}
	if got.PeriodFrom != "2026-03" || got.PeriodTo != "2026-03" {
		t.Fatalf("period = %s~%s, want 2026-03~2026-03", got.PeriodFrom, got.PeriodTo)
	}
	if got.Perspective != BossPerspectiveContractFirst {
		t.Fatalf("Perspective = %s, want %s", got.Perspective, BossPerspectiveContractFirst)
	}
}

func TestRewriteBossQueryExplicitBankCash(t *testing.T) {
	anchor := time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC)

	got := RewriteBossQuery("2026年3月银行卡实际到账多少？", anchor)

	if got.Metric != BossMetricReceipts {
		t.Fatalf("Metric = %s, want %s", got.Metric, BossMetricReceipts)
	}
	if got.Scope != BossScopeCompany {
		t.Fatalf("Scope = %s, want %s", got.Scope, BossScopeCompany)
	}
	if got.PeriodFrom != "2026-03" || got.PeriodTo != "2026-03" {
		t.Fatalf("period = %s~%s, want 2026-03~2026-03", got.PeriodFrom, got.PeriodTo)
	}
	if got.Perspective != BossPerspectiveExplicitCash {
		t.Fatalf("Perspective = %s, want %s", got.Perspective, BossPerspectiveExplicitCash)
	}
	if got.SourceConstraint != BossSourceBankStatement {
		t.Fatalf("SourceConstraint = %q, want %q", got.SourceConstraint, BossSourceBankStatement)
	}
}

func TestRewriteBossQueryBareYearCumulativeRevenue(t *testing.T) {
	anchor := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)

	got := RewriteBossQuery("飞未云科2026累计销售额多少？", anchor)

	if got.Metric != BossMetricRevenue {
		t.Fatalf("Metric = %s, want %s", got.Metric, BossMetricRevenue)
	}
	if got.PeriodFrom != "2026-01" || got.PeriodTo != "2026-03" {
		t.Fatalf("period = %s~%s, want 2026-01~2026-03", got.PeriodFrom, got.PeriodTo)
	}
	if got.Granularity != BossGranularityAggregate {
		t.Fatalf("Granularity = %s, want %s", got.Granularity, BossGranularityAggregate)
	}
}

func TestBuildQuerySpecCarriesBossRewrite(t *testing.T) {
	anchor := time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC)

	spec := BuildQuerySpec("2026年Q1利润多少？", anchor)

	if spec.BossRewrite.Metric != BossMetricProfit {
		t.Fatalf("BossRewrite.Metric = %s, want %s", spec.BossRewrite.Metric, BossMetricProfit)
	}
	if spec.BossRewrite.Perspective != BossPerspectiveContractFirst {
		t.Fatalf("BossRewrite.Perspective = %s, want %s", spec.BossRewrite.Perspective, BossPerspectiveContractFirst)
	}
	if spec.SourceConstraint != "" {
		t.Fatalf("SourceConstraint = %q, want empty", spec.SourceConstraint)
	}
}

func TestBuildQuerySpecEnvelopeIncludesBossRewrite(t *testing.T) {
	spec := QuerySpec{
		QueryFamily: QueryFamilyCoreMetric,
		MetricKind:  MetricKindRevenue,
		BossRewrite: BossQueryRewrite{
			Metric:              BossMetricRevenue,
			Scope:               BossScopeCompany,
			PeriodFrom:          "2026-01",
			PeriodTo:            "2026-03",
			Granularity:         BossGranularityAggregate,
			Perspective:         BossPerspectiveContractFirst,
			RequiresSourceProbe: true,
		},
	}

	envelope := buildQuerySpecEnvelope(spec)
	raw, ok := envelope["boss_rewrite"].(map[string]any)
	if !ok {
		t.Fatalf("boss_rewrite missing from envelope: %#v", envelope)
	}
	if raw["metric"] != BossMetricRevenue {
		t.Fatalf("boss_rewrite.metric = %v, want %s", raw["metric"], BossMetricRevenue)
	}
	if raw["perspective"] != BossPerspectiveContractFirst {
		t.Fatalf("boss_rewrite.perspective = %v, want %s", raw["perspective"], BossPerspectiveContractFirst)
	}
}
