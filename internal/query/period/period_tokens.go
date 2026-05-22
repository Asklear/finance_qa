package period

import (
	"fmt"
	"strconv"
	"strings"
)

func formatPeriodValue(year, month int) string {
	return fmt.Sprintf("%04d-%02d", year, month)
}

func halfRange(year, half int) (string, string) {
	if half == 1 {
		return formatPeriodValue(year, 1), formatPeriodValue(year, 6)
	}
	return formatPeriodValue(year, 7), formatPeriodValue(year, 12)
}

func quarterRange(year, quarter int) (string, string) {
	startMonth := (quarter-1)*3 + 1
	endMonth := startMonth + 2
	return formatPeriodValue(year, startMonth), formatPeriodValue(year, endMonth)
}

func parseQuarterToken(token string) int {
	s := strings.ToUpper(strings.TrimSpace(token))
	s = strings.TrimPrefix(s, "第")
	s = strings.TrimSuffix(s, "季度")
	s = strings.TrimSuffix(s, "季")
	s = strings.TrimSuffix(s, "度")
	s = strings.TrimPrefix(s, "Q")
	switch s {
	case "1", "一":
		return 1
	case "2", "二", "两":
		return 2
	case "3", "三":
		return 3
	case "4", "四":
		return 4
	}
	return 0
}

func parseChineseOrDigitMonth(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	switch raw {
	case "一":
		return 1
	case "二", "两":
		return 2
	case "三":
		return 3
	case "四":
		return 4
	case "五":
		return 5
	case "六":
		return 6
	case "七":
		return 7
	case "八":
		return 8
	case "九":
		return 9
	case "十":
		return 10
	case "十一":
		return 11
	case "十二":
		return 12
	default:
		return 0
	}
}

func validMonth(m int) bool {
	return m >= 1 && m <= 12
}

func normalizeYearToken(raw string) int {
	y := mustAtoi(strings.TrimSpace(raw))
	if y >= 0 && y < 100 {
		return 2000 + y
	}
	return y
}

func MustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func mustAtoi(s string) int {
	return MustAtoi(s)
}
