package query

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"financeqa/internal/analysis"
)

func (e *Engine) rangeProfitCashBridge(ctx context.Context, from, to string) (*analysis.ProfitCashBridge, []string, []string) {
	periods, err := periodsBetween(from, to)
	if err != nil || len(periods) == 0 {
		return nil, nil, nil
	}

	total := &analysis.ProfitCashBridge{
		Company: e.Company,
		Period:  displayPeriod(from, to),
	}
	sqls := []string{
		"profit_cash_bridge(range): sum AnalyzeProfitCashBridgeWithDB over each month in selected range",
	}
	logs := make([]string, 0, len(periods)+1)
	for _, period := range periods {
		bridge, bridgeErr := analysis.AnalyzeProfitCashBridgeWithDB(ctx, e.db, e.Company, period)
		if bridgeErr != nil {
			logs = append(logs, fmt.Sprintf("[利润调现金桥] period=%s skipped=%v", period, bridgeErr))
			return nil, sqls, logs
		}
		addProfitCashBridge(total, bridge)
		logs = append(logs, fmt.Sprintf("[利润调现金桥] period=%s estimated_operating_cash=%.2f bank_net_cash=%.2f non_operating_delta=%.2f", period, bridge.EstimatedOperatingCash, bridge.BankNetCash, bridge.NonOperatingCashDelta))
	}
	total.Period = displayPeriod(from, to)
	logs = append(logs, fmt.Sprintf("[利润调现金桥-区间汇总] period=%s estimated_operating_cash=%.2f bank_net_cash=%.2f non_operating_delta=%.2f", total.Period, total.EstimatedOperatingCash, total.BankNetCash, total.NonOperatingCashDelta))
	return total, sqls, logs
}

func addProfitCashBridge(total *analysis.ProfitCashBridge, add analysis.ProfitCashBridge) {
	if total == nil {
		return
	}
	total.NetProfit = round2(total.NetProfit + add.NetProfit)
	total.Depreciation = round2(total.Depreciation + add.Depreciation)
	total.FixedAssetPurchasePrincipal = round2(total.FixedAssetPurchasePrincipal + add.FixedAssetPurchasePrincipal)
	total.ARIncrease = round2(total.ARIncrease + add.ARIncrease)
	total.PrepaymentIncrease = round2(total.PrepaymentIncrease + add.PrepaymentIncrease)
	total.OtherReceivableIncrease = round2(total.OtherReceivableIncrease + add.OtherReceivableIncrease)
	total.OtherPayableIncrease = round2(total.OtherPayableIncrease + add.OtherPayableIncrease)
	total.APIncrease = round2(total.APIncrease + add.APIncrease)
	total.AdvanceReceiptIncrease = round2(total.AdvanceReceiptIncrease + add.AdvanceReceiptIncrease)
	total.PayrollIncrease = round2(total.PayrollIncrease + add.PayrollIncrease)
	total.TaxBalanceIncrease = round2(total.TaxBalanceIncrease + add.TaxBalanceIncrease)
	total.TaxTimingAdjustment = round2(total.TaxTimingAdjustment + add.TaxTimingAdjustment)
	total.EstimatedOperatingCash = round2(total.EstimatedOperatingCash + add.EstimatedOperatingCash)
	total.AdjustedOperatingCashEstimate = round2(total.AdjustedOperatingCashEstimate + add.AdjustedOperatingCashEstimate)
	total.OperatingCashIn = round2(total.OperatingCashIn + add.OperatingCashIn)
	total.OperatingCashOut = round2(total.OperatingCashOut + add.OperatingCashOut)
	total.OperatingCashNet = round2(total.OperatingCashNet + add.OperatingCashNet)
	total.NonOperatingCashIn = round2(total.NonOperatingCashIn + add.NonOperatingCashIn)
	total.NonOperatingCashOut = round2(total.NonOperatingCashOut + add.NonOperatingCashOut)
	total.NonOperatingCashNet = round2(total.NonOperatingCashNet + add.NonOperatingCashNet)
	total.MixedCashIn = round2(total.MixedCashIn + add.MixedCashIn)
	total.MixedCashOut = round2(total.MixedCashOut + add.MixedCashOut)
	total.MixedCashNet = round2(total.MixedCashNet + add.MixedCashNet)
	total.BankNetCash = round2(total.BankNetCash + add.BankNetCash)
	total.ExcludedCashNet = round2(total.ExcludedCashNet + add.ExcludedCashNet)
	total.OperatingCashGap = round2(total.OperatingCashGap + add.OperatingCashGap)
	total.AdjustedOperatingCashGap = round2(total.AdjustedOperatingCashGap + add.AdjustedOperatingCashGap)
	total.BankCashGap = round2(total.BankCashGap + add.BankCashGap)
	total.AdjustedBankCashGap = round2(total.AdjustedBankCashGap + add.AdjustedBankCashGap)
	total.NonOperatingCashDelta = round2(total.NonOperatingCashDelta + add.NonOperatingCashDelta)
	total.DeltaSources = mergeBridgeDeltaSources(total.DeltaSources, add.DeltaSources)
}

func mergeBridgeDeltaSources(base, add map[string]string) map[string]string {
	if len(add) == 0 {
		return cloneStringMap(base)
	}
	merged := cloneStringMap(base)
	if merged == nil {
		merged = make(map[string]string, len(add))
	}
	for key, value := range add {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		existing := strings.TrimSpace(merged[key])
		if existing == "" {
			merged[key] = value
			continue
		}
		if existing == value {
			continue
		}
		seen := map[string]struct{}{}
		parts := strings.Split(existing+","+value, ",")
		clean := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			clean = append(clean, part)
		}
		sort.Strings(clean)
		merged[key] = strings.Join(clean, ",")
	}
	return merged
}
