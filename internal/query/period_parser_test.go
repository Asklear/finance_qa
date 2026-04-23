package query

import "testing"

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
