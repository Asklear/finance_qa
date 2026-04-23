package query

import "math"

type cumulativeValidationSummary struct {
	CurrentSum      float64
	CumulativeDelta float64
	Diff            float64
	Passed          bool
	LatestPeriod    string
	PreviousPeriod  string
}

func summarizeCumulativeValidationAccumulator(acc cumulativeValidationAccumulator) cumulativeValidationSummary {
	previous := 0.0
	if acc.PreviousCumu.Valid {
		previous = acc.PreviousCumu.Float64
	}
	cumulativeDelta := 0.0
	if acc.LatestCumu.Valid {
		cumulativeDelta = round2(acc.LatestCumu.Float64 - previous)
	}
	diff := round2(acc.CurrentSum - cumulativeDelta)
	return cumulativeValidationSummary{
		CurrentSum:      round2(acc.CurrentSum),
		CumulativeDelta: cumulativeDelta,
		Diff:            diff,
		Passed:          math.Abs(diff) <= 1.00,
		LatestPeriod:    acc.LatestAt,
		PreviousPeriod:  acc.PreviousAt,
	}
}
