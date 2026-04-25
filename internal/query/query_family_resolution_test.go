package query

import "testing"

func TestResolveForcedQueryFamilyPromotesContractDimension(t *testing.T) {
	got, ok := resolveForcedQueryFamily(true)
	if !ok {
		t.Fatalf("expected forced family resolution")
	}
	if got != QueryFamilyContractDimension {
		t.Fatalf("forced query family = %s, want %s", got, QueryFamilyContractDimension)
	}
}

func TestResolveOperationalQueryFamilyPrioritizesOperationalRoutes(t *testing.T) {
	cfg := getRuleConfig()

	cases := []struct {
		name   string
		q      string
		intent Intent
		want   QueryFamily
	}{
		{
			name:   "readiness",
			q:      "南京林悦智能科技有限公司3月数据出来了吗？",
			intent: IntentGeneral,
			want:   QueryFamilyReadiness,
		},
		{
			name:   "supplier_payments",
			q:      "2026年3月有多少家供应商发生付款？",
			intent: IntentFallback,
			want:   QueryFamilySupplierPayments,
		},
		{
			name:   "hr_cost",
			q:      "2026年3月人力成本多少？工资、社保、公积金分别是多少？",
			intent: IntentFallback,
			want:   QueryFamilyHRCost,
		},
		{
			name:   "contract_first_arap",
			q:      "2026年3月应付账款多少（已收发票未付款）？",
			intent: IntentARAPQuery,
			want:   QueryFamilyCoreMetric,
		},
		{
			name:   "explicit_official_arap",
			q:      "2026年3月科目余额中的应付账款多少？",
			intent: IntentARAPQuery,
			want:   QueryFamilyARAP,
		},
		{
			name:   "reconciliation",
			q:      "为什么2026年3月收入和利润差异这么大？",
			intent: IntentAnalysis,
			want:   QueryFamilyReconciliation,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveOperationalQueryFamily(tc.q, tc.intent, cfg)
			if !ok {
				t.Fatalf("expected operational route for %q", tc.q)
			}
			if got != tc.want {
				t.Fatalf("operational query family = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestResolveMetricDrivenQueryFamilySeparatesAggregateAndCounterparty(t *testing.T) {
	cfg := getRuleConfig()

	tests := []struct {
		name             string
		q                string
		entity           string
		from             string
		to               string
		hasRealishEntity bool
		want             QueryFamily
	}{
		{
			name:             "boss_aggregate_summary",
			q:                "2026年Q1营收多少？",
			entity:           "",
			from:             "2026-01",
			to:               "2026-03",
			hasRealishEntity: false,
			want:             QueryFamilyCoreMetric,
		},
		{
			name:             "counterparty_metric",
			q:                "飞未云科2026年3月回款多少？",
			entity:           "飞未云科（深圳）技术有限公司",
			from:             "2026-03",
			to:               "2026-03",
			hasRealishEntity: true,
			want:             QueryFamilyCounterparty,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveMetricDrivenQueryFamily(tc.q, tc.entity, tc.from, tc.to, cfg, tc.hasRealishEntity)
			if !ok {
				t.Fatalf("expected metric-driven route for %q", tc.q)
			}
			if got != tc.want {
				t.Fatalf("metric-driven query family = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestResolveFallbackQueryFamilyUsesIntentAndEntity(t *testing.T) {
	if got := resolveFallbackQueryFamily(IntentMonthlySummary, false); got != QueryFamilyCoreMetric {
		t.Fatalf("monthly summary fallback = %s, want %s", got, QueryFamilyCoreMetric)
	}
	if got := resolveFallbackQueryFamily(IntentFallback, true); got != QueryFamilyCounterparty {
		t.Fatalf("fallback with entity = %s, want %s", got, QueryFamilyCounterparty)
	}
	if got := resolveFallbackQueryFamily(IntentGeneral, false); got != QueryFamilyGeneral {
		t.Fatalf("general fallback = %s, want %s", got, QueryFamilyGeneral)
	}
}
