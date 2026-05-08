package query

import "strings"

type fallbackRoute string

const (
	fallbackRouteNone               fallbackRoute = "none"
	fallbackRouteReconciliation     fallbackRoute = "reconciliation"
	fallbackRouteCoreMetric         fallbackRoute = "core_metric"
	fallbackRouteSupplierPayments   fallbackRoute = "supplier_payments"
	fallbackRouteHRBreakdown        fallbackRoute = "hr_breakdown"
	fallbackRouteMonthlyExpense     fallbackRoute = "monthly_expense"
	fallbackRouteEntityReadiness    fallbackRoute = "entity_readiness"
	fallbackRouteProjectIncomeCost  fallbackRoute = "project_income_cost"
	fallbackRouteCounterpartyAmount fallbackRoute = "counterparty_amount"
)

type fallbackRouteContext struct {
	q             string
	entity        string
	hasRealEntity bool
	from          string
	to            string
	cfg           RuleConfig
}

func resolveFallbackRoute(ctx fallbackRouteContext) fallbackRoute {
	switch {
	case shouldUseReconciliation(ctx.q):
		return fallbackRouteReconciliation
	case isIntervalCoreMetricQuestionWithConfig(ctx.q, ctx.entity, ctx.hasRealEntity, ctx.from, ctx.to, ctx.cfg) ||
		shouldPreferCoreMetricSummaryWithConfig(ctx.q, ctx.entity, ctx.hasRealEntity, ctx.from, ctx.to, ctx.cfg):
		return fallbackRouteCoreMetric
	case isSupplierPaymentsFallbackQuestion(ctx.q):
		return fallbackRouteSupplierPayments
	case containsAny(ctx.q, ctx.cfg.intentKeywordGroup(routerGroupHRCost)):
		return fallbackRouteHRBreakdown
	case containsAny(ctx.q, ctx.cfg.FallbackMonthlyExpenseKeywords):
		return fallbackRouteMonthlyExpense
	case strings.TrimSpace(ctx.entity) != "" && strings.Contains(ctx.q, "数据出来"):
		return fallbackRouteEntityReadiness
	case strings.TrimSpace(ctx.entity) != "" && strings.Contains(ctx.q, "项目") && containsAny(ctx.q, []string{"收入", "成本", "支出"}):
		return fallbackRouteProjectIncomeCost
	case strings.TrimSpace(ctx.entity) != "":
		return fallbackRouteCounterpartyAmount
	default:
		return fallbackRouteNone
	}
}

func isSupplierPaymentsFallbackQuestion(q string) bool {
	return strings.Contains(q, "供应商") && strings.Contains(q, "多少")
}
