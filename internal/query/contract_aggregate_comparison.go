package query

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

func (e *Engine) collectContractRevenuePeriodComparison(question, defaultFrom, defaultTo, like string) (contractAggregatePeriodComparison, bool) {
	currentFrom, currentTo, baselineFrom, baselineTo, ok := parseRevenueComparisonPeriods(question, defaultFrom, defaultTo)
	if !ok {
		return contractAggregatePeriodComparison{}, false
	}
	current, err := e.collectFundIncomeTotals(context.Background(), currentFrom, currentTo, like)
	if err != nil || current.RowCount == 0 {
		return contractAggregatePeriodComparison{}, false
	}
	baseline, err := e.collectFundIncomeTotals(context.Background(), baselineFrom, baselineTo, like)
	if err != nil || baseline.RowCount == 0 {
		return contractAggregatePeriodComparison{}, false
	}
	months := countPeriodsInclusive(baselineFrom, baselineTo)
	if months <= 0 {
		months = 1
	}
	avg := round2(baseline.Settlement / float64(months))
	diff := round2(current.Settlement - avg)
	ratio := 0.0
	if avg != 0 {
		ratio = roundRatio(current.Settlement / avg)
	}
	return contractAggregatePeriodComparison{
		CurrentLabel:           displayPeriod(currentFrom, currentTo),
		CurrentFrom:            currentFrom,
		CurrentTo:              currentTo,
		CurrentRevenue:         round2(current.Settlement),
		BaselineLabel:          displayPeriod(baselineFrom, baselineTo),
		BaselineFrom:           baselineFrom,
		BaselineTo:             baselineTo,
		BaselineRevenue:        round2(baseline.Settlement),
		BaselineMonthlyAverage: avg,
		DifferenceVsAverage:    diff,
		RatioVsAverage:         ratio,
	}, true
}

func parseRevenueComparisonPeriods(question, defaultFrom, defaultTo string) (string, string, string, string, bool) {
	q := strings.TrimSpace(question)
	if !containsAny(q, []string{"比起来", "对比", "相比", "比"}) || !containsAny(q, []string{"收入", "营收", "销售额"}) {
		return "", "", "", "", false
	}
	month := extractComparisonMonth(q)
	baselineQuarter := extractComparisonQuarter(q)
	if month == 0 || baselineQuarter == 0 {
		return "", "", "", "", false
	}
	year, _ := parsePeriod(defaultTo)
	if year == 0 {
		year, _ = parsePeriod(defaultFrom)
	}
	if year == 0 {
		return "", "", "", "", false
	}
	current := fmt.Sprintf("%04d-%02d", year, month)
	baselineFrom, baselineTo := quarterPeriodRange(year, baselineQuarter)
	return current, current, baselineFrom, baselineTo, true
}

func extractComparisonMonth(q string) int {
	re := regexp.MustCompile(`(?:进入|到了?|到|看|现在)?\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})\s*月`)
	if m := re.FindStringSubmatch(q); len(m) == 2 {
		return parseContractAggregateMonthToken(m[1])
	}
	return 0
}

func extractComparisonQuarter(q string) int {
	re := regexp.MustCompile(`(?i)Q\s*([1-4])|第?\s*([一二三四1234])\s*季度`)
	if m := re.FindStringSubmatch(q); len(m) == 3 {
		token := m[1]
		if token == "" {
			token = m[2]
		}
		switch token {
		case "1", "一":
			return 1
		case "2", "二":
			return 2
		case "3", "三":
			return 3
		case "4", "四":
			return 4
		}
	}
	return 0
}

func parseContractAggregateMonthToken(token string) int {
	token = strings.TrimSpace(token)
	switch token {
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
		n := mustAtoi(token)
		if n >= 1 && n <= 12 {
			return n
		}
		return 0
	}
}

func quarterPeriodRange(year, quarter int) (string, string) {
	startMonth := (quarter-1)*3 + 1
	return fmt.Sprintf("%04d-%02d", year, startMonth), fmt.Sprintf("%04d-%02d", year, startMonth+2)
}
