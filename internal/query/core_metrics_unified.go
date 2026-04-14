package query

import (
	"fmt"
	"math"
	"strings"

	"financeqa/internal/accounting"
)

type unifiedCoreMetrics struct {
	Period      string
	Cash        accounting.CashPerspective
	Accrual     monthlyBookView
	AccrualFrom string
	Guard       map[string]any
}

func (e *Engine) computeUnifiedCoreMetrics(from, to string, year, month int) (*unifiedCoreMetrics, []string, []string, error) {
	_ = from // cash period由 year/month 推导，保留参数用于调用侧一致性
	dual, err := e.calc.ComputeDualPerspective(e.Company, year, month)
	if err != nil {
		return nil, nil, nil, err
	}
	rawAccrual := dual.Accrual

	book, source, err := e.monthlyBookSummary(year, month)
	if err != nil {
		return nil, nil, nil, err
	}

	guard := buildConsistencyGuard(rawAccrual, book, dual.Cash)
	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	logs := append([]string{}, e.calc.CalculationLogs...)
	logs = append(logs, fmt.Sprintf("[统一口径] accrual_source=%s cash_source=bank_statement", source))
	if passed, _ := guard["passed"].(bool); !passed {
		logs = append(logs, "[一致性守卫] 发现跨口径漂移，已强制采用标准口径输出")
	}

	return &unifiedCoreMetrics{
		Period:      to,
		Cash:        dual.Cash,
		Accrual:     book,
		AccrualFrom: source,
		Guard:       guard,
	}, sqls, logs, nil
}

func buildConsistencyGuard(raw accounting.AccrualPerspective, selected monthlyBookView, cash accounting.CashPerspective) map[string]any {
	revenueDelta := round2(selected.Revenue - raw.Revenue)
	costDelta := round2(selected.TotalCost - raw.TotalCost)
	profitDelta := round2(selected.Profit - raw.Profit)
	cashIdentityDelta := round2((cash.Income - cash.Expense) - cash.Net)
	accrualIdentityDelta := round2((selected.Revenue - selected.TotalCost) - selected.Profit)

	// 收入口径可能存在营业外收支/税费调整，允许 1 元误差。
	passed := math.Abs(cashIdentityDelta) <= 0.02 &&
		math.Abs(accrualIdentityDelta) <= 1.00

	return map[string]any{
		"passed":                 passed,
		"selected_accrual":       "monthly_book_summary",
		"cash_identity_delta":    cashIdentityDelta,
		"accrual_identity_delta": accrualIdentityDelta,
		"source_drift": map[string]any{
			"revenue_delta": revenueDelta,
			"cost_delta":    costDelta,
			"profit_delta":  profitDelta,
		},
	}
}

func buildBossDualPerspectiveMessage(period string, cash accounting.CashPerspective, accrual monthlyBookView) string {
	profitGap := round2(accrual.Profit - cash.Net)
	revenueTiming := round2(accrual.Revenue - cash.Income)
	costTiming := round2(cash.Expense - accrual.TotalCost)
	otherAdjustments := round2(profitGap - revenueTiming - costTiming)

	lines := []string{
		fmt.Sprintf("先说结论：%s 账上看利润 %.2f 元，银行卡上净增减 %.2f 元，两边相差 %.2f 元。", period, accrual.Profit, cash.Net, profitGap),
		fmt.Sprintf("银行卡上看：实际到账 %.2f 元，实际支出 %.2f 元，净增加 %.2f 元。", cash.Income, cash.Expense, cash.Net),
		fmt.Sprintf("账上看：确认收入 %.2f 元，确认成本及费用 %.2f 元，账面利润 %.2f 元。", accrual.Revenue, accrual.TotalCost, accrual.Profit),
		"差异最大的3个原因：",
		fmt.Sprintf("1. 收入确认和回款时间差 %.2f 元（账上收入减去实际到账）。", revenueTiming),
		fmt.Sprintf("2. 付款和成本确认时间差 %.2f 元（实际支出减去账上成本及费用）。", costTiming),
		fmt.Sprintf("3. 其他调节项 %.2f 元（含税费/营业外收支/四舍五入等）。", otherAdjustments),
		"建议动作：先盯未回款客户和大额支出项目，周内做一次回款与结算单对齐。",
	}
	return strings.Join(lines, "\n")
}
