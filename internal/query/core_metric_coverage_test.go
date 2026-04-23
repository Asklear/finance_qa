package query

import "testing"

func TestBuildCoreMetricCoverageAppliesTruncationAndNoDataRules(t *testing.T) {
	cases := []struct {
		name           string
		requestedFrom  string
		requestedTo    string
		availableTo    string
		wantActualFrom string
		wantActualTo   string
		wantTruncated  bool
		wantHasData    bool
	}{
		{
			name:           "truncate to latest available period",
			requestedFrom:  "2026-01",
			requestedTo:    "2026-04",
			availableTo:    "2026-03",
			wantActualFrom: "2026-01",
			wantActualTo:   "2026-03",
			wantTruncated:  true,
			wantHasData:    true,
		},
		{
			name:           "no data when requested range starts after latest available period",
			requestedFrom:  "2026-05",
			requestedTo:    "2026-06",
			availableTo:    "2026-03",
			wantActualFrom: "",
			wantActualTo:   "",
			wantTruncated:  false,
			wantHasData:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildCoreMetricCoverage(tc.requestedFrom, tc.requestedTo, tc.availableTo)
			if got.ActualFrom != tc.wantActualFrom || got.ActualTo != tc.wantActualTo {
				t.Fatalf("buildCoreMetricCoverage(%s,%s,%s) actual=%s~%s, want %s~%s", tc.requestedFrom, tc.requestedTo, tc.availableTo, got.ActualFrom, got.ActualTo, tc.wantActualFrom, tc.wantActualTo)
			}
			if got.Truncated != tc.wantTruncated {
				t.Fatalf("buildCoreMetricCoverage(%s,%s,%s) truncated=%t, want %t", tc.requestedFrom, tc.requestedTo, tc.availableTo, got.Truncated, tc.wantTruncated)
			}
			if got.HasData != tc.wantHasData {
				t.Fatalf("buildCoreMetricCoverage(%s,%s,%s) hasData=%t, want %t", tc.requestedFrom, tc.requestedTo, tc.availableTo, got.HasData, tc.wantHasData)
			}
		})
	}
}
