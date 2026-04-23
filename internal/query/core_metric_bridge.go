package query

import "financeqa/internal/analysis"

func bridgeToMap(bridge *analysis.ProfitCashBridge) map[string]any {
	if bridge == nil {
		return nil
	}
	return map[string]any{
		"net_profit":                       bridge.NetProfit,
		"depreciation":                     bridge.Depreciation,
		"fixed_asset_purchase_principal":   bridge.FixedAssetPurchasePrincipal,
		"ar_increase":                      bridge.ARIncrease,
		"prepayment_increase":              bridge.PrepaymentIncrease,
		"other_receivable_increase":        bridge.OtherReceivableIncrease,
		"other_payable_increase":           bridge.OtherPayableIncrease,
		"ap_increase":                      bridge.APIncrease,
		"advance_receipt_increase":         bridge.AdvanceReceiptIncrease,
		"payroll_increase":                 bridge.PayrollIncrease,
		"tax_balance_increase":             bridge.TaxBalanceIncrease,
		"tax_timing_adjustment":            bridge.TaxTimingAdjustment,
		"estimated_operating_cash":         bridge.EstimatedOperatingCash,
		"adjusted_operating_cash_estimate": bridge.AdjustedOperatingCashEstimate,
		"operating_cash_in":                bridge.OperatingCashIn,
		"operating_cash_out":               bridge.OperatingCashOut,
		"operating_cash_net":               bridge.OperatingCashNet,
		"non_operating_cash_in":            bridge.NonOperatingCashIn,
		"non_operating_cash_out":           bridge.NonOperatingCashOut,
		"non_operating_cash_net":           bridge.NonOperatingCashNet,
		"mixed_cash_in":                    bridge.MixedCashIn,
		"mixed_cash_out":                   bridge.MixedCashOut,
		"mixed_cash_net":                   bridge.MixedCashNet,
		"bank_net_cash":                    bridge.BankNetCash,
		"excluded_cash_net":                bridge.ExcludedCashNet,
		"operating_cash_gap":               bridge.OperatingCashGap,
		"adjusted_operating_cash_gap":      bridge.AdjustedOperatingCashGap,
		"non_operating_cash_delta":         bridge.NonOperatingCashDelta,
	}
}

func bridgeEstimatedCash(bridge *analysis.ProfitCashBridge) float64 {
	if bridge == nil {
		return 0
	}
	return bridge.EstimatedOperatingCash
}

func bridgeAdjustedEstimatedCash(bridge *analysis.ProfitCashBridge) float64 {
	if bridge == nil {
		return 0
	}
	return bridge.AdjustedOperatingCashEstimate
}

func bridgeNonOperatingDelta(bridge *analysis.ProfitCashBridge) float64 {
	if bridge == nil {
		return 0
	}
	return bridge.NonOperatingCashDelta
}
