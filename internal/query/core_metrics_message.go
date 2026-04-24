package query

import (
	"fmt"
	"math"
	"strings"

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
)

func buildConsistencyGuard(raw accounting.AccrualPerspective, selected monthlyBookView, cash accounting.CashPerspective, selectedLabel string, validation map[string]any) map[string]any {
	revenueDelta := round2(selected.Revenue - raw.Revenue)
	costDelta := round2(selected.TotalCost - raw.TotalCost)
	profitDelta := round2(selected.Profit - raw.Profit)
	cashIdentityDelta := round2((cash.Income - cash.Expense) - cash.Net)
	accrualIdentityDelta := round2((selected.Revenue - selected.TotalCost) - selected.Profit)

	// 收入口径可能存在营业外收支/税费调整，允许 1 元误差。
	passed := math.Abs(cashIdentityDelta) <= 0.02 &&
		math.Abs(accrualIdentityDelta) <= 1.00
	if validationPassed, ok := validation["passed"].(bool); ok {
		passed = passed && validationPassed
	}

	guard := map[string]any{
		"passed":                 passed,
		"selected_accrual":       selectedLabel,
		"cash_identity_delta":    cashIdentityDelta,
		"accrual_identity_delta": accrualIdentityDelta,
		"source_drift": map[string]any{
			"revenue_delta": revenueDelta,
			"cost_delta":    costDelta,
			"profit_delta":  profitDelta,
		},
	}
	if validation != nil {
		guard["range_validation"] = validation
	}
	return guard
}

func buildBossDualPerspectiveMessage(period string, cash accounting.CashPerspective, accrual monthlyBookView, bridge *analysis.ProfitCashBridge) string {
	profitGap := round2(accrual.Profit - cash.Net)
	revenueTiming := round2(accrual.Revenue - cash.Income)
	costTiming := round2(cash.Expense - accrual.TotalCost)
	otherAdjustments := round2(profitGap - revenueTiming - costTiming)

	lines := []string{
		fmt.Sprintf("先说现金口径：%s 实际到账 %.2f 元，实际支出 %.2f 元，净增加 %.2f 元。", period, cash.Income, cash.Expense, cash.Net),
		fmt.Sprintf("再补经营口径：确认收入 %.2f 元，确认成本及费用 %.2f 元，利润 %.2f 元（含营业外收入 %.2f 元、营业外支出 %.2f 元）。", accrual.Revenue, accrual.TotalCost, accrual.Profit, accrual.NonOperatingIncome, accrual.NonOperatingExpense),
		fmt.Sprintf("两个口径之间，利润和净现金流相差 %.2f 元。", profitGap),
		"差异最大的3个原因：",
		fmt.Sprintf("1. 收入确认和回款时间差 %.2f 元（账上收入减去实际到账）。", revenueTiming),
		fmt.Sprintf("2. 付款和成本确认时间差 %.2f 元（实际支出减去账上成本及费用）。", costTiming),
		fmt.Sprintf("3. 其他调节项 %.2f 元（含税费/营业外收支/四舍五入等）。", otherAdjustments),
	}
	if bridge != nil {
		lines = append(lines,
			buildProfitCashBridgeNarrative(bridge, "合计净现金流估算"),
			fmt.Sprintf("按凭证同组科目过滤后，经营性现金净额 %.2f 元；已识别的非经营/混合现金净额 %.2f 元。", bridge.OperatingCashNet, bridge.ExcludedCashNet),
		)
		if bridge.TaxTimingAdjustment != 0 {
			lines = append(lines, fmt.Sprintf("另有税项时点调节 %.2f 元，作为复核项单独保留，不再直接并入净现金流桥。", bridge.TaxTimingAdjustment))
		}
	}
	lines = append(lines, "建议动作：先盯未回款客户和大额支出项目，周内做一次回款与结算单对齐。")
	return strings.Join(lines, "\n")
}
