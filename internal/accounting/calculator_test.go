package accounting

import "testing"

func TestMonthDateBounds(t *testing.T) {
	tests := []struct {
		year      int
		month     int
		wantStart string
		wantEnd   string
	}{
		{year: 2026, month: 4, wantStart: "2026-04-01", wantEnd: "2026-04-30"},
		{year: 2026, month: 12, wantStart: "2026-12-01", wantEnd: "2026-12-31"},
		{year: 2024, month: 2, wantStart: "2024-02-01", wantEnd: "2024-02-29"},
	}

	for _, tc := range tests {
		start, end := monthDateBounds(tc.year, tc.month)
		if start != tc.wantStart || end != tc.wantEnd {
			t.Fatalf("monthDateBounds(%d, %d) = (%s, %s), want (%s, %s)", tc.year, tc.month, start, end, tc.wantStart, tc.wantEnd)
		}
	}
}
