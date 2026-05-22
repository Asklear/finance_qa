package period

import (
	"testing"
	"time"
)

func TestParseQuarterTokenSupportsChineseAndQFormats(t *testing.T) {
	cases := []struct {
		token string
		want  int
	}{
		{token: "第一季度", want: 1},
		{token: "Q2", want: 2},
		{token: "三季", want: 3},
		{token: "第4季度", want: 4},
		{token: "半年", want: 0},
	}

	for _, tc := range cases {
		if got := parseQuarterToken(tc.token); got != tc.want {
			t.Fatalf("parseQuarterToken(%q) = %d, want %d", tc.token, got, tc.want)
		}
	}
}

func TestResolveRelativeHalfRangeUsesAnchorMonth(t *testing.T) {
	from, to := resolveRelativeHalfRange(2026, 4, "下半年")
	if from != "2025-07" || to != "2025-12" {
		t.Fatalf("resolveRelativeHalfRange() = (%s,%s), want (2025-07,2025-12)", from, to)
	}
}

func TestResolveRelativeQuarterRangeUsesAnchorMonth(t *testing.T) {
	from, to := resolveRelativeQuarterRange(2026, 4, "Q4")
	if from != "2025-10" || to != "2025-12" {
		t.Fatalf("resolveRelativeQuarterRange() = (%s,%s), want (2025-10,2025-12)", from, to)
	}
}

func TestExtractPeriodWithNowTreatsBareCumulativeAsYearToDate(t *testing.T) {
	anchor := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)

	from, to := ExtractPeriodWithNow("飞未云科累计销售额多少？", anchor)

	if from != "2026-01" || to != "2026-03" {
		t.Fatalf("ExtractPeriodWithNow() = (%s,%s), want (2026-01,2026-03)", from, to)
	}
}

func TestExtractPeriodWithNowTreatsSameYearMonthDashAsRange(t *testing.T) {
	anchor := time.Date(2026, time.May, 22, 0, 0, 0, 0, time.UTC)

	from, to := ExtractPeriodWithNow("四川其妙 2026年1-3月 未付款金额是多少？", anchor)

	if from != "2026-01" || to != "2026-03" {
		t.Fatalf("ExtractPeriodWithNow() = (%s,%s), want (2026-01,2026-03)", from, to)
	}
}

func TestExtractPeriodWithNowSupportsCompactTwoDigitQuarter(t *testing.T) {
	anchor := time.Date(2026, time.May, 22, 0, 0, 0, 0, time.UTC)

	from, to := ExtractPeriodWithNow("辽宁金程25年Q4未付款金额是多少？", anchor)

	if from != "2025-10" || to != "2025-12" {
		t.Fatalf("ExtractPeriodWithNow() = (%s,%s), want (2025-10,2025-12)", from, to)
	}
}
