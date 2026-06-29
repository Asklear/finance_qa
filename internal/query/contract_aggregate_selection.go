package query

import (
	"fmt"
	"strings"
)

type contractAggregateSelection struct {
	RequestedMetrics  []string
	PrimaryMetric     string
	IncludeRevenue    bool
	IncludeCost       bool
	IncludeProfit     bool
	IncludeReceivable bool
	IncludePayable    bool
	IncludeInvoiceAR  bool
	IncludeInvoiceAP  bool
}

func resolveContractAggregateSelection(spec QuerySpec, summary contractAggregateSummary) contractAggregateSelection {
	requestedMetrics := append([]string{}, summary.RequestedMetrics...)
	return contractAggregateSelection{
		RequestedMetrics:  requestedMetrics,
		PrimaryMetric:     firstMetricOrDefault(requestedMetrics, detectCoreMetric(spec.OriginalQuestion)),
		IncludeRevenue:    contractAggregateIncludesMetric(requestedMetrics, "收入"),
		IncludeCost:       contractAggregateIncludesMetric(requestedMetrics, "成本"),
		IncludeProfit:     contractAggregateIncludesMetric(requestedMetrics, "利润"),
		IncludeReceivable: contractAggregateIncludesMetric(requestedMetrics, "应收"),
		IncludePayable:    contractAggregateIncludesMetric(requestedMetrics, "应付"),
		IncludeInvoiceAR:  contractAggregateIncludesMetric(requestedMetrics, "已开票未回款"),
		IncludeInvoiceAP:  contractAggregateIncludesMetric(requestedMetrics, "已收票未付款"),
	}
}

func buildContractAggregateScopeLabel(summary contractAggregateSummary) string {
	scopeLabel := fmt.Sprintf("%s 老板口径先看项目汇总", summary.Period)
	if strings.TrimSpace(summary.Entity) != "" {
		scopeLabel = fmt.Sprintf("[%s] %s 老板口径先看项目汇总", summary.Entity, summary.Period)
	}
	return scopeLabel
}

func contractAggregateIncludesMetric(requestedMetrics []string, metric string) bool {
	if len(requestedMetrics) == 0 {
		return true
	}
	for _, requested := range requestedMetrics {
		if strings.TrimSpace(requested) == metric {
			return true
		}
	}
	return false
}

func contractAggregateMetricValue(metric string, summary contractAggregateSummary) float64 {
	switch strings.TrimSpace(metric) {
	case "成本":
		return round2(summary.CostSettlement)
	case "利润":
		return round2(summary.Profit)
	case "应收":
		return round2(summary.RevenueReceivable)
	case "应付":
		return round2(summary.CostPayable)
	case "已开票未回款":
		return round2(summary.RevenueInvoiceOpen)
	case "已收票未付款":
		return round2(summary.CostInvoiceOpen)
	default:
		return round2(summary.RevenueSettlement)
	}
}

func contractAggregateMetricLabel(metric string) string {
	switch strings.TrimSpace(metric) {
	case "收入":
		return "项目结算收入（营收）"
	case "成本":
		return "项目成本"
	case "利润":
		return "利润"
	case "应收":
		return "项目应收（应收未收）"
	case "应付":
		return "项目应付（应付未付/未付款）"
	case "已开票未回款":
		return "项目口径已开票未回款"
	case "已收票未付款":
		return "项目成本口径已收票未付款"
	default:
		return strings.TrimSpace(metric)
	}
}

func contractAggregateBusinessBasis(metric string) string {
	switch strings.TrimSpace(metric) {
	case "收入":
		return "项目口径：按合同资金收入/收入成本表的项目结算收入统计，不按银行到账或账上余额口径。"
	case "成本":
		return "项目成本口径：按合同成本结算金额统计。"
	case "利润":
		return "项目经营口径：项目结算收入减项目成本，作为项目毛利/利润。"
	case "应收":
		return "项目口径：项目结算收入减已到账，表示项目应收未收。"
	case "应付":
		return "项目成本口径：项目成本减已付款，表示应付未付/未付款，不按收入未回款口径。"
	case "已开票未回款":
		return "项目口径：已开票金额减已到账，表示收入侧已开票未回款。"
	case "已收票未付款":
		return "项目成本口径：已收票金额减已付款，表示供应商侧已收票未付款。"
	default:
		return "项目经营口径。"
	}
}
