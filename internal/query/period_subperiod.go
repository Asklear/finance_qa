package query

import (
	"strings"
	"time"
)

func extractReceiptSubPeriod(q, from, to string) (string, bool) {
	if !strings.Contains(q, "其中") {
		return "", false
	}
	idx := strings.Index(q, "其中")
	if idx < 0 || idx >= len(q)-len("其中") {
		return "", false
	}
	subQuestion := strings.TrimSpace(q[idx+len("其中"):])
	if subQuestion == "" {
		return "", false
	}
	anchorYear, _ := parsePeriod(to)
	anchor := time.Date(anchorYear, time.December, 1, 0, 0, 0, 0, time.UTC)
	subFrom, subTo := ExtractPeriodWithNow(subQuestion, anchor)
	if subFrom == "" || subTo == "" || subFrom != subTo {
		return "", false
	}
	return subFrom, true
}
