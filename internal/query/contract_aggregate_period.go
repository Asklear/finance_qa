package query

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

type contractAggregatePeriodCoverage struct {
	RequestedFrom string
	RequestedTo   string
	ActualFrom    string
	ActualTo      string
	Note          string
}

func (c contractAggregatePeriodCoverage) Adjusted() bool {
	return strings.TrimSpace(c.RequestedFrom) != strings.TrimSpace(c.ActualFrom) ||
		strings.TrimSpace(c.RequestedTo) != strings.TrimSpace(c.ActualTo)
}

func (e *Engine) resolveContractAggregatePeriodCoverage(spec QuerySpec, requestedMetrics []string, like string) contractAggregatePeriodCoverage {
	coverage := contractAggregatePeriodCoverage{
		RequestedFrom: strings.TrimSpace(spec.PeriodFrom),
		RequestedTo:   strings.TrimSpace(spec.PeriodTo),
		ActualFrom:    strings.TrimSpace(spec.PeriodFrom),
		ActualTo:      strings.TrimSpace(spec.PeriodTo),
	}
	if coverage.ActualFrom == "" || coverage.ActualTo == "" {
		return coverage
	}

	specs := contractFinanceSpecsForRequestedMetrics(requestedMetrics)
	if len(specs) == 0 {
		return coverage
	}
	if e.contractFinanceHasRowsForAllSpecs(specs, coverage.RequestedFrom, coverage.RequestedTo, like) {
		return coverage
	}
	if !contractAggregateCanUseLatestAvailablePeriod(spec.OriginalQuestion) {
		return coverage
	}

	latest := e.latestContractFinancePeriodForSpecs(specs)
	if latest == "" {
		return coverage
	}
	fallbackFrom := contractAggregateFallbackPeriodFrom(spec.OriginalQuestion, latest)
	if fallbackFrom == "" {
		fallbackFrom = latest
	}
	if !e.contractFinanceHasRowsForAllSpecs(specs, fallbackFrom, latest, like) {
		fallbackFrom = latest
		if !e.contractFinanceHasRowsForAllSpecs(specs, fallbackFrom, latest, like) {
			return coverage
		}
	}
	coverage.ActualFrom = fallbackFrom
	coverage.ActualTo = latest
	if coverage.Adjusted() {
		coverage.Note = fmt.Sprintf("[项目口径覆盖] requested=%s actual=%s reason=请求期间无收入/成本表记录，改用项目表最新可用期间",
			displayPeriod(coverage.RequestedFrom, coverage.RequestedTo),
			displayPeriod(coverage.ActualFrom, coverage.ActualTo))
	}
	return coverage
}

func contractFinanceSpecsForRequestedMetrics(requestedMetrics []string) []contractFinanceTotalsSpec {
	specs := make([]contractFinanceTotalsSpec, 0, 2)
	if contractAggregateNeedsRevenueData(requestedMetrics) {
		specs = append(specs, fundIncomeTotalsSpec())
	}
	if contractAggregateNeedsCostData(requestedMetrics) {
		specs = append(specs, costSettlementTotalsSpec())
	}
	return specs
}

func (e *Engine) contractFinanceHasRowsForAllSpecs(specs []contractFinanceTotalsSpec, from, to, like string) bool {
	for _, spec := range specs {
		totals, err := e.collectContractFinanceTotals(context.Background(), spec, from, to, like)
		if err != nil || totals.RowCount == 0 {
			return false
		}
	}
	return len(specs) > 0
}

func (e *Engine) latestContractFinancePeriodForSpecs(specs []contractFinanceTotalsSpec) string {
	latest := ""
	for _, spec := range specs {
		candidate := e.latestContractFinancePeriod(spec)
		if candidate == "" {
			return ""
		}
		if latest == "" || candidate < latest {
			latest = candidate
		}
	}
	return latest
}

func (e *Engine) latestContractFinancePeriod(spec contractFinanceTotalsSpec) string {
	best := e.latestContractFinancePeriodFromTable(spec.DirectTable, spec.MovementColumn)
	if e.hasContractFinanceGroupTables(spec) {
		best = maxPeriodString(best, e.latestContractFinancePeriodFromTable(spec.GroupTable, spec.MovementColumn))
	}
	return best
}

