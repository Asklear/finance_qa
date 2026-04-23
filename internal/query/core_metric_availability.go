package query

import (
	"database/sql"
	"strings"
)

func (e *Engine) latestAvailableFinancialPeriod() string {
	companyKey := strings.TrimSpace(e.Company)
	e.cacheMu.RLock()
	if cached := strings.TrimSpace(e.availablePeriod[companyKey]); cached != "" {
		e.cacheMu.RUnlock()
		return cached
	}
	e.cacheMu.RUnlock()

	queries := []string{
		`SELECT MAX(period) FROM income_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`,
		`SELECT MAX(period) FROM balance_detail WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`,
		`SELECT MAX(period) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`,
		`SELECT MAX(SUBSTR(transaction_date, 1, 7)) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`,
	}
	best := ""
	for _, sqlTxt := range queries {
		var period sql.NullString
		if err := e.db.QueryRow(sqlTxt, e.Company, e.Company).Scan(&period); err != nil {
			continue
		}
		candidate := strings.TrimSpace(period.String)
		if candidate != "" && candidate > best {
			best = candidate
		}
	}
	if best != "" {
		e.cacheMu.Lock()
		e.availablePeriod[companyKey] = best
		e.cacheMu.Unlock()
		return best
	}
	anchor := e.getLatestPeriodAnchor()
	if anchor.IsZero() {
		return ""
	}
	best = anchor.Format("2006-01")
	e.cacheMu.Lock()
	e.availablePeriod[companyKey] = best
	e.cacheMu.Unlock()
	return best
}
