package query

import (
	"context"
	"fmt"
	"strings"

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
)

type unifiedCoreMetrics struct {
	Period            string
	Cash              accounting.CashPerspective
	Accrual           monthlyBookView
	AccrualFrom       string
	AccrualValidation map[string]any
	Bridge            *analysis.ProfitCashBridge
	Guard             map[string]any
}

type incomeStatementMetricMatcher struct {
	key      string
	patterns []string
}

func cloneUnifiedCoreMetrics(in *unifiedCoreMetrics) *unifiedCoreMetrics {
	if in == nil {
		return nil
	}
	out := *in
	out.AccrualValidation = cloneMap(in.AccrualValidation)
	out.Guard = cloneMap(in.Guard)
	if in.Bridge != nil {
		bridgeCopy := *in.Bridge
		out.Bridge = &bridgeCopy
	}
	return &out
}

func (e *Engine) unifiedCoreMetricCacheKey(from, to string) string {
	return strings.TrimSpace(e.Company) + "|" + strings.TrimSpace(from) + "|" + strings.TrimSpace(to)
}

func (e *Engine) getUnifiedCoreMetricsCached(from, to string) (*unifiedCoreMetrics, []string, []string, error) {
	cacheKey := e.unifiedCoreMetricCacheKey(from, to)
	e.cacheMu.RLock()
	if cached, ok := e.coreMetricCache[cacheKey]; ok && cached.metrics != nil {
		e.cacheMu.RUnlock()
		sqls := append([]string{}, cached.sqls...)
		logs := append([]string{}, cached.logs...)
		logs = append(logs, fmt.Sprintf("[缓存] unified_core_metrics cache_hit key=%s", cacheKey))
		return cloneUnifiedCoreMetrics(cached.metrics), sqls, logs, nil
	}
	e.cacheMu.RUnlock()

	unified, sqls, logs, err := e.computeUnifiedCoreMetrics(from, to)
	if err != nil {
		return nil, nil, nil, err
	}
	e.cacheMu.Lock()
	e.coreMetricCache[cacheKey] = cachedUnifiedCoreMetrics{
		metrics: cloneUnifiedCoreMetrics(unified),
		sqls:    append([]string{}, sqls...),
		logs:    append([]string{}, logs...),
	}
	e.cacheMu.Unlock()
	return unified, sqls, logs, nil
}

func (e *Engine) computeUnifiedCoreMetrics(from, to string) (*unifiedCoreMetrics, []string, []string, error) {
	cash, err := e.calc.ComputeCashFlow(e.Company, from, to)
	if err != nil {
		return nil, nil, nil, err
	}

	book, source, validation, extraSQLs, extraLogs, err := e.bookSummaryForRange(from, to)
	if err != nil {
		return nil, nil, nil, err
	}

	rawAccrual := accounting.AccrualPerspective{
		Description: "经营口径",
		Revenue:     book.Revenue,
		TotalCost:   book.TotalCost,
		Profit:      book.Profit,
	}

	guard := buildConsistencyGuard(rawAccrual, book, *cash, source, validation)
	sqls := append([]string{}, e.calc.ExecutedSQLs...)
	sqls = appendUniqueStrings(sqls, extraSQLs...)
	logs := append([]string{}, e.calc.CalculationLogs...)
	logs = append(logs, extraLogs...)
	logs = append(logs, fmt.Sprintf("[统一口径] accrual_source=%s cash_source=bank_statement", source))
	var bridge *analysis.ProfitCashBridge
	if from == to {
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
	} else {
		logs = append(logs, fmt.Sprintf("[利润调现金桥] skipped: multi-period aggregation %s~%s", from, to))
	}
	if passed, _ := guard["passed"].(bool); !passed {
		logs = append(logs, "[一致性守卫] 发现跨口径漂移，已强制采用标准口径输出")
	}

	return &unifiedCoreMetrics{
		Period:            displayPeriod(from, to),
		Cash:              *cash,
		Accrual:           book,
		AccrualFrom:       source,
		AccrualValidation: validation,
		Bridge:            bridge,
		Guard:             guard,
	}, sqls, logs, nil
}
