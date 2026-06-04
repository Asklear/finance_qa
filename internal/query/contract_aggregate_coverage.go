package query

import (
	"fmt"
	"strings"
)

func contractAggregateCanAnswer(requestedMetrics []string, summary contractAggregateSummary) bool {
	for _, metric := range requestedMetrics {
		switch strings.TrimSpace(metric) {
		case "收入":
			if !summary.HasRevenueCoverage {
				return false
			}
		case "成本":
			if !summary.HasCostCoverage {
				return false
			}
		case "利润":
			if !(summary.HasRevenueCoverage && summary.HasCostCoverage) {
				return false
			}
		case "应收":
			if !summary.HasRevenueCoverage {
				return false
			}
		case "应付":
			if !summary.HasCostCoverage {
				return false
			}
		case "已开票未回款":
			if !summary.HasRevenueCoverage {
				return false
			}
		case "已收票未付款":
			if !summary.HasCostCoverage {
				return false
			}
		}
	}
	return len(requestedMetrics) > 0
}

func contractAggregateFallbackReason(requestedMetrics []string, summary contractAggregateSummary) string {
	missing := make([]string, 0, 2)
	for _, metric := range requestedMetrics {
		switch strings.TrimSpace(metric) {
		case "收入":
			if !summary.HasRevenueCoverage {
				missing = append(missing, "营收结算")
			}
		case "成本":
			if !summary.HasCostCoverage {
				missing = append(missing, "项目成本")
			}
		case "利润":
			if !summary.HasRevenueCoverage {
				missing = append(missing, "营收结算")
			}
			if !summary.HasCostCoverage {
				missing = append(missing, "项目成本")
			}
		case "应收":
			if !summary.HasRevenueCoverage {
				missing = append(missing, "项目应收")
			}
		case "应付":
			if !summary.HasCostCoverage {
				missing = append(missing, "项目应付")
			}
		case "已开票未回款":
			if !summary.HasRevenueCoverage {
				missing = append(missing, "已开票未回款")
			}
		case "已收票未付款":
			if !summary.HasCostCoverage {
				missing = append(missing, "已收票未付款")
			}
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("项目汇总表当前缺少%s", joinWithComma(dedupeStrings(missing)))
}
