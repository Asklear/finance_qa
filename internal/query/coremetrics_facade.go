package query

import (
	"financeqa/internal/analysis"
	querycoremetrics "financeqa/internal/query/coremetrics"
)

func bridgeToMap(bridge *analysis.ProfitCashBridge) map[string]any {
	return querycoremetrics.BridgeToMap(bridge)
}

func cloneStringMap(in map[string]string) map[string]string {
	return querycoremetrics.CloneStringMap(in)
}

func bridgeEstimatedCash(bridge *analysis.ProfitCashBridge) float64 {
	return querycoremetrics.BridgeEstimatedCash(bridge)
}

func bridgeAdjustedEstimatedCash(bridge *analysis.ProfitCashBridge) float64 {
	return querycoremetrics.BridgeAdjustedEstimatedCash(bridge)
}

func bridgeNonOperatingDelta(bridge *analysis.ProfitCashBridge) float64 {
	return querycoremetrics.BridgeNonOperatingDelta(bridge)
}
