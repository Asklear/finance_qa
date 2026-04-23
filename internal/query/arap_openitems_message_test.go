package query

import (
	"strings"
	"testing"

	"financeqa/internal/openitems"
)

func TestBuildARAPOpenItemMessageIncludesProbableAndUnmatchedSettlements(t *testing.T) {
	summary := openitems.Summary{
		ClosingBalance:    18190.20,
		OpeningBalance:    19275.00,
		CurrentIncrease:   18190.20,
		CurrentDecrease:   19275.00,
		CurrentSettlement: 0,
		SettlementConfidence: openitems.SettlementConfidence{
			ProbableHistoricalSettlement: 1000.00,
			ProbableCurrentSettlement:    200.00,
			UnmatchedDecrease:            75.50,
		},
	}

	got := formatARAPOpenItemSummaryMessage("2026-03", "应收账款", "任拓", summary, "历史应收", "当月新增应收")

	wantParts := []string{
		"2026-03 任拓 应收账款合计 18190.20 元",
		"期初 19275.00",
		"本月新增 18190.20",
		"本月减少 19275.00",
		"高概率冲销历史应收 1000.00",
		"高概率冲销当月新增应收 200.00",
		"未能直接配对的本月减少 75.50",
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Fatalf("message %q missing part %q", got, want)
		}
	}
}
