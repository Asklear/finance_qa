package period

import (
	"fmt"
	"strings"
	"time"
)

func MonthEndDay(period string) string {
	period = strings.TrimSpace(period)
	if period == "" {
		return time.Now().Format("2006-01-02")
	}
	if t, err := time.Parse("2006-01-02", period); err == nil {
		return t.Format("2006-01-02")
	}
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Now().Format("2006-01-02")
	}
	return t.AddDate(0, 1, -1).Format("2006-01-02")
}

func Parse(period string) (int, int) {
	parts := strings.Split(period, "-")
	if len(parts) == 2 {
		y := mustAtoi(parts[0])
		m := mustAtoi(parts[1])
		return y, m
	}
	return 0, 0
}

func Display(from, to string) string {
	if strings.TrimSpace(from) == "" {
		return to
	}
	if from == to {
		return to
	}
	return from + "~" + to
}

func DisplaySubPeriodLabel(period string) string {
	year, month := Parse(period)
	if year == 0 || month == 0 {
		return period
	}
	return fmt.Sprintf("%d月", month)
}

func DisplayReceiptPeriodLabel(q, from, to string) string {
	if strings.Contains(q, "今年") && strings.HasSuffix(strings.TrimSpace(from), "-01") {
		return "今年"
	}
	return Display(from, to) + " "
}
