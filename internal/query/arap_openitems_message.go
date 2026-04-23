package query

import (
	"fmt"
	"strings"

	"financeqa/internal/openitems"
)

func formatARAPOpenItemSummaryMessage(period, account, entity string, summary openitems.Summary, historyLabel, currentLabel string) string {
	scopeLabel := account
	if strings.TrimSpace(entity) != "" {
		scopeLabel = fmt.Sprintf("%s %s", entity, account)
	}

	settlementParts := make([]string, 0, 5)
	if summary.HistoricalSettlement > 0 {
		settlementParts = append(settlementParts, fmt.Sprintf("冲销%s %.2f", historyLabel, summary.HistoricalSettlement))
	}
	if summary.CurrentSettlement > 0 {
		settlementParts = append(settlementParts, fmt.Sprintf("冲销%s %.2f", currentLabel, summary.CurrentSettlement))
	}
	if summary.SettlementConfidence.ProbableHistoricalSettlement > 0 {
		settlementParts = append(settlementParts, fmt.Sprintf("高概率冲销%s %.2f", historyLabel, summary.SettlementConfidence.ProbableHistoricalSettlement))
	}
	if summary.SettlementConfidence.ProbableCurrentSettlement > 0 {
		settlementParts = append(settlementParts, fmt.Sprintf("高概率冲销%s %.2f", currentLabel, summary.SettlementConfidence.ProbableCurrentSettlement))
	}
	if summary.SettlementConfidence.UnmatchedDecrease > 0 {
		settlementParts = append(settlementParts, fmt.Sprintf("未能直接配对的本月减少 %.2f", summary.SettlementConfidence.UnmatchedDecrease))
	}

	msg := fmt.Sprintf("%s %s合计 %.2f 元（期初 %.2f，本月新增 %.2f，本月减少 %.2f",
		period,
		scopeLabel,
		summary.ClosingBalance,
		summary.OpeningBalance,
		summary.CurrentIncrease,
		summary.CurrentDecrease)
	if len(settlementParts) > 0 {
		msg += "，其中" + joinWithComma(settlementParts)
	}
	msg += "）"
	return msg
}

func buildARAPOpenItemMessage(data map[string]any) string {
	typ := anyToString(data["type"])
	historyLabel := "历史应收"
	currentLabel := "当月新增应收"
	if typ == "payable" {
		historyLabel = "历史应付"
		currentLabel = "当月新增应付"
	}

	confidence := openitems.SettlementConfidence{}
	if raw, ok := data["settlement_confidence"].(map[string]any); ok {
		confidence = openitems.SettlementConfidence{
			ConfirmedHistoricalSettlement: anyToFloat64(raw["confirmed_historical_settlement"]),
			ProbableHistoricalSettlement:  anyToFloat64(raw["probable_historical_settlement"]),
			ConfirmedCurrentSettlement:    anyToFloat64(raw["confirmed_current_settlement"]),
			ProbableCurrentSettlement:     anyToFloat64(raw["probable_current_settlement"]),
			UnmatchedDecrease:             anyToFloat64(raw["unmatched_decrease"]),
		}
	}

	return formatARAPOpenItemSummaryMessage(
		anyToString(data["period"]),
		anyToString(data["account"]),
		anyToString(data["entity"]),
		openitems.Summary{
			ClosingBalance:       anyToFloat64(data["total"]),
			OpeningBalance:       anyToFloat64(data["opening_balance"]),
			CurrentIncrease:      anyToFloat64(data["current_increase"]),
			CurrentDecrease:      anyToFloat64(data["current_decrease"]),
			HistoricalSettlement: anyToFloat64(data["historical_settlement"]),
			CurrentSettlement:    anyToFloat64(data["current_period_settlement"]),
			SettlementConfidence: confidence,
		},
		historyLabel,
		currentLabel,
	)
}
