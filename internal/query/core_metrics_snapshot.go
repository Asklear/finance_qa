package query

import (
	"fmt"

	"financeqa/internal/accounting"
)

type coreMetricDualSnapshot struct {
	Message          string
	Data             map[string]any
	Metric           string
	RequestedMetrics []string
	CashValue        float64
	AccrualValue     float64
}

func buildCoreMetricDualSnapshot(question string, spec QuerySpec, coverage coreMetricCoverage, unified *unifiedCoreMetrics) coreMetricDualSnapshot {
	dualCash := unified.Cash
	dualAccrual := unified.Accrual
	periodLabel := unified.Period
	requestedPeriodLabel := displayPeriod(spec.PeriodFrom, spec.PeriodTo)

	request := resolveCoreMetricRequest(question, "核心指标")
	requestedMetrics := request.RequestedMetrics
	primaryMetric := request.PrimaryMetric
	metric := request.MetricLabel
	explicitNetProfit := request.ExplicitNetProfit
	cashView := accounting.CashPerspective{
		Description: "现金口径",
		Income:      dualCash.Income,
		Expense:     dualCash.Expense,
		Net:         dualCash.Net,
	}
	accrualView := accounting.AccrualPerspective{
		Description: "经营口径",
		Revenue:     dualAccrual.Revenue,
		TotalCost:   dualAccrual.TotalCost,
		Profit:      dualAccrual.Profit,
	}
	cashValue, accrualValue := pickMetricValue(metric, &accounting.DualPerspective{
		Cash:    cashView,
		Accrual: accrualView,
	})
	msg := buildBossDualPerspectiveMessage(periodLabel, dualCash, dualAccrual, unified.Bridge)
	if len(requestedMetrics) == 1 {
		cashValue, accrualValue = pickMetricValue(primaryMetric, &accounting.DualPerspective{
			Cash:    cashView,
			Accrual: accrualView,
		})
		if explicitNetProfit {
			accrualValue = dualAccrual.NetProfit
		}
		msg = fmt.Sprintf("%s\n补充你当前关注的指标：%s - 现金口径 %.2f 元，经营口径 %.2f 元。", msg, primaryMetric, cashValue, accrualValue)
	}
	displayedBookProfit := dualAccrual.Profit
	if explicitNetProfit {
		displayedBookProfit = dualAccrual.NetProfit
	}
	if coverage.Truncated {
		msg = fmt.Sprintf("你问的是 %s，但当前账务数据仅到 %s，以下先按 %s 已出账数据回答。\n%s", requestedPeriodLabel, coverage.AvailableTo, periodLabel, msg)
	}

	summaryPayload := buildCoreMetricSummaryPayload(spec.PeriodFrom, spec.PeriodTo, unified.AccrualFrom, dualAccrual)

	data := map[string]any{
		"period":           periodLabel,
		"requested_period": requestedPeriodLabel,
		"metric":           metric,
		"money_view":       cashView,
		"account_view":     accrualView,
		"money_value":      cashValue,
		"account_value":    accrualValue,
		"total":            accrualValue,
		"data_ready":       true,
		"source_tables":    sourceTablesForCoreMetric(unified.AccrualFrom, true),
		"coverage": map[string]any{
			"requested_from": spec.PeriodFrom,
			"requested_to":   spec.PeriodTo,
			"actual_from":    coverage.ActualFrom,
			"actual_to":      coverage.ActualTo,
			"available_to":   coverage.AvailableTo,
			"truncated":      coverage.Truncated,
			"data_ready":     true,
		},
		"requested_metrics": requestedMetrics,
		"query_spec_overrides": map[string]any{
			"semantic_families": []string{"profit_statement", "financial_statement"},
		},
		"一致性守卫":              unified.Guard,
		"range_validation":   unified.AccrualValidation,
		"profit_cash_bridge": bridgeToMap(unified.Bridge),
		"metrics":            buildCoreMetricMetricsMap(dualAccrual),
		"monthly":            cloneMap(summaryPayload),
		"range_summary":      cloneMap(summaryPayload),
		"现金流入":               dualCash.Income,
		"现金流出":               dualCash.Expense,
		"净现金流":               dualCash.Net,
		"cash_flow":          buildCoreMetricCashFlowSummary(&dualCash),
		"财务做账口径(看利润)":        buildCoreMetricBookView(dualAccrual, displayedBookProfit),
		"difference_bridge": map[string]any{
			"利润与现金净额差":     round2(dualAccrual.Profit - dualCash.Net),
			"收入确认回款时间差":    round2(dualAccrual.Revenue - dualCash.Income),
			"成本付款确认时间差":    round2(dualCash.Expense - dualAccrual.TotalCost),
			"其他调节项":        round2((dualAccrual.Profit - dualCash.Net) - (dualAccrual.Revenue - dualCash.Income) - (dualCash.Expense - dualAccrual.TotalCost)),
			"经营现金净额估算":     bridgeEstimatedCash(unified.Bridge),
			"含税项调节后经营现金估算": bridgeAdjustedEstimatedCash(unified.Bridge),
			"非经营现金差额":      bridgeNonOperatingDelta(unified.Bridge),
		},
		"dual_perspective": map[string]any{
			"cash": map[string]any{
				"说明":   "现金口径",
				"现金流入": dualCash.Income,
				"现金流出": dualCash.Expense,
				"净现金流": dualCash.Net,
			},
			"accrual": map[string]any{
				"说明":      "经营口径",
				"营业收入":    dualAccrual.Revenue,
				"营业成本及费用": dualAccrual.TotalCost,
				"营业外收入":   dualAccrual.NonOperatingIncome,
				"营业外支出":   dualAccrual.NonOperatingExpense,
				"账面利润":    displayedBookProfit,
				"净利润":     dualAccrual.NetProfit,
			},
		},
	}

	return coreMetricDualSnapshot{
		Message:          msg,
		Data:             data,
		Metric:           metric,
		RequestedMetrics: requestedMetrics,
		CashValue:        cashValue,
		AccrualValue:     accrualValue,
	}
}
