package query

import (
	"fmt"
	"strings"

	"financeqa/internal/analysis"
)

type profitCashBridgeComponent struct {
	label string
	value float64
}

func buildProfitCashBridgeNarrative(bridge *analysis.ProfitCashBridge, totalLabel string) string {
	if bridge == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("净利润 %.2f", bridge.NetProfit)}
	for _, component := range profitCashBridgeComponents(bridge) {
		parts = append(parts, fmt.Sprintf("%s %s", component.label, formatSignedMoney(component.value)))
	}
	parts = append(parts, fmt.Sprintf("%s %.2f 元", strings.TrimSpace(totalLabel), bridge.EstimatedOperatingCash))
	return "按利润调现金桥拆：" + strings.Join(parts, "，") + "。"
}

func profitCashBridgeComponents(bridge *analysis.ProfitCashBridge) []profitCashBridgeComponent {
	return []profitCashBridgeComponent{
		{label: "折旧", value: bridge.Depreciation},
		{label: "应收账款变动", value: -bridge.ARIncrease},
		{label: "预付账款变动", value: -bridge.PrepaymentIncrease},
		{label: "其他应收款变动", value: -bridge.OtherReceivableIncrease},
		{label: "应付账款变动", value: bridge.APIncrease},
		{label: "预收账款变动", value: bridge.AdvanceReceiptIncrease},
		{label: "应付职工薪酬变动", value: bridge.PayrollIncrease},
		{label: "应交税费变动", value: bridge.TaxBalanceIncrease},
		{label: "固定资产购置", value: -bridge.FixedAssetPurchasePrincipal},
	}
}

func formatSignedMoney(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("+%.2f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

func reconciliationSupportingPeriodLabel(period string) string {
	if strings.Contains(period, "~") {
		return "期间"
	}
	return strings.TrimSpace(period)
}
