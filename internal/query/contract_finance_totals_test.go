package query

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCollectContractFinanceTotalsSupportsRevenueAndCostGroupRows(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "contract-finance-totals.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE fin_contracts (
			contract_id TEXT PRIMARY KEY,
			customer_name TEXT,
			contract_content TEXT
		)`,
		`CREATE TABLE fin_fund_income (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT,
			year_month TEXT,
			settlement_amount REAL,
			received_amount REAL,
			invoice_amount REAL
		)`,
		`CREATE TABLE fin_fund_income_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_name TEXT,
			year_month TEXT,
			settlement_amount REAL,
			received_amount REAL,
			invoice_amount REAL
		)`,
		`CREATE TABLE fin_fund_income_group_members (
			group_id INTEGER,
			contract_id TEXT
		)`,
		`CREATE TABLE fin_cost_settlements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT,
			year_month TEXT,
			settlement_amount REAL,
			paid_amount REAL,
			invoice_amount REAL
		)`,
		`CREATE TABLE fin_cost_settlement_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_name TEXT,
			year_month TEXT,
			settlement_amount REAL,
			paid_amount REAL,
			invoice_amount REAL
		)`,
		`CREATE TABLE fin_cost_settlement_group_members (
			group_id INTEGER,
			contract_id TEXT
		)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('C1', '客户A', '项目A'),
			('C2', '客户B', '项目B'),
			('C3', '供应商A', '项目C')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			('C1', '2026-03', 100, 70, 90)`,
		`INSERT INTO fin_fund_income_groups(id, customer_name, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			(1, '客户A', '2026-03', 200, 150, 180)`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id) VALUES
			(1, 'C1'),
			(1, 'C2')`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, settlement_amount, paid_amount, invoice_amount) VALUES
			('C3', '2026-03', 80, 30, 60)`,
		`INSERT INTO fin_cost_settlement_groups(id, customer_name, year_month, settlement_amount, paid_amount, invoice_amount) VALUES
			(1, '供应商A', '2026-03', 120, 100, 110)`,
		`INSERT INTO fin_cost_settlement_group_members(group_id, contract_id) VALUES
			(1, 'C3')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt: %v", err)
		}
	}

	engine := &Engine{
		db:                db,
		dbPath:            dbPath,
		tableColumnCache:  map[string]map[string]bool{},
		latestAnchorCache: map[string]time.Time{},
		availablePeriod:   map[string]string{},
	}

	revenue, err := engine.collectContractFinanceTotals(context.Background(), contractFinanceTotalsSpec{
		DirectTable:      "fin_fund_income",
		GroupTable:       "fin_fund_income_groups",
		GroupMemberTable: "fin_fund_income_group_members",
		MovementColumn:   "received_amount",
	}, "2026-03", "2026-03", "")
	if err != nil {
		t.Fatalf("collect revenue totals: %v", err)
	}
	if revenue.Settlement != 300 || revenue.Movement != 220 || revenue.Invoice != 270 {
		t.Fatalf("revenue totals = %+v, want settlement=300 movement=220 invoice=270", revenue)
	}
	if revenue.SettlementOpen != 80 || revenue.InvoiceOpen != 50 {
		t.Fatalf("revenue open totals = %+v, want settlement_open=80 invoice_open=50", revenue)
	}
	if revenue.RowCount != 2 || revenue.MonthCount != 1 || revenue.ContractCount != 2 {
		t.Fatalf("revenue coverage = %+v, want row=2 month=1 contract=2", revenue)
	}

	cost, err := engine.collectContractFinanceTotals(context.Background(), contractFinanceTotalsSpec{
		DirectTable:      "fin_cost_settlements",
		GroupTable:       "fin_cost_settlement_groups",
		GroupMemberTable: "fin_cost_settlement_group_members",
		MovementColumn:   "paid_amount",
	}, "2026-03", "2026-03", "")
	if err != nil {
		t.Fatalf("collect cost totals: %v", err)
	}
	if cost.Settlement != 200 || cost.Movement != 130 || cost.Invoice != 170 {
		t.Fatalf("cost totals = %+v, want settlement=200 movement=130 invoice=170", cost)
	}
	if cost.SettlementOpen != 70 || cost.InvoiceOpen != 40 {
		t.Fatalf("cost open totals = %+v, want settlement_open=70 invoice_open=40", cost)
	}
	if cost.RowCount != 2 || cost.MonthCount != 1 || cost.ContractCount != 1 {
		t.Fatalf("cost coverage = %+v, want row=2 month=1 contract=1", cost)
	}
}
