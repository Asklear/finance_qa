package query

import (
	"testing"
	"time"
)

func TestExtractReceiptSubPeriodCapturesSingleMonthSubQuestion(t *testing.T) {
	subPeriod, ok := extractReceiptSubPeriod("金程今年回款多少？其中3月到账多少？", "2026-01", "2026-04")
	if !ok {
		t.Fatalf("expected sub period to be extracted")
	}
	if subPeriod != "2026-03" {
		t.Fatalf("subPeriod = %q, want %q", subPeriod, "2026-03")
	}
}

func TestDisplayReceiptPeriodLabelUsesCurrentYearShortcut(t *testing.T) {
	if got := displayReceiptPeriodLabel("金程今年回款多少？其中3月到账多少？", "2026-01", "2026-04"); got != "今年" {
		t.Fatalf("displayReceiptPeriodLabel() = %q, want %q", got, "今年")
	}
	if got := displayReceiptPeriodLabel("2026年1月到2026年3月回款多少？", "2026-01", "2026-03"); got != "2026-01~2026-03 " {
		t.Fatalf("displayReceiptPeriodLabel(range) = %q, want %q", got, "2026-01~2026-03 ")
	}
}

func TestParseAnchorDateValueSupportsStringsAndBytes(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want string
	}{
		{name: "date string", raw: "2026-03-31", want: "2026-03-31"},
		{name: "timestamp bytes", raw: []byte("2026-03-31T00:00:00Z"), want: "2026-03-31"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseAnchorDateValue(tc.raw)
			if !ok {
				t.Fatalf("parseAnchorDateValue(%v) returned ok=false", tc.raw)
			}
			if got.Format("2006-01-02") != tc.want {
				t.Fatalf("parseAnchorDateValue(%v) = %s, want %s", tc.raw, got.Format("2006-01-02"), tc.want)
			}
		})
	}

	explicit := time.Date(2026, time.April, 23, 0, 0, 0, 0, time.UTC)
	got, ok := parseAnchorDateValue(explicit)
	if !ok || !got.Equal(explicit) {
		t.Fatalf("parseAnchorDateValue(time.Time) = (%v,%t), want (%v,true)", got, ok, explicit)
	}
}