func (e *Engine) latestContractFinancePeriodFromTable(tableName, movementColumn string) string {
	cols := e.tableColumns(tableName)
	if !cols["year_month"] {
		return ""
	}
	amountPredicates := make([]string, 0, 3)
	for _, col := range []string{"settlement_amount", movementColumn, "invoice_amount"} {
		col = strings.TrimSpace(col)
		if col != "" && cols[col] {
			amountPredicates = append(amountPredicates, fmt.Sprintf("COALESCE(%s, 0) <> 0", col))
		}
	}
	where := "COALESCE(TRIM(year_month), '') <> ''"
	if len(amountPredicates) > 0 {
		where += " AND (" + strings.Join(amountPredicates, " OR ") + ")"
	}
	sqlText := fmt.Sprintf("SELECT MAX(year_month) FROM %s WHERE %s", tableName, where)
	var period sql.NullString
	if err := e.db.QueryRow(sqlText).Scan(&period); err != nil {
		return ""
	}
	return strings.TrimSpace(period.String)
}

func contractAggregateCanUseLatestAvailablePeriod(question string) bool {
	q := strings.TrimSpace(question)
	if q == "" {
		return true
	}
	if contractAggregateHasRelativeCurrentPeriodToken(q) {
		return true
	}
	return !contractAggregateHasAbsolutePeriodToken(q)
}

func contractAggregateHasRelativeCurrentPeriodToken(q string) bool {
	return containsAny(q, []string{
		"现在", "当前", "目前", "最近", "最新", "至今", "截至", "截止", "累计",
		"本月", "这个月", "当月", "上个月", "上月", "上一个完整自然月",
		"这季度", "这个季度", "本季度",
	})
}

func contractAggregateHasAbsolutePeriodToken(q string) bool {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`20\d{2}\s*年`),
		regexp.MustCompile(`(?i)(20\d{2}|\d{2})?\s*年?\s*Q\s*[1-4]`),
		regexp.MustCompile(`第?\s*[一二三四1234]\s*季度`),
		regexp.MustCompile(`(?:20\d{2}|\d{2})?\s*年?\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})\s*月`),
		regexp.MustCompile(`上半年|下半年|全年|全年度|整年|年度`),
	}
	for _, pattern := range patterns {
		if pattern.MatchString(q) {
			return true
		}
	}
	return false
}

func contractAggregateFallbackPeriodFrom(question, latest string) string {
	if strings.TrimSpace(latest) == "" {
		return ""
	}
	if containsAny(question, []string{"本月", "这个月", "当月", "上个月", "上月", "上一个完整自然月", "最新月份", "最新的月份"}) {
		return latest
	}
	year, month := parsePeriod(latest)
	if year == 0 || month == 0 {
		return latest
	}
	if containsAny(question, []string{"这季度", "这个季度", "本季度", "季度"}) || month%3 == 0 {
		startMonth := ((month - 1) / 3 * 3) + 1
		return fmt.Sprintf("%04d-%02d", year, startMonth)
	}
	return fmt.Sprintf("%04d-01", year)
}

func timeScopeFromPeriodRange(from, to string) TimeScope {
	fromYear, fromMonth := parsePeriod(from)
	toYear, toMonth := parsePeriod(to)
	if fromYear == 0 || toYear == 0 {
		return TimeScopeMonth
	}
	if from == to {
		return TimeScopeMonth
	}
	if fromYear == toYear {
		switch {
		case fromMonth == 1 && toMonth == 12:
			return TimeScopeYearFull
		case fromMonth == 1 && toMonth == 6:
			return TimeScopeHalfYear
		case fromMonth == 7 && toMonth == 12:
			return TimeScopeHalfYear
		case toMonth-fromMonth == 2 && (fromMonth == 1 || fromMonth == 4 || fromMonth == 7 || fromMonth == 10):
			return TimeScopeQuarter
		case fromMonth == 1:
			return TimeScopeYearToDate
		}
	}
	return TimeScopeCustom
}
