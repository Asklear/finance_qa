package query

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

func (e *Engine) resolveCumulativeRangeLowerBound(from, to string) (string, string) {
	boundary, boundaryLog := e.resolveRangeOpeningBoundary(from, to)
	year, month := parsePeriod(from)
	yearStart := fmt.Sprintf("%04d-01", year)
	if boundary < yearStart {
		return yearStart, fmt.Sprintf("[区间累计口径] lower_bound=%s reason=cross_year_reset boundary_note=%s", yearStart, boundaryLog)
	}
	if boundary == from && month != 1 {
		if yearStart < from {
			return yearStart, fmt.Sprintf("[区间累计口径] lower_bound=%s reason=year_start_fallback boundary_note=%s", yearStart, boundaryLog)
		}
	}
	return boundary, fmt.Sprintf("[区间累计口径] lower_bound=%s boundary_note=%s", boundary, boundaryLog)
}

func (e *Engine) resolveRangeOpeningBoundary(from, to string) (string, string) {
	defaultBoundary := from
	hasOpeningPeriod, err := e.tableHasColumn("balance_detail", "opening_period")
	if err != nil {
		return defaultBoundary, fmt.Sprintf("[区间校验] opening boundary fallback=%s reason=detect opening_period failed: %v", defaultBoundary, err)
	}
	if !hasOpeningPeriod {
		return defaultBoundary, fmt.Sprintf("[区间校验] opening boundary fallback=%s reason=balance_detail has no opening_period", defaultBoundary)
	}

	var opening sql.NullString
	err = e.db.QueryRow(`
SELECT MIN(opening_period)
FROM balance_detail
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period <= ?
  AND opening_period IS NOT NULL
  AND TRIM(opening_period) <> ''
`, e.Company, e.Company, to).Scan(&opening)
	if err != nil {
		return defaultBoundary, fmt.Sprintf("[区间校验] opening boundary fallback=%s reason=query opening_period failed: %v", defaultBoundary, err)
	}
	if !opening.Valid {
		return defaultBoundary, fmt.Sprintf("[区间校验] opening boundary fallback=%s reason=no opening_period data", defaultBoundary)
	}

	candidate := normalizeBoundaryPeriod(opening.String)
	if candidate == "" {
		return defaultBoundary, fmt.Sprintf("[区间校验] opening boundary fallback=%s reason=invalid opening_period(%s)", defaultBoundary, opening.String)
	}
	if candidate > from {
		return from, fmt.Sprintf("[区间校验] opening boundary adjusted=%s opening_period=%s", from, candidate)
	}
	return candidate, fmt.Sprintf("[区间校验] opening boundary=%s source=balance_detail.opening_period", candidate)
}

func normalizeBoundaryPeriod(v string) string {
	s := strings.TrimSpace(v)
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "/", "-")
	if len(s) == 6 && !strings.Contains(s, "-") {
		s = s[:4] + "-" + s[4:]
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return ""
	}
	year, errY := strconv.Atoi(parts[0])
	month, errM := strconv.Atoi(parts[1])
	if errY != nil || errM != nil || year <= 0 || month < 1 || month > 12 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d", year, month)
}

func (e *Engine) tableHasColumn(tableName, columnName string) (bool, error) {
	cols := e.tableColumns(tableName)
	if len(cols) == 0 {
		return false, nil
	}
	return cols[strings.ToLower(strings.TrimSpace(columnName))], nil
}
