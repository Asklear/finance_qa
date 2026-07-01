package query

import (
	"regexp"
	"strings"
	"time"
)

func (e *Engine) periodParserAnchorForQuestion(question string, anchor time.Time) time.Time {
	if !e.asOfAnchor.IsZero() || anchor.IsZero() {
		return anchor
	}
	if shouldAdvanceDataMonthAnchorForCompleteWindow(question, anchor) {
		return anchor.AddDate(0, 1, 0)
	}
	return anchor
}

func shouldAdvanceDataMonthAnchorForCompleteWindow(question string, anchor time.Time) bool {
	q := strings.TrimSpace(question)
	if q == "" {
		return false
	}
	if hasExplicitStartToCurrentCompleteWindow(q) || hasPreviousCompleteMonthToken(q) {
		return true
	}
	return looseYearRangeEndsAtYear(q, anchor.Year())
}

func hasExplicitStartToCurrentCompleteWindow(q string) bool {
	return regexp.MustCompile(`(?:从|自)?\s*(20\d{2}|\d{2}|今年|本年|去年)\s*年?\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月?(?:底|末)?\s*(?:起|开始)?\s*(?:到|至|截至|截止)?\s*(?:至今|现在|目前)`).MatchString(q)
}

func hasPreviousCompleteMonthToken(q string) bool {
	return containsAny(q, []string{"上一个完整自然月", "上个完整自然月", "上个月", "上月"})
}
