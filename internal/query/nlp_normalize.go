package query

import (
	"strconv"
	"strings"
	"time"
)

var digitMap = map[string]string{
	"一": "1", "二": "2", "两": "2", "三": "3",
	"四": "4", "五": "5", "六": "6", "七": "7",
	"八": "8", "九": "9", "十": "10",
	"十一": "11", "十二": "12",
}

// NormalizeQuestion normalizes natural language dates to numerical representations
// and maps temporal pronouns like "今年", "这个月" to explicit dates.
func NormalizeQuestion(question string) string {
	q := strings.TrimSpace(question)
	
	// Default to replacing current year
	// Note: Currently DB only covers 2026. Ideally this dynamically matches DB logic.
	now := time.Now()
	
	// Convert "三月" to "3月"
	for cn, num := range digitMap {
		if strings.Contains(q, cn+"月") {
			q = strings.ReplaceAll(q, cn+"月", num+"月")
		}
	}

	// Dynamic temporal fallback: if user says "这个月" / "本月", we map it to latest DB month (2026-02)
	// Because currently we know 2026-02 is the latest robust active month.
	// In a real system, we'd query SELECT MAX(period) FROM journal.
	latestMonthStr := "2026年2月" // Fallback to 2月 as requested
	
	if strings.Contains(q, "这个月") {
		q = strings.ReplaceAll(q, "这个月", latestMonthStr)
	}
	if strings.Contains(q, "本月") {
		q = strings.ReplaceAll(q, "本月", latestMonthStr)
	}
	if strings.Contains(q, "上个月") {
		q = strings.ReplaceAll(q, "上个月", "2026年1月") // Hardcode fallback for 1-2月 dataset
	}
	
	// Year
	if strings.Contains(q, "今年") {
		q = strings.ReplaceAll(q, "今年", "2026年")
	}
	if strings.Contains(q, "去年") {
		q = strings.ReplaceAll(q, "去年", strconv.Itoa(now.Year()-1)+"年")
	}

	return q
}
