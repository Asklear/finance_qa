package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"financeqa/internal/accounting"
	dbpkg "financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/openitems"
)

type Engine struct {
	db                 *sql.DB
	dbPath             string
	Company            string
	available          []string
	calc               *accounting.Calculator
	dim                *dimensions.Manager
	ruleConfigProvider RuleConfigProvider

	cacheMu             sync.RWMutex
	latestAnchorCache   map[string]time.Time
	availablePeriod     map[string]string
	tableColumnCache    map[string]map[string]bool
	counterpartyEvCache map[string][]LedgerEvidence
	counterpartyNames   map[string][]string
	coreMetricCache     map[string]cachedUnifiedCoreMetrics
	hrBreakdownCache    map[string]Result
	branchTransferCache map[string]cachedBranchTransfer
	openItemSummary     map[string]openitems.Summary
}

type cachedUnifiedCoreMetrics struct {
	metrics *unifiedCoreMetrics
	sqls    []string
	logs    []string
}

type cachedBranchTransfer struct {
	total float64
	query string
	logs  []string
}

// EngineOption customizes Engine construction without changing the default constructor path.
type EngineOption func(*Engine)

// WithRuleConfigProvider injects the provider used to resolve query routing and rule-based behavior.
func WithRuleConfigProvider(provider RuleConfigProvider) EngineOption {
	return func(e *Engine) {
		if provider != nil {
			e.ruleConfigProvider = provider
		}
	}
}

func NewEngine(dbPath, company string, opts ...EngineOption) (*Engine, error) {
	if err := dbpkg.Bootstrap(context.Background(), dbPath); err != nil {
		return nil, fmt.Errorf("bootstrap db: %w", err)
	}
	db, err := dbpkg.Open(context.Background(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	available, _ := availableCompanies(db)
	dimRepo := dimensions.NewSQLiteRepository(db)
	dimMgr := dimensions.NewManager(dimRepo)
	resolvedCompany := ResolveCompany(company, available)
	calc := accounting.NewCalculator(db)
	if resolvedCompany != "" {
		if mapper, err := dimMgr.GetMapper(context.Background(), resolvedCompany); err == nil {
			calc.Mapper = mapper
		}
	}
	engine := &Engine{
		db:                  db,
		dbPath:              dbPath,
		Company:             resolvedCompany,
		available:           available,
		calc:                calc,
		dim:                 dimMgr,
		ruleConfigProvider:  defaultRuleConfigProviderInstance,
		latestAnchorCache:   map[string]time.Time{},
		availablePeriod:     map[string]string{},
		tableColumnCache:    map[string]map[string]bool{},
		counterpartyEvCache: map[string][]LedgerEvidence{},
		counterpartyNames:   map[string][]string{},
		coreMetricCache:     map[string]cachedUnifiedCoreMetrics{},
		hrBreakdownCache:    map[string]Result{},
		branchTransferCache: map[string]cachedBranchTransfer{},
		openItemSummary:     map[string]openitems.Summary{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(engine)
		}
	}
	return engine, nil
}

func (e *Engine) Close() error {
	if e.db == nil {
		return nil
	}
	return e.db.Close()
}

func (e *Engine) getLatestPeriodAnchor() time.Time {
	companyKey := strings.TrimSpace(e.Company)
	e.cacheMu.RLock()
	if cached, ok := e.latestAnchorCache[companyKey]; ok && !cached.IsZero() {
		e.cacheMu.RUnlock()
		return cached
	}
	e.cacheMu.RUnlock()

	candidates := []struct {
		sqlText string
		args    []any
		parser  func(any) (time.Time, bool)
	}{
		{
			sqlText: `SELECT MAX(voucher_date) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`,
			args:    []any{e.Company, e.Company},
			parser:  parseAnchorDateValue,
		},
		{
			sqlText: `SELECT MAX(transaction_date) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`,
			args:    []any{e.Company, e.Company},
			parser:  parseAnchorDateValue,
		},
		{
			sqlText: `SELECT MAX(period) FROM income_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`,
			args:    []any{e.Company, e.Company},
			parser:  parseAnchorMonthValue,
		},
		{
			sqlText: `SELECT MAX(period) FROM balance_detail WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`,
			args:    []any{e.Company, e.Company},
			parser:  parseAnchorMonthValue,
		},
		{
			sqlText: `SELECT MAX(year_month) FROM fin_fund_income`,
			args:    nil,
			parser:  parseAnchorMonthValue,
		},
		{
			sqlText: `SELECT MAX(year_month) FROM fin_fund_income_groups`,
			args:    nil,
			parser:  parseAnchorMonthValue,
		},
		{
			sqlText: `SELECT MAX(year_month) FROM fin_cost_settlements`,
			args:    nil,
			parser:  parseAnchorMonthValue,
		},
		{
			sqlText: `SELECT MAX(year_month) FROM fin_cost_settlement_groups`,
			args:    nil,
			parser:  parseAnchorMonthValue,
		},
	}

	candidateTimes := make([]time.Time, 0, len(candidates))
	for _, candidate := range candidates {
		var raw any
		if err := e.db.QueryRow(candidate.sqlText, candidate.args...).Scan(&raw); err != nil {
			continue
		}
		if t, ok := candidate.parser(raw); ok {
			candidateTimes = append(candidateTimes, t)
		}
	}
	best := selectLatestAnchorMonth(candidateTimes, time.Now())

	if !best.IsZero() {
		e.cacheMu.Lock()
		e.latestAnchorCache[companyKey] = best
		e.cacheMu.Unlock()
		return best
	}
	return time.Now()
}

func selectLatestAnchorMonth(candidates []time.Time, now time.Time) time.Time {
	nowMonth := startOfAnchorMonth(now)
	bestNonFuture := time.Time{}
	hasCandidate := false
	for _, candidate := range candidates {
		if candidate.IsZero() {
			continue
		}
		hasCandidate = true
		month := startOfAnchorMonth(candidate)
		if month.After(nowMonth) {
			continue
		}
		if bestNonFuture.IsZero() || month.After(bestNonFuture) {
			bestNonFuture = month
		}
	}
	if !bestNonFuture.IsZero() {
		return bestNonFuture
	}
	if hasCandidate {
		return nowMonth
	}
	return time.Time{}
}

func startOfAnchorMonth(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func capFutureAnchorMonth(anchor, now time.Time) time.Time {
	if anchor.IsZero() {
		return anchor
	}
	anchorMonth := startOfAnchorMonth(anchor)
	nowMonth := startOfAnchorMonth(now)
	if anchorMonth.After(nowMonth) {
		return nowMonth
	}
	return anchorMonth
}

func parseAnchorMonthValue(v any) (time.Time, bool) {
	switch raw := v.(type) {
	case nil:
		return time.Time{}, false
	case string:
		return parseAnchorMonthString(raw)
	case []byte:
		return parseAnchorMonthString(string(raw))
	default:
		return parseAnchorDateValue(v)
	}
}

func parseAnchorMonthString(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if len(raw) == len("2006-01") {
		if t, err := time.Parse("2006-01", raw); err == nil {
			return t, true
		}
	}
	return parseAnchorDateString(raw)
}
