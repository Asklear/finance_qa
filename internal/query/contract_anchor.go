package query

import (
	"strings"
	"time"
)

func (e *Engine) getLatestContractPeriodAnchor() time.Time {
	if !e.asOfAnchor.IsZero() {
		return e.asOfAnchor
	}
	cacheKey := strings.TrimSpace(e.Company) + "|contracts"
	e.cacheMu.RLock()
	if cached, ok := e.latestAnchorCache[cacheKey]; ok && !cached.IsZero() {
		e.cacheMu.RUnlock()
		return cached
	}
	e.cacheMu.RUnlock()

	candidates := []struct {
		sqlText string
		parser  func(any) (time.Time, bool)
	}{
		{
			sqlText: `SELECT MAX(year_month) FROM fin_fund_income`,
			parser:  parseAnchorMonthValue,
		},
		{
			sqlText: `SELECT MAX(year_month) FROM fin_fund_income_groups`,
			parser:  parseAnchorMonthValue,
		},
		{
			sqlText: `SELECT MAX(year_month) FROM fin_cost_settlements`,
			parser:  parseAnchorMonthValue,
		},
		{
			sqlText: `SELECT MAX(year_month) FROM fin_cost_settlement_groups`,
			parser:  parseAnchorMonthValue,
		},
	}

	candidateTimes := make([]time.Time, 0, len(candidates))
	for _, candidate := range candidates {
		var raw any
		if err := e.db.QueryRow(candidate.sqlText).Scan(&raw); err != nil {
			continue
		}
		if t, ok := candidate.parser(raw); ok {
			candidateTimes = append(candidateTimes, t)
		}
	}

	best := selectLatestAnchorMonth(candidateTimes, time.Now())
	if best.IsZero() {
		best = e.getLatestPeriodAnchor()
	}
	if !best.IsZero() {
		e.cacheMu.Lock()
		e.latestAnchorCache[cacheKey] = best
		e.cacheMu.Unlock()
	}
	return best
}
