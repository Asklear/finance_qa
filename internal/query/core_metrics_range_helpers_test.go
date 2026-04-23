package query

import "testing"

func TestBuildCumulativeDeltaBookDerivesProfitAndNetProfit(t *testing.T) {
	accumulators := map[string]*cumulativeMetricAccumulator{
		"revenue": {
			LatestPeriod:       "2026-03",
			LatestCumulative:   600,
			HasLatest:          true,
			PreviousPeriod:     "2026-01",
			PreviousCumulative: 100,
			HasPrevious:        true,
		},
		"cost": {
			LatestPeriod:       "2026-03",
			LatestCumulative:   260,
			HasLatest:          true,
			PreviousPeriod:     "2026-01",
			PreviousCumulative: 60,
			HasPrevious:        true,
		},
		"admin_expense": {
			LatestPeriod:       "2026-03",
			LatestCumulative:   55,
			HasLatest:          true,
			PreviousPeriod:     "2026-01",
			PreviousCumulative: 5,
			HasPrevious:        true,
		},
		"tax_surcharge": {
			LatestPeriod:       "2026-03",
			LatestCumulative:   10,
			HasLatest:          true,
			PreviousPeriod:     "2026-01",
			PreviousCumulative: 2,
			HasPrevious:        true,
		},
		"non_operating_income": {
			LatestPeriod:       "2026-03",
			LatestCumulative:   21,
			HasLatest:          true,
			PreviousPeriod:     "2026-01",
			PreviousCumulative: 1,
			HasPrevious:        true,
		},
		"non_operating_expense": {
			LatestPeriod:       "2026-03",
			LatestCumulative:   6,
			HasLatest:          true,
			PreviousPeriod:     "2026-01",
			PreviousCumulative: 1,
			HasPrevious:        true,
		},
		"income_tax": {
			LatestPeriod:       "2026-03",
			LatestCumulative:   35,
			HasLatest:          true,
			PreviousPeriod:     "2026-01",
			PreviousCumulative: 5,
			HasPrevious:        true,
		},
	}

	got := buildCumulativeDeltaBook(accumulators)

	if got.Revenue != 500 {
		t.Fatalf("Revenue = %.2f, want 500", got.Revenue)
	}
	if got.TotalCost != 258 {
		t.Fatalf("TotalCost = %.2f, want 258", got.TotalCost)
	}
	if got.OperatingProfit != 242 {
		t.Fatalf("OperatingProfit = %.2f, want 242", got.OperatingProfit)
	}
	if got.Profit != 257 {
		t.Fatalf("Profit = %.2f, want 257", got.Profit)
	}
	if got.NetProfit != 227 {
		t.Fatalf("NetProfit = %.2f, want 227", got.NetProfit)
	}
}
