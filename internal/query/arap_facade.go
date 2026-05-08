package query

import (
	"financeqa/internal/openitems"
	queryarap "financeqa/internal/query/arap"
)

func formatARAPOpenItemSummaryMessage(period, account, entity string, summary openitems.Summary, historyLabel, currentLabel string) string {
	return queryarap.FormatOpenItemSummaryMessage(period, account, entity, summary, historyLabel, currentLabel)
}

func buildARAPOpenItemMessage(data map[string]any) string {
	return queryarap.BuildOpenItemMessage(data)
}
