package query

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ExtractPeriodWithNow 从自然语言提取账期。
func ExtractPeriodWithNow(question string, anchor time.Time) (string, string) {
	year := anchor.Year()
	anchorMonth := int(anchor.Month())
	q := strings.TrimSpace(question)

	if from, to, ok := extractFullYearRange(q, year); ok {
		return from, to
	}
	if from, to, ok := extractHalfYearRange(q, year, anchorMonth); ok {
		return from, to
	}
	if from, to, ok := extractQuarterRange(q, year, anchorMonth); ok {
		return from, to
	}
	if from, to, ok := extractExplicitMonthRange(q); ok {
		return from, to
	}
	if from, to, ok := extractYearCumulativeRange(q, year, anchorMonth); ok {
		return from, to
	}
	if from, to, ok := extractExplicitYearMonthRange(q); ok {
		return from, to
	}
	if from, to, ok := extractRelativeNamedRange(q, anchor); ok {
		return from, to
	}
	if from, to, ok := extractRelativeMonthRange(q, year, anchorMonth); ok {
		return from, to
	}

	period := anchor.Format("2006-01")
	return period, period
}

func extractFullYearRange(q string, anchorYear int) (string, string, bool) {
	fullYearRe := regexp.MustCompile(`(20\d{2})年\s*(?:全年|整年|全年度|年度)`)
	if m := fullYearRe.FindStringSubmatch(q); len(m) == 2 {
		y := mustAtoi(m[1])
		return formatPeriodValue(y, 1), formatPeriodValue(y, 12), true
	}
	if strings.Contains(q, "今年全年") || strings.Contains(q, "本年全年") {
		return formatPeriodValue(anchorYear, 1), formatPeriodValue(anchorYear, 12), true
	}
	return "", "", false
}

func extractHalfYearRange(q string, anchorYear, anchorMonth int) (string, string, bool) {
	explicitHalfRe := regexp.MustCompile(`(20\d{2})年\s*(上半年|下半年)`)
	if m := explicitHalfRe.FindStringSubmatch(q); len(m) == 3 {
		y := mustAtoi(m[1])
		if strings.Contains(m[2], "上") {
			from, to := halfRange(y, 1)
			return from, to, true
		}
		from, to := halfRange(y, 2)
		return from, to, true
	}
	if strings.Contains(q, "上半年") || strings.Contains(q, "下半年") {
		target := "上半年"
		if strings.Contains(q, "下半年") {
			target = "下半年"
		}
		from, to := resolveRelativeHalfRange(anchorYear, anchorMonth, target)
		return from, to, true
	}
	return "", "", false
}

func extractQuarterRange(q string, anchorYear, anchorMonth int) (string, string, bool) {
	explicitQuarterRe := regexp.MustCompile(`(20\d{2})年\s*(?:第?\s*([一二三四1234])\s*季度|Q\s*([1-4]))`)
	if m := explicitQuarterRe.FindStringSubmatch(q); len(m) == 4 {
		y := mustAtoi(m[1])
		token := m[2]
		if token == "" {
			token = m[3]
		}
		if quarter := parseQuarterToken(token); quarter >= 1 && quarter <= 4 {
			from, to := quarterRange(y, quarter)
			return from, to, true
		}
	}
	relativeQuarterRe := regexp.MustCompile(`(?:第?\s*([一二三四1234])\s*季度|Q\s*([1-4]))`)
	if m := relativeQuarterRe.FindStringSubmatch(q); len(m) == 3 {
		token := m[1]
		if token == "" {
			token = m[2]
		}
		if quarter := parseQuarterToken(token); quarter >= 1 && quarter <= 4 {
			from, to := resolveRelativeQuarterRange(anchorYear, anchorMonth, token)
			return from, to, true
		}
	}
	return "", "", false
}

func extractExplicitMonthRange(q string) (string, string, bool) {
	rangeRe := regexp.MustCompile(`(20\d{2})年\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月?\s*(?:到|至|-|~)\s*(20\d{2})年\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月`)
	m := rangeRe.FindStringSubmatch(q)
	if len(m) != 5 {
		return "", "", false
	}
	y1, _ := strconv.Atoi(m[1])
	y2, _ := strconv.Atoi(m[3])
	m1 := parseChineseOrDigitMonth(m[2])
	m2 := parseChineseOrDigitMonth(m[4])
	if !validMonth(m1) || !validMonth(m2) {
		return "", "", false
	}
	return formatPeriodValue(y1, m1), formatPeriodValue(y2, m2), true
}

func extractYearCumulativeRange(q string, anchorYear, anchorMonth int) (string, string, bool) {
	yearCumulativeRe := regexp.MustCompile(`(20\d{2})年?\s*(?:累计|年内|累计销售额|累计收入|累计营收|累计回款)`)
	if m := yearCumulativeRe.FindStringSubmatch(q); len(m) == 2 {
		y := mustAtoi(m[1])
		endMonth := 12
		if y == anchorYear {
			endMonth = anchorMonth
		}
		return formatPeriodValue(y, 1), formatPeriodValue(y, endMonth), true
	}
	return "", "", false
}

func extractExplicitYearMonthRange(q string) (string, string, bool) {
	type ym struct {
		year  int
		month int
	}
	ymRe := regexp.MustCompile(`(20\d{2})年\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月`)
	yms := ymRe.FindAllStringSubmatch(q, -1)
	if len(yms) >= 2 {
		first := ym{year: mustAtoi(yms[0][1]), month: parseChineseOrDigitMonth(yms[0][2])}
		last := ym{year: mustAtoi(yms[len(yms)-1][1]), month: parseChineseOrDigitMonth(yms[len(yms)-1][2])}
		if validMonth(first.month) && validMonth(last.month) {
			return formatPeriodValue(first.year, first.month), formatPeriodValue(last.year, last.month), true
		}
	}
	if len(yms) == 1 {
		y := mustAtoi(yms[0][1])
		m := parseChineseOrDigitMonth(yms[0][2])
		if validMonth(m) {
			p := formatPeriodValue(y, m)
			return p, p, true
		}
	}
	return "", "", false
}

func extractRelativeNamedRange(q string, anchor time.Time) (string, string, bool) {
	switch {
	case strings.Contains(q, "今年") || strings.Contains(q, "本年"):
		return formatPeriodValue(anchor.Year(), 1), anchor.Format("2006-01"), true
	case strings.Contains(q, "上个月"):
		t := anchor.AddDate(0, -1, 0)
		p := t.Format("2006-01")
		return p, p, true
	case strings.Contains(q, "下个月"):
		t := anchor.AddDate(0, 1, 0)
		p := t.Format("2006-01")
		return p, p, true
	case strings.Contains(q, "本月") || strings.Contains(q, "这个月") || strings.Contains(q, "当月"):
		p := anchor.Format("2006-01")
		return p, p, true
	}
	return "", "", false
}

func extractRelativeMonthRange(q string, anchorYear, anchorMonth int) (string, string, bool) {
	monthRe := regexp.MustCompile(`([0-1]?\d|[一二三四五六七八九十两]{1,3})月`)
	if m := monthRe.FindStringSubmatch(q); len(m) == 2 {
		month := parseChineseOrDigitMonth(m[1])
		if validMonth(month) {
			y := anchorYear
			if month > anchorMonth && (month-anchorMonth) >= 6 {
				y = anchorYear - 1
			}
			p := formatPeriodValue(y, month)
			return p, p, true
		}
	}
	return "", "", false
}
