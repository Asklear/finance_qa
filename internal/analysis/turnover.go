package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbpkg "financeqa/internal/db"
	"financeqa/internal/openitems"
)

type TurnoverSummary struct {
	Company                string  `json:"company"`
	Period                 string  `json:"period"`
	ReceivableTurnoverDays float64 `json:"receivableTurnoverDays"`
	PayableTurnoverDays    float64 `json:"payableTurnoverDays"`
	InventoryTurnoverDays  float64 `json:"inventoryTurnoverDays"`
	CashConversionCycle    float64 `json:"cashConversionCycle"`
}

type TurnoverEngine struct {
	db *sql.DB
}

func NewTurnoverEngine(dbPath string) *TurnoverEngine {
	db, err := dbpkg.Open(context.Background(), dbPath)
	if err != nil {
		return &TurnoverEngine{}
	}
	return &TurnoverEngine{db: db}
}

func (e *TurnoverEngine) Close() error {
	if e == nil || e.db == nil {
		return nil
	}
	return e.db.Close()
}

func (e *TurnoverEngine) Analyze(company, period string) (TurnoverSummary, error) {
	if e.db == nil {
		return TurnoverSummary{}, fmt.Errorf("db not available")
	}

	receivables, err := avgMonthlyBalance(e.db, company, period, "%应收账款%", "1122", openitems.Receivable)
	if err != nil {
		return TurnoverSummary{}, err
	}
	payables, err := avgMonthlyBalance(e.db, company, period, "%应付账款%", "2202", openitems.Payable)
	if err != nil {
		return TurnoverSummary{}, err
	}
	inventory, err := avgBalanceFromMonthBoundaries(e.db, company, period, "%存货%")
	if err != nil {
		return TurnoverSummary{}, err
	}
	revenue, err := statementAmount(e.db, company, period, "%营业收入%")
	if err != nil {
		return TurnoverSummary{}, err
	}
	cost, err := statementAmount(e.db, company, period, "%营业成本%")
	if err != nil {
		return TurnoverSummary{}, err
	}

	receivableDays := safeTurnoverDays(receivables, revenue)
	payableDays := safeTurnoverDays(payables, cost)
	inventoryDays := safeTurnoverDays(inventory, cost)

	return TurnoverSummary{
		Company:                company,
		Period:                 period,
		ReceivableTurnoverDays: receivableDays,
		PayableTurnoverDays:    payableDays,
		InventoryTurnoverDays:  inventoryDays,
		CashConversionCycle:    receivableDays + inventoryDays - payableDays,
	}, nil
}

func avgMonthlyBalance(db *sql.DB, company, period, accountLike, accountCodePrefix string, kind openitems.AccountKind) (float64, error) {
	summary, err := openitems.BuildSummary(context.Background(), db, openitems.Options{
		Company:           company,
		Period:            period,
		AccountCodePrefix: accountCodePrefix,
		Kind:              kind,
	})
	if err != nil {
		return 0, fmt.Errorf("query open item balance: %w", err)
	}
	if summary.HasData {
		return round2((summary.OpeningBalance + summary.ClosingBalance) / 2.0), nil
	}
	return avgBalanceFromMonthBoundaries(db, company, period, accountLike)
}

func avgBalanceFromMonthBoundaries(db *sql.DB, company, period, accountLike string) (float64, error) {
	prevPeriod, err := previousPeriod(period)
	if err != nil {
		return 0, err
	}
	var prevClosing float64
	var hasPrev bool
	if prevPeriod != "" {
		prevClosing, hasPrev, err = periodClosingBalance(db, company, prevPeriod, accountLike)
		if err != nil {
			return 0, err
		}
	}

	currentOpening, currentClosing, hasCurrent, err := currentPeriodBalance(db, company, period, accountLike)
	if err != nil {
		return 0, err
	}
	if !hasCurrent {
		return 0, nil
	}
	opening := currentOpening
	if hasPrev {
		opening = prevClosing
	}
	return round2((opening + currentClosing) / 2.0), nil
}

func currentPeriodBalance(db *sql.DB, company, period, accountLike string) (float64, float64, bool, error) {
	row := db.QueryRow(`
SELECT COALESCE(SUM(COALESCE(opening_balance, 0)), 0), COALESCE(SUM(COALESCE(closing_balance, 0)), 0), COUNT(1)
FROM balance_sheet
WHERE company = ?
  AND period = ?
  AND account_name LIKE ?
`, company, period, accountLike)
	var opening, closing float64
	var count int
	if err := row.Scan(&opening, &closing, &count); err != nil {
		return 0, 0, false, fmt.Errorf("query current period balance: %w", err)
	}
	return opening, closing, count > 0, nil
}

func periodClosingBalance(db *sql.DB, company, period, accountLike string) (float64, bool, error) {
	row := db.QueryRow(`
SELECT COALESCE(SUM(COALESCE(closing_balance, 0)), 0), COUNT(1)
FROM balance_sheet
WHERE company = ?
  AND period = ?
  AND account_name LIKE ?
`, company, period, accountLike)
	var closing float64
	var count int
	if err := row.Scan(&closing, &count); err != nil {
		return 0, false, fmt.Errorf("query prior closing balance: %w", err)
	}
	return closing, count > 0, nil
}

func statementAmount(db *sql.DB, company, period, itemLike string) (float64, error) {
	row := db.QueryRow(`
SELECT COALESCE(SUM(COALESCE(current_amount, 0)), 0)
FROM income_statement
WHERE company = ?
  AND period = ?
  AND item_name LIKE ?
`, company, period, itemLike)
	var v float64
	if err := row.Scan(&v); err != nil {
		return 0, fmt.Errorf("query statement amount: %w", err)
	}
	return v, nil
}

func safeTurnoverDays(balance, amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	return balance / amount * 365
}

func previousPeriod(period string) (string, error) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return "", fmt.Errorf("invalid period %q: %w", period, err)
	}
	return t.AddDate(0, -1, 0).Format("2006-01"), nil
}
