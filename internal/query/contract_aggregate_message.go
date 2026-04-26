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
	if selection.IncludeReceivable {
		parts = append(parts, fmt.Sprintf("合同应收 %.2f 元", summary.RevenueReceivable))
	}
	if selection.IncludePayable {
		parts = append(parts, fmt.Sprintf("合同应付 %.2f 元", summary.CostPayable))
	}
	if selection.IncludeInvoiceAR {
		parts = append(parts, fmt.Sprintf("已开票未回款 %.2f 元", summary.RevenueInvoiceOpen))
	}
	if selection.IncludeInvoiceAP {
		parts = append(parts, fmt.Sprintf("已收票未付款 %.2f 元", summary.CostInvoiceOpen))
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
	case selection.IncludeReceivable && !selection.IncludePayable:
		return fmt.Sprintf("补充合同结算 %.2f 元、已到账 %.2f 元；其中已开票未回款 %.2f 元。", round2(summary.RevenueSettlement), round2(summary.RevenueReceived), round2(summary.RevenueInvoiceOpen))
	case selection.IncludePayable && !selection.IncludeReceivable:
		return fmt.Sprintf("补充合同成本 %.2f 元、已付款 %.2f 元；其中已收票未付款 %.2f 元。", round2(summary.CostSettlement), round2(summary.CostPaid), round2(summary.CostInvoiceOpen))
	case selection.IncludeInvoiceAR && !selection.IncludeInvoiceAP:
		detail := buildRevenueInvoiceOpenDetailSentence(summary.RevenueInvoiceOpenItems)
		return fmt.Sprintf("补充已开票 %.2f 元、已到账 %.2f 元。%s", round2(summary.RevenueInvoiced), round2(summary.RevenueReceived), detail)
	case selection.IncludeInvoiceAP && !selection.IncludeInvoiceAR:
		return fmt.Sprintf("补充已收票 %.2f 元、已付款 %.2f 元。", round2(summary.CostInvoiced), round2(summary.CostPaid))
	default:
		parts := make([]string, 0, 4)
		if selection.IncludeRevenue {
			parts = append(parts, fmt.Sprintf("合同现金回款 %.2f 元", round2(summary.RevenueReceived)))
			parts = append(parts, fmt.Sprintf("已开票 %.2f 元", round2(summary.RevenueInvoiced)))
		}
		if selection.IncludeCost {
			parts = append(parts, fmt.Sprintf("合同现金付款 %.2f 元", round2(summary.CostPaid)))
		}
		if selection.IncludeReceivable {
			parts = append(parts, fmt.Sprintf("已到账 %.2f 元", round2(summary.RevenueReceived)))
			parts = append(parts, fmt.Sprintf("已开票未回款 %.2f 元", round2(summary.RevenueInvoiceOpen)))
		}
		if selection.IncludePayable {
			parts = append(parts, fmt.Sprintf("已付款 %.2f 元", round2(summary.CostPaid)))
			parts = append(parts, fmt.Sprintf("已收票未付款 %.2f 元", round2(summary.CostInvoiceOpen)))
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

func buildRevenueInvoiceOpenDetailSentence(items []contractAggregateOpenItem) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for i, item := range items {
		if i >= 3 {
			break
		}
		label := strings.TrimSpace(item.CustomerName)
		content := strings.TrimSpace(item.ContractContent)
		if content != "" {
			if label != "" {
				label += "-"
			}
			label += content
		}
		if label == "" {
			label = "未命名项目"
		}
		parts = append(parts, fmt.Sprintf("%s 已开票 %.2f 元、已回款 %.2f 元、未回款 %.2f 元", label, round2(item.InvoiceAmount), round2(item.ReceivedAmount), round2(item.OpenAmount)))
	}
	suffix := ""
	if len(items) > 3 {
		suffix = fmt.Sprintf("等 %d 个项目", len(items))
	}
	return "明细：" + strings.Join(parts, "；") + suffix + "。"
}
