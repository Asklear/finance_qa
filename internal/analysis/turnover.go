package analysis

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
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
	db, err := sql.Open("sqlite", dbPath)
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
		return TurnoverSummary{}, fmt.Errorf("sqlite db not available")
	}

	receivables, err := avgClosingBalance(e.db, company, period, "%应收账款%")
	if err != nil {
		return TurnoverSummary{}, err
	}
	payables, err := avgClosingBalance(e.db, company, period, "%应付账款%")
	if err != nil {
		return TurnoverSummary{}, err
	}
	inventory, err := avgClosingBalance(e.db, company, period, "%存货%")
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

func avgClosingBalance(db *sql.DB, company, period, accountLike string) (float64, error) {
	row := db.QueryRow(`
SELECT COALESCE(AVG((COALESCE(opening_balance, 0) + COALESCE(closing_balance, 0)) / 2.0), 0)
FROM balance_sheet
WHERE company = ?
  AND period = ?
  AND account_name LIKE ?
`, company, period, accountLike)
	var v float64
	if err := row.Scan(&v); err != nil {
		return 0, fmt.Errorf("query avg balance: %w", err)
	}
	return v, nil
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
