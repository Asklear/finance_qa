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
	db        *sql.DB
	dbPath    string
	Company   string
	available []string
	calc      *accounting.Calculator
	dim       *dimensions.Manager

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

func NewEngine(dbPath, company string) (*Engine, error) {
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
	return &Engine{
		db:                  db,
		dbPath:              dbPath,
		Company:             resolvedCompany,
		available:           available,
		calc:                calc,
		dim:                 dimMgr,
		latestAnchorCache:   map[string]time.Time{},
		availablePeriod:     map[string]string{},
		tableColumnCache:    map[string]map[string]bool{},
		counterpartyEvCache: map[string][]LedgerEvidence{},
		counterpartyNames:   map[string][]string{},
		coreMetricCache:     map[string]cachedUnifiedCoreMetrics{},
		hrBreakdownCache:    map[string]Result{},
		branchTransferCache: map[string]cachedBranchTransfer{},
		openItemSummary:     map[string]openitems.Summary{},
	}, nil
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

	var maxDate any
	if err := e.db.QueryRow(`SELECT MAX(voucher_date) FROM journal WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')`, e.Company, e.Company).Scan(&maxDate); err != nil {
		return time.Now()
	}
	if t, ok := parseAnchorDateValue(maxDate); ok {
		e.cacheMu.Lock()
		e.latestAnchorCache[companyKey] = t
		e.cacheMu.Unlock()
		return t
	}
	return time.Now()
}
