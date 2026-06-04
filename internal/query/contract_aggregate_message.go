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
	if marginNote := buildContractAggregateMarginNote(selection, summary); marginNote != "" {
		message += marginNote
	}
	if comparisonNote := buildContractAggregateComparisonNote(summary); comparisonNote != "" {
		message += comparisonNote
	}
	return message
}

func buildContractAggregateMetricParts(selection contractAggregateSelection, summary contractAggregateSummary) []string {
	parts := make([]string, 0, 3)
	if selection.IncludeRevenue {
		parts = append(parts, fmt.Sprintf("营收 %.2f 元", summary.RevenueSettlement))
	}
	if selection.IncludeCost {
		parts = append(parts, fmt.Sprintf("项目成本 %.2f 元", summary.CostSettlement))
	}
	if selection.IncludeProfit {
		parts = append(parts, fmt.Sprintf("利润 %.2f 元", summary.Profit))
	}
	if selection.IncludeReceivable {
		parts = append(parts, fmt.Sprintf("项目应收 %.2f 元", summary.RevenueReceivable))
	}
	if selection.IncludePayable {
		parts = append(parts, fmt.Sprintf("项目应付 %.2f 元", summary.CostPayable))
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

func buildContractAggregateMarginNote(selection contractAggregateSelection, summary contractAggregateSummary) string {
	if !selection.IncludeProfit {
		return ""
	}
	parts := []string{
		fmt.Sprintf("项目毛利=项目结算收入 %.2f 元 - 项目成本 %.2f 元 = %.2f 元", round2(summary.RevenueSettlement), round2(summary.CostSettlement), round2(summary.Profit)),
	}
	if summary.HasNetProfitContext {
		parts = append(parts, fmt.Sprintf("财报净利口径 %.2f 元", round2(summary.NetProfitContext)))
	}
	return "口径说明：" + strings.Join(parts, "；") + "。"
}

func buildContractAggregateComparisonNote(summary contractAggregateSummary) string {
	if summary.RevenueComparison == nil {
		return ""
	}
	cmp := summary.RevenueComparison
	direction := "低于"
	if cmp.DifferenceVsAverage >= 0 {
		direction = "高于"
	}
	currentLabel := displayContractAggregateComparisonLabel(cmp.CurrentFrom, cmp.CurrentTo, cmp.CurrentLabel)
	return fmt.Sprintf("%s收入 %.2f 元；%s收入 %.2f 元、月均 %.2f 元，%s较%s月均%s %.2f 元。",
		currentLabel,
		round2(cmp.CurrentRevenue),
		cmp.BaselineLabel,
		round2(cmp.BaselineRevenue),
		round2(cmp.BaselineMonthlyAverage),
		currentLabel,
		cmp.BaselineLabel,
		direction,
		round2(absFloat(cmp.DifferenceVsAverage)))
}

func displayContractAggregateComparisonLabel(from, to, fallback string) string {
	if strings.TrimSpace(from) != "" && from == to {
		return displaySubPeriodLabel(from)
	}
	return fallback
}

func buildContractAggregateSupplement(selection contractAggregateSelection, summary contractAggregateSummary) string {
	switch {
	case selection.IncludeProfit && !selection.IncludeRevenue && !selection.IncludeCost:
		return fmt.Sprintf("补充项目现金净额 %.2f 元（回款 %.2f 元，付款 %.2f 元）。",
			round2(summary.RevenueReceived-summary.CostPaid),
			round2(summary.RevenueReceived),
			round2(summary.CostPaid))
	case selection.IncludeCost && !selection.IncludeRevenue && !selection.IncludeProfit:
		return fmt.Sprintf("补充项目现金付款 %.2f 元。%s", round2(summary.CostPaid), buildCostDetailSentence(summary.CostItems))
	case selection.IncludeRevenue && !selection.IncludeCost && !selection.IncludeProfit:
		return fmt.Sprintf("补充项目现金到账 %.2f 元，已开票 %.2f 元。%s%s", round2(summary.RevenueReceived), round2(summary.RevenueInvoiced), buildRevenueRankingSentence(summary.RevenueCustomerRanking, summary.Top2RevenueSettlement, summary.Top2RevenueShare), buildRevenueDetailSentence(summary.RevenueItems))
	case selection.IncludeReceivable && !selection.IncludePayable:
		return fmt.Sprintf("补充项目结算 %.2f 元、已到账 %.2f 元；其中已开票未回款 %.2f 元。%s%s", round2(summary.RevenueSettlement), round2(summary.RevenueReceived), round2(summary.RevenueInvoiceOpen), buildRevenueReceivableCustomerSentence(summary), buildRevenueReceivableDetailSentence(summary.RevenueItems))
	case selection.IncludePayable && !selection.IncludeReceivable:
		return fmt.Sprintf("补充项目成本 %.2f 元、已付款 %.2f 元；其中已收票未付款 %.2f 元。", round2(summary.CostSettlement), round2(summary.CostPaid), round2(summary.CostInvoiceOpen))
	case selection.IncludeInvoiceAR && !selection.IncludeInvoiceAP:
		detail := buildRevenueInvoiceOpenDetailSentence(summary.RevenueInvoiceOpenItems)
		return fmt.Sprintf("补充已开票 %.2f 元、已到账 %.2f 元。%s", round2(summary.RevenueInvoiced), round2(summary.RevenueReceived), detail)
	case selection.IncludeInvoiceAP && !selection.IncludeInvoiceAR:
		detail := buildCostInvoiceOpenDetailSentence(summary.CostInvoiceOpenItems)
		return fmt.Sprintf("补充已收票 %.2f 元、已付款 %.2f 元。%s", round2(summary.CostInvoiced), round2(summary.CostPaid), detail)
	default:
		parts := make([]string, 0, 4)
		if selection.IncludeRevenue {
			parts = append(parts, fmt.Sprintf("项目回款 %.2f 元", round2(summary.RevenueReceived)))
			parts = append(parts, fmt.Sprintf("已开票 %.2f 元", round2(summary.RevenueInvoiced)))
		}
		if selection.IncludeCost {
			parts = append(parts, fmt.Sprintf("项目付款 %.2f 元", round2(summary.CostPaid)))
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

func buildRevenueReceivableCustomerSentence(summary contractAggregateSummary) string {
	rows := summary.RevenueOpenRanking
	if len(rows) == 0 {
		return ""
	}
	bucketsByName := map[string]contractAggregateOpenBucket{}
	for _, bucket := range summary.RevenueOpenBuckets {
		bucketsByName[bucket.Name] = bucket
	}
	parts := make([]string, 0, len(rows))
	for i, row := range rows {
		if i >= 3 {
			break
		}
		part := fmt.Sprintf("%s 结算 %.2f 元、已回款 %.2f 元、未回款 %.2f 元", row.Name, round2(row.SettlementAmount), round2(row.MovementAmount), round2(row.OpenAmount))
		if bucket, ok := bucketsByName[row.Name]; ok {
			part += fmt.Sprintf("（%s未回款 %.2f 元，%s未回款 %.2f 元）", bucket.PriorLabel, round2(bucket.PriorOpen), bucket.CurrentLabel, round2(bucket.CurrentOpen))
		}
		parts = append(parts, part)
	}
	prefix := "客户挂账汇总："
	if containsAny(summary.OriginalQuestion, []string{"催", "最该", "最多"}) {
		prefix = "优先催收客户："
	}
	return contractAggregateDetailSentence(parts, len(rows), prefix)
}

func buildRevenueReceivableDetailSentence(items []contractAggregateOpenItem) string {
	openItems := filterOpenContractAggregateItems(items)
	if len(openItems) == 0 {
		return ""
	}
	parts := make([]string, 0, len(openItems))
	for i, item := range openItems {
		if i >= 3 {
			break
		}
		label := contractAggregateItemLabel(item)
		parts = append(parts, fmt.Sprintf("%s 结算 %.2f 元、已回款 %.2f 元、未回款 %.2f 元", label, round2(item.SettlementAmount), round2(item.ReceivedAmount), round2(item.OpenAmount)))
	}
	return contractAggregateDetailSentence(parts, len(openItems), "挂账明细：")
}

func buildRevenueRankingSentence(rows []contractAggregateDimensionRow, top2Settlement, top2Share float64) string {
	if len(rows) == 0 {
		return ""
	}
	parts := make([]string, 0, len(rows))
	for i, row := range rows {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %.2f 元（%.2f%%）", row.Name, round2(row.SettlementAmount), round2(row.Share*100)))
	}
	suffix := ""
	if len(rows) > 3 {
		suffix = fmt.Sprintf("等 %d 个客户", len(rows))
	}
	if len(rows) >= 2 {
		suffix += fmt.Sprintf("，前两家合计 %.2f 元、%.2f%%、约%.0f%%", round2(top2Settlement), round2(top2Share*100), top2Share*100)
	}
	return "客户收入排名：" + strings.Join(parts, "；") + suffix + "。"
}

func buildCostInvoiceOpenDetailSentence(items []contractAggregateOpenItem) string {
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
		parts = append(parts, fmt.Sprintf("%s 已收票 %.2f 元、已付款 %.2f 元、未付款 %.2f 元", label, round2(item.InvoiceAmount), round2(item.ReceivedAmount), round2(item.OpenAmount)))
	}
	suffix := ""
	if len(items) > 3 {
		suffix = fmt.Sprintf("等 %d 个项目", len(items))
	}
	return "明细：" + strings.Join(parts, "；") + suffix + "。"
}

func buildRevenueDetailSentence(items []contractAggregateOpenItem) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for i, item := range items {
		if i >= 3 {
			break
		}
		label := contractAggregateItemLabel(item)
		parts = append(parts, fmt.Sprintf("%s 结算 %.2f 元、已回款 %.2f 元、已开票 %.2f 元", label, round2(item.SettlementAmount), round2(item.ReceivedAmount), round2(item.InvoiceAmount)))
	}
	return contractAggregateDetailSentence(parts, len(items), "明细：")
}

func buildCostDetailSentence(items []contractAggregateOpenItem) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for i, item := range items {
		if i >= 3 {
			break
		}
		label := contractAggregateItemLabel(item)
		parts = append(parts, fmt.Sprintf("%s 结算 %.2f 元、已付款 %.2f 元、已收票 %.2f 元", label, round2(item.SettlementAmount), round2(item.ReceivedAmount), round2(item.InvoiceAmount)))
	}
	return contractAggregateDetailSentence(parts, len(items), "明细：")
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
		label := contractAggregateItemLabel(item)
		parts = append(parts, fmt.Sprintf("%s 已开票 %.2f 元、已回款 %.2f 元、未回款 %.2f 元", label, round2(item.InvoiceAmount), round2(item.ReceivedAmount), round2(item.OpenAmount)))
	}
	return contractAggregateDetailSentence(parts, len(items), "明细：")
}

func contractAggregateItemLabel(item contractAggregateOpenItem) string {
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
	return label
}

func contractAggregateDetailSentence(parts []string, itemCount int, prefix string) string {
	if len(parts) == 0 {
		return ""
	}
	suffix := ""
	if itemCount > len(parts) {
		suffix = fmt.Sprintf("等 %d 个项目", itemCount)
	}
	return prefix + strings.Join(parts, "；") + suffix + "。"
}
