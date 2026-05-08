package query

import (
	"time"

	queryperiod "financeqa/internal/query/period"
)

// ExtractPeriodWithNow preserves the query package API while the implementation
// lives in internal/query/period.
func ExtractPeriodWithNow(question string, anchor time.Time) (string, string) {
	return queryperiod.ExtractPeriodWithNow(question, anchor)
}

func extractReceiptSubPeriod(q, from, to string) (string, bool) {
	return queryperiod.ExtractReceiptSubPeriod(q, from, to)
}

func monthEndDay(period string) string {
	return queryperiod.MonthEndDay(period)
}

func parsePeriod(period string) (int, int) {
	return queryperiod.Parse(period)
}

func displayPeriod(from, to string) string {
	return queryperiod.Display(from, to)
}

func displaySubPeriodLabel(period string) string {
	return queryperiod.DisplaySubPeriodLabel(period)
}

func displayReceiptPeriodLabel(q, from, to string) string {
	return queryperiod.DisplayReceiptPeriodLabel(q, from, to)
}

func periodsBetween(from, to string) ([]string, error) {
	return queryperiod.Between(from, to)
}

func parseAnchorDateValue(v any) (time.Time, bool) {
	return queryperiod.ParseAnchorDateValue(v)
}

func parseAnchorDateString(raw string) (time.Time, bool) {
	return queryperiod.ParseAnchorDateString(raw)
}

func mustAtoi(s string) int {
	return queryperiod.MustAtoi(s)
}
