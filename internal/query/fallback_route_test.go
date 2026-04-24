package query

import "testing"

func TestResolveFallbackRoute(t *testing.T) {
	cfg := getRuleConfig()

	tests := []struct {
		name string
		ctx  fallbackRouteContext
		want fallbackRoute
	}{
		{
			name: "reconciliation_first",
			ctx: fallbackRouteContext{
				q:   "为什么2026年3月收入和利润差异这么大？",
				cfg: cfg,
			},
			want: fallbackRouteReconciliation,
		},
		{
			name: "core_metric_before_supplier",
			ctx: fallbackRouteContext{
				q:             "2026年3月收入、成本、利润分别是多少？",
				from:          "2026-03",
				to:            "2026-03",
				hasRealEntity: false,
				cfg:           cfg,
			},
			want: fallbackRouteCoreMetric,
		},
		{
			name: "supplier_payments",
			ctx: fallbackRouteContext{
				q:   "2026年3月有多少家供应商发生付款？",
				cfg: cfg,
			},
			want: fallbackRouteSupplierPayments,
		},
		{
			name: "hr_breakdown",
			ctx: fallbackRouteContext{
				q:   "2026年3月人力成本多少？工资、社保、公积金分别是多少？",
				cfg: cfg,
			},
			want: fallbackRouteHRBreakdown,
		},
		{
			name: "monthly_expense",
			ctx: fallbackRouteContext{
				q:   "这个月整体支出多少",
				cfg: cfg,
			},
			want: fallbackRouteMonthlyExpense,
		},
		{
			name: "entity_readiness",
			ctx: fallbackRouteContext{
				q:      "南京林悦智能科技有限公司3月数据出来了吗？",
				entity: "南京林悦智能科技有限公司",
				cfg:    cfg,
			},
			want: fallbackRouteEntityReadiness,
		},
		{
			name: "project_income_cost",
			ctx: fallbackRouteContext{
				q:      "飞未云科项目支出多少？",
				entity: "飞未云科",
				cfg:    cfg,
			},
			want: fallbackRouteProjectIncomeCost,
		},
		{
			name: "counterparty_fallback",
			ctx: fallbackRouteContext{
				q:      "飞未云科3月回款多少？",
				entity: "飞未云科",
				cfg:    cfg,
			},
			want: fallbackRouteCounterpartyAmount,
		},
		{
			name: "none",
			ctx: fallbackRouteContext{
				q:   "你好",
				cfg: cfg,
			},
			want: fallbackRouteNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveFallbackRoute(tc.ctx); got != tc.want {
				t.Fatalf("resolveFallbackRoute(%+v) = %s, want %s", tc.ctx, got, tc.want)
			}
		})
	}
}
