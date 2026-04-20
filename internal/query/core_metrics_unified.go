package query

import (
	"context"
	"fmt"
	"math"
	"strings"

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
)

type unifiedCoreMetrics struct {
	Period      string
	Cash        accounting.CashPerspective
	Accrual     monthlyBookView
	AccrualFrom string
	Bridge      *analysis.ProfitCashBridge
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
	var bridge *analysis.ProfitCashBridge
	if cashBridge, bridgeErr := analysis.AnalyzeProfitCashBridgeWithDB(context.Background(), e.db, e.Company, to); bridgeErr == nil {
		bridge = &cashBridge
		logs = append(logs, fmt.Sprintf("[利润调现金桥] profit=%.2f depreciation=%.2f ar=%.2f prepayment=%.2f other_receivable=%.2f other_payable=%.2f ap=%.2f advance_receipt=%.2f payroll=%.2f tax_balance=%.2f tax_timing=%.2f estimated_operating_cash=%.2f adjusted_operating_cash=%.2f bank_net_cash=%.2f non_operating_delta=%.2f",
			bridge.NetProfit, bridge.Depreciation, bridge.ARIncrease, bridge.PrepaymentIncrease, bridge.OtherReceivableIncrease, bridge.OtherPayableIncrease, bridge.APIncrease, bridge.AdvanceReceiptIncrease, bridge.PayrollIncrease, bridge.TaxBalanceIncrease, bridge.TaxTimingAdjustment, bridge.EstimatedOperatingCash, bridge.AdjustedOperatingCashEstimate, bridge.BankNetCash, bridge.NonOperatingCashDelta))
		sqls = appendUniqueStrings(sqls,
			"profit_cash_bridge(balance_detail): SELECT closing_debit, closing_credit FROM balance_detail WHERE ... AND period IN (?, previous_period) AND account_code IN ('1602','1122','1123','1221','2202','2203','2211','2221','2241','22210101','22210106')",
			"profit_cash_bridge(income_statement): SELECT current_amount FROM income_statement WHERE ... AND period = ? AND item_name LIKE '%净利润%'",
		)
	} else {
		logs = append(logs, fmt.Sprintf("[利润调现金桥] skipped: %v", bridgeErr))
	}
	if passed, _ := guard["passed"].(bool); !passed {
		logs = append(logs, "[一致性守卫] 发现跨口径漂移，已强制采用标准口径输出")
	}

	return &unifiedCoreMetrics{
		Period:      to,
		Cash:        dual.Cash,
		Accrual:     book,
		AccrualFrom: source,
		Bridge:      bridge,
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

func buildBossDualPerspectiveMessage(period string, cash accounting.CashPerspective, accrual monthlyBookView, bridge *analysis.ProfitCashBridge) string {
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
	}
	if bridge != nil {
		gapLabel := fmt.Sprintf("含税项调节后的利润桥和过滤后经营现金仍有 %.2f 元差额待继续拆分。", math.Abs(bridge.AdjustedOperatingCashGap))
		if bridge.AdjustedOperatingCashGap < 0 {
			gapLabel = fmt.Sprintf("含税项调节后的利润桥比过滤后的经营现金高 %.2f 元，说明还有营运资金或分类口径没补齐。", math.Abs(bridge.AdjustedOperatingCashGap))
		} else if bridge.AdjustedOperatingCashGap > 0 {
			gapLabel = fmt.Sprintf("过滤后的经营现金比含税项调节后的利润桥高 %.2f 元，说明还有现金分类或桥接项待核实。", math.Abs(bridge.AdjustedOperatingCashGap))
		}
		lines = append(lines,
			fmt.Sprintf("按利润调现金桥还原：净利润 %.2f + 折旧 %.2f - 应收净增加 %.2f - 预付净增加 %.2f - 其他应付款净增加 %.2f + 应付账款净增加 %.2f + 预收账款净增加 %.2f + 应付职工薪酬净增加 %.2f = 经营现金 %.2f 元。",
				bridge.NetProfit, bridge.Depreciation, bridge.ARIncrease, bridge.PrepaymentIncrease, bridge.OtherPayableIncrease, bridge.APIncrease, bridge.AdvanceReceiptIncrease, bridge.PayrollIncrease, bridge.EstimatedOperatingCash),
			fmt.Sprintf("再加税项时点调节 %.2f 元后，经营现金估算 %.2f 元。", bridge.TaxTimingAdjustment, bridge.AdjustedOperatingCashEstimate),
			fmt.Sprintf("按凭证同组科目过滤后，经营性现金净额 %.2f 元；已识别的非经营/混合现金净额 %.2f 元。", bridge.OperatingCashNet, bridge.ExcludedCashNet),
			gapLabel,
		)
		if bridge.OtherReceivableIncrease != 0 || bridge.TaxBalanceIncrease != 0 {
			lines = append(lines, fmt.Sprintf("补充披露：其他应收款净增加 %.2f 元、应交税费净变动 %.2f 元，这两项先单独列示，不直接并入经营现金桥，避免把内部往来或税项时差误当经营现金。", bridge.OtherReceivableIncrease, bridge.TaxBalanceIncrease))
		}
	}
	lines = append(lines, "建议动作：先盯未回款客户和大额支出项目，周内做一次回款与结算单对齐。")
	return strings.Join(lines, "\n")
}
