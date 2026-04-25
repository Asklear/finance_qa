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
	scopeLabel := fmt.Sprintf("%s 老板口径先看合同/项目汇总", summary.Period)
	if strings.TrimSpace(summary.Entity) != "" {
		scopeLabel = fmt.Sprintf("[%s] %s 老板口径先看合同/项目汇总", summary.Entity, summary.Period)
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
