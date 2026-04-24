package query

import (
	"fmt"
	"strings"
)

func buildContractAggregateResultMessage(selection contractAggregateSelection, summary contractAggregateSummary) string {
	message := fmt.Sprintf("%s：%s。",
		buildContractAggregateScopeLabel(summary),
		strings.Join(buildContractAggregateMetricParts(selection, summary), "，"))
	if supplement := buildContractAggregateSupplement(selection, summary); supplement != "" {
		message += supplement
	}
	return message
}

func buildContractAggregateMetricParts(selection contractAggregateSelection, summary contractAggregateSummary) []string {
	parts := make([]string, 0, 3)
	if selection.IncludeRevenue {
		parts = append(parts, fmt.Sprintf("营收 %.2f 元", summary.RevenueSettlement))
	}
	if selection.IncludeCost {
		parts = append(parts, fmt.Sprintf("合同成本 %.2f 元", summary.CostSettlement))
	}
	if selection.IncludeProfit {
		parts = append(parts, fmt.Sprintf("利润 %.2f 元", summary.Profit))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("营收 %.2f 元", summary.RevenueSettlement))
	}
	return parts
}

func buildContractAggregateSupplement(selection contractAggregateSelection, summary contractAggregateSummary) string {
	switch {
	case selection.IncludeProfit && !selection.IncludeRevenue && !selection.IncludeCost:
		return fmt.Sprintf("补充合同现金净额 %.2f 元（回款 %.2f 元，付款 %.2f 元）。",
			round2(summary.RevenueReceived-summary.CostPaid),
			round2(summary.RevenueReceived),
			round2(summary.CostPaid))
	case selection.IncludeCost && !selection.IncludeRevenue && !selection.IncludeProfit:
		return fmt.Sprintf("补充合同现金付款 %.2f 元。", round2(summary.CostPaid))
	case selection.IncludeRevenue && !selection.IncludeCost && !selection.IncludeProfit:
		return fmt.Sprintf("补充合同现金到账 %.2f 元，已开票 %.2f 元。", round2(summary.RevenueReceived), round2(summary.RevenueInvoiced))
	default:
		parts := make([]string, 0, 4)
		if selection.IncludeRevenue {
			parts = append(parts, fmt.Sprintf("合同现金回款 %.2f 元", round2(summary.RevenueReceived)))
			parts = append(parts, fmt.Sprintf("已开票 %.2f 元", round2(summary.RevenueInvoiced)))
		}
		if selection.IncludeCost {
			parts = append(parts, fmt.Sprintf("合同现金付款 %.2f 元", round2(summary.CostPaid)))
		}
		if selection.IncludeProfit || (selection.IncludeRevenue && selection.IncludeCost) {
			parts = append(parts, fmt.Sprintf("净现金 %.2f 元", round2(summary.RevenueReceived-summary.CostPaid)))
		}
		if len(parts) == 0 {
			return ""
		}
		return "补充" + strings.Join(parts, "，") + "。"
	}
}
