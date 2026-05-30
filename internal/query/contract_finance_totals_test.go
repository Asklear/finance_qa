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

func TestCollectContractInvoiceOpenItemsSupportsDirectTablesWithoutGroupTables(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "contract-finance-direct-open-items.sqlite")
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
		`CREATE TABLE fin_cost_settlements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT,
			year_month TEXT,
			settlement_amount REAL,
			paid_amount REAL,
			invoice_amount REAL
		)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('R1', '客户A', '收入项目A'),
			('S1', '供应商A', '成本项目A')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			('R1', '2026-04', 100, 40, 90)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, settlement_amount, paid_amount, invoice_amount) VALUES
			('S1', '2026-04', 100, 30, 80)`,
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

	revenueItems, err := engine.collectRevenueInvoiceOpenItems("2026-04", "2026-04", "")
	if err != nil {
		t.Fatalf("collect revenue invoice open items: %v", err)
	}
	if len(revenueItems) != 1 || revenueItems[0].OpenAmount != 50 {
		t.Fatalf("revenue invoice open items = %#v, want one direct item with open 50", revenueItems)
	}

	costItems, err := engine.collectCostInvoiceOpenItems("2026-04", "2026-04", "")
	if err != nil {
		t.Fatalf("collect cost invoice open items: %v", err)
	}
	if len(costItems) != 1 || costItems[0].OpenAmount != 50 {
		t.Fatalf("cost invoice open items = %#v, want one direct item with open 50", costItems)
	}
}

func TestCollectFundIncomeTotalsAppliesInvoiceOpenOffsetWithoutChangingReceipts(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "fund-income-invoice-open-offset.sqlite")
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
			invoice_amount REAL,
			invoice_open_offset_amount REAL,
			invoice_open_offset_reason TEXT
		)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('B1', '倍壮（上海）信息技术有限公司', '信息服务协议')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount, invoice_open_offset_amount, invoice_open_offset_reason) VALUES
			('B1', '2026-01', 8854.25, 8854.25, 11854.25, 3000, '3000元2025年11月已经收到但当时未开票')`,
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

	totals, err := engine.collectFundIncomeTotals(context.Background(), "2026-01", "2026-01", "")
	if err != nil {
		t.Fatalf("collect fund income totals: %v", err)
	}
	if got, want := round2(totals.Received), 8854.25; got != want {
		t.Fatalf("received = %.2f, want unchanged actual receipt %.2f", got, want)
	}
	if got := round2(totals.InvoiceOpen); got != 0 {
		t.Fatalf("invoice open = %.2f, want offset to clear prepaid-before-invoice amount", got)
	}

	items, err := engine.collectRevenueInvoiceOpenItems("2026-01", "2026-01", "")
	if err != nil {
		t.Fatalf("collect invoice open items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("invoice open items = %#v, want none after invoice open offset", items)
	}
}

func TestCollectFundIncomeTotalsAttributesMergedGroupAmountToOwnerContract(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "contract-finance-owner-row.sqlite")
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
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			settlement_amount REAL,
			received_amount REAL,
			invoice_amount REAL
		)`,
		`CREATE TABLE fin_fund_income_group_members (
			group_id INTEGER,
			contract_id TEXT,
			source_row_number INTEGER
		)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('C048', '四川其妙科技有限公司', '行业商品数据采购合同-A01'),
			('C049', '四川其妙科技有限公司', '行业商品数据采购合同-A02'),
			('C050', '四川其妙科技有限公司', '行业商品数据采购合同-A03')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			('C048', '2026-03', 1604085.34, 0, 0),
			('C049', '2026-03', 583271.29, 0, 0)`,
		`INSERT INTO fin_fund_income_groups(id, customer_name, year_month, source_start_row, source_end_row, merge_range, settlement_amount, received_amount, invoice_amount) VALUES
			(287, '四川其妙科技有限公司', '2026-01', 3, 5, 'J3:J5', 883796.76, 0, 1668149.01),
			(288, '四川其妙科技有限公司', '2026-02', 3, 5, 'O3:O5', 1189200.60, 0, 1895935.02)`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number) VALUES
			(287, 'C048', 3), (287, 'C049', 4), (287, 'C050', 5),
			(288, 'C048', 3), (288, 'C049', 4), (288, 'C050', 5)`,
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

	a01, err := engine.collectFundIncomeTotals(context.Background(), "2026-01", "2026-03", "%行业商品数据采购合同-A01%")
	if err != nil {
		t.Fatalf("collect A01 income totals: %v", err)
	}
	if got, want := round2(a01.Settlement), 3677082.70; got != want {
		t.Fatalf("A01 settlement = %.2f, want %.2f", got, want)
	}
	if a01.RowCount != 3 || a01.MonthCount != 3 || a01.ContractCount != 1 {
		t.Fatalf("A01 coverage = %+v, want row=3 month=3 contract=1", a01)
	}
	if got := round2(a01.Invoice); got != 0 {
		t.Fatalf("A01 invoice = %.2f, want 0 because group invoice covers A01/A02/A03 and is not contract-attributable", got)
	}
	if got, want := round2(a01.UnattributedInvoice), 3564084.03; got != want {
		t.Fatalf("A01 unattributed invoice = %.2f, want merged group invoice %.2f", got, want)
	}
	if got, want := contractFinanceTestContents(a01.UnattributedInvoiceContracts), []string{
		"行业商品数据采购合同-A01",
		"行业商品数据采购合同-A02",
		"行业商品数据采购合同-A03",
	}; !stringSlicesEqual(got, want) {
		t.Fatalf("A01 unattributed invoice contracts = %#v, want %#v", got, want)
	}

	a02, err := engine.collectFundIncomeTotals(context.Background(), "2026-01", "2026-03", "%行业商品数据采购合同-A02%")
	if err != nil {
		t.Fatalf("collect A02 income totals: %v", err)
	}
	if got, want := round2(a02.Settlement), 583271.29; got != want {
		t.Fatalf("A02 settlement = %.2f, want only direct row %.2f", got, want)
	}
	if a02.RowCount != 1 || a02.MonthCount != 1 || a02.ContractCount != 1 {
		t.Fatalf("A02 coverage = %+v, want row=1 month=1 contract=1", a02)
	}

	customer, err := engine.collectFundIncomeTotals(context.Background(), "2026-01", "2026-03", "%四川其妙%")
	if err != nil {
		t.Fatalf("collect customer income totals: %v", err)
	}
	if got, want := round2(customer.Invoice), 3564084.03; got != want {
		t.Fatalf("customer invoice = %.2f, want merged group invoice %.2f", got, want)
	}
	if got := round2(customer.UnattributedInvoice); got != 0 {
		t.Fatalf("customer unattributed invoice = %.2f, want 0 because customer-level query can include merged invoice", got)
	}
}

func TestCollectFundIncomeTotalsNetsMergedInvoiceAgainstMemberReceipts(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "fund-income-merged-invoice-receipts.sqlite")
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
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			settlement_amount REAL,
			received_amount REAL,
			invoice_amount REAL
		)`,
		`CREATE TABLE fin_fund_income_group_members (
			group_id INTEGER,
			contract_id TEXT,
			source_row_number INTEGER
		)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('C048', '四川其妙科技有限公司', '行业商品数据采购合同-A01'),
			('C049', '四川其妙科技有限公司', '行业商品数据采购合同-A02'),
			('C050', '四川其妙科技有限公司', '行业商品数据采购合同-A03')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			('C049', '2026-01', 541382.41, 541382.41, 0),
			('C050', '2026-01', 242969.84, 242969.84, 0),
			('C049', '2026-02', 596381.74, 596381.74, 0),
			('C050', '2026-02', 110352.68, 110352.68, 0)`,
		`INSERT INTO fin_fund_income_groups(id, customer_name, year_month, source_start_row, source_end_row, merge_range, settlement_amount, received_amount, invoice_amount) VALUES
			(327, '四川其妙科技有限公司', '2026-01', 3, 5, 'J3:J5', 883796.76, 883796.76, 1668149.01),
			(328, '四川其妙科技有限公司', '2026-02', 3, 5, 'O3:O5', 1189200.60, 1189200.60, 1895935.02)`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number) VALUES
			(327, 'C048', 3), (327, 'C049', 4), (327, 'C050', 5),
			(328, 'C048', 3), (328, 'C049', 4), (328, 'C050', 5)`,
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

	totals, err := engine.collectFundIncomeTotals(context.Background(), "2026-01", "2026-02", "")
	if err != nil {
		t.Fatalf("collect fund income totals: %v", err)
	}
	if got, want := round2(totals.Invoice), 3564084.03; got != want {
		t.Fatalf("invoice = %.2f, want %.2f", got, want)
	}
	if got, want := round2(totals.Received), 3564084.03; got != want {
		t.Fatalf("received = %.2f, want %.2f", got, want)
	}
	if got := round2(totals.InvoiceOpen); got != 0 {
		t.Fatalf("invoice open = %.2f, want 0 after member receipts offset merged invoice", got)
	}

	items, err := engine.collectRevenueInvoiceOpenItems("2026-01", "2026-02", "")
	if err != nil {
		t.Fatalf("collect invoice open items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("invoice open items = %#v, want none after member receipts offset merged invoice", items)
	}
}

func TestCollectContractFinanceTotalsNetsMergedAmountsAgainstMemberMovements(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "contract-finance-merged-open-net.sqlite")
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
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			settlement_amount REAL,
			received_amount REAL,
			invoice_amount REAL
		)`,
		`CREATE TABLE fin_fund_income_group_members (
			group_id INTEGER,
			contract_id TEXT,
			source_row_number INTEGER
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
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			settlement_amount REAL,
			paid_amount REAL,
			invoice_amount REAL
		)`,
		`CREATE TABLE fin_cost_settlement_group_members (
			group_id INTEGER,
			contract_id TEXT,
			source_row_number INTEGER
		)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('R1', '客户A', '收入项目A'),
			('R2', '客户A', '收入项目B'),
			('S1', '供应商A', '成本项目A'),
			('S2', '供应商A', '成本项目B')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			('R2', '2026-04', 0, 600, 0)`,
		`INSERT INTO fin_fund_income_groups(id, customer_name, year_month, source_start_row, source_end_row, merge_range, settlement_amount, received_amount, invoice_amount) VALUES
			(1, '客户A', '2026-04', 3, 4, 'H3:H4', 1000, 400, 0)`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number) VALUES
			(1, 'R1', 3), (1, 'R2', 4)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, settlement_amount, paid_amount, invoice_amount) VALUES
			('S2', '2026-04', 0, 600, 0)`,
		`INSERT INTO fin_cost_settlement_groups(id, customer_name, year_month, source_start_row, source_end_row, merge_range, settlement_amount, paid_amount, invoice_amount) VALUES
			(1, '供应商A', '2026-04', 3, 4, 'I3:I4', 1000, 400, 1000)`,
		`INSERT INTO fin_cost_settlement_group_members(group_id, contract_id, source_row_number) VALUES
			(1, 'S1', 3), (1, 'S2', 4)`,
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

	revenue, err := engine.collectFundIncomeTotals(context.Background(), "2026-04", "2026-04", "")
	if err != nil {
		t.Fatalf("collect revenue totals: %v", err)
	}
	if got := round2(revenue.Receivable); got != 0 {
		t.Fatalf("revenue settlement open = %.2f, want 0 after member receipts offset merged settlement", got)
	}

	cost, err := engine.collectCostSettlementTotals(context.Background(), "2026-04", "2026-04", "")
	if err != nil {
		t.Fatalf("collect cost totals: %v", err)
	}
	if got := round2(cost.Payable); got != 0 {
		t.Fatalf("cost payable = %.2f, want 0 after member payments offset merged settlement", got)
	}
	if got := round2(cost.InvoiceOpen); got != 0 {
		t.Fatalf("cost invoice open = %.2f, want 0 after member payments offset merged invoice", got)
	}

	costItems, err := engine.collectCostInvoiceOpenItems("2026-04", "2026-04", "")
	if err != nil {
		t.Fatalf("collect cost invoice open items: %v", err)
	}
	if len(costItems) != 0 {
		t.Fatalf("cost invoice open items = %#v, want none after member payments offset merged invoice", costItems)
	}
}

func contractFinanceTestContents(rows []contractDimensionRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ContractContent)
	}
	return out
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestCollectRevenueItemsAttributesMergedAmountsToOwnerRow(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "revenue-items-owner.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, settlement_amount REAL, received_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_fund_income_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, customer_name TEXT, year_month TEXT, source_start_row INTEGER, source_end_row INTEGER, settlement_amount REAL, received_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_fund_income_group_members (group_id INTEGER, contract_id TEXT, source_row_number INTEGER)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('C1', '四川其妙科技有限公司', '行业商品数据采购合同-A01'),
			('C2', '四川其妙科技有限公司', '行业商品数据采购合同-A02')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			('C1', '2026-03', 1600, 0, 0)`,
		`INSERT INTO fin_fund_income_groups(id, customer_name, year_month, source_start_row, source_end_row, settlement_amount, received_amount, invoice_amount) VALUES
			(1, '四川其妙科技有限公司', '2026-01', 3, 4, 900, 900, 1200)`,
		`INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number) VALUES
			(1, 'C1', 3), (1, 'C2', 4)`,
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
	items, err := engine.collectRevenueItems("2026-01", "2026-03", "%四川其妙%")
	if err != nil {
		t.Fatalf("collect revenue items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v, want one owner-row item", items)
	}
	if got, want := items[0].ContractContent, "行业商品数据采购合同-A01"; got != want {
		t.Fatalf("contract_content = %q, want %q", got, want)
	}
	if got, want := items[0].SettlementAmount, float64(2500); got != want {
		t.Fatalf("settlement = %.2f, want %.2f; items=%#v", got, want, items)
	}
	if got, want := items[0].ReceivedAmount, float64(900); got != want {
		t.Fatalf("received = %.2f, want %.2f; items=%#v", got, want, items)
	}
	if got := items[0].InvoiceAmount; got != 0 {
		t.Fatalf("invoice = %.2f, want 0 because merged invoice covers multiple contracts", got)
	}
}

func TestResolveContractSubjectKeepsContractSuffixWhenContractWordMissing(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "contract-subject-suffix.sqlite")
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
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('C048', '四川其妙科技有限公司', '行业商品数据采购合同-A01'),
			('C049', '四川其妙科技有限公司', '行业商品数据采购合同-A02'),
			('C050', '四川其妙科技有限公司', '行业商品数据采购合同-A03')`,
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

	entity := engine.resolveContractSubject("四川其妙的行业商品数据采购-A01 的 26Q1 结算金额是多少？", "")
	if entity != "行业商品数据采购合同-A01" {
		t.Fatalf("resolved entity = %q, want 行业商品数据采购合同-A01", entity)
	}

	contracts := engine.queryMatchingContracts(entity)
	if len(contracts) != 1 || contracts[0].ContractID != "C048" {
		t.Fatalf("matching contracts = %#v, want only C048", contracts)
	}
}

func TestCollectContractDimensionSummaryScopesContractContentByMentionedCustomer(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "contract-content-customer-scope.sqlite")
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
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('C001', '辽宁金程信息科技有限公司', '行业商品数据采购合同-A01'),
			('C048', '四川其妙科技有限公司', '行业商品数据采购合同-A01'),
			('C049', '四川其妙科技有限公司', '行业商品数据采购合同-A02')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			('C048', '2026-03', 1604085.34, 0, 0)`,
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

	question := "四川其妙的行业商品数据采购-A01 的 26Q1 结算金额是多少？"
	if got := engine.mentionedContractCustomers(question); len(got) != 1 || got[0] != "四川其妙科技有限公司" {
		t.Fatalf("mentioned customers = %#v, want 四川其妙科技有限公司", got)
	}
	summary, err := engine.collectContractDimensionSummaryForPeriod(question, "", "2026-01", "2026-03")
	if err != nil {
		t.Fatalf("collect summary: %v", err)
	}
	if got := summary.Data["contract_count"]; got != 1 {
		t.Fatalf("contract_count = %v, want 1; contracts=%#v", got, summary.Contracts)
	}
	if len(summary.Contracts) != 1 || summary.Contracts[0]["contract_id"] != "C048" {
		t.Fatalf("contracts = %#v, want only C048", summary.Contracts)
	}
}

func TestCompoundRevenueUnpaidNetsGroupedOverReceipts(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "compound-revenue-unpaid-net.sqlite")
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
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
			('C1', '四川其妙科技有限公司', '行业商品数据服务')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			('C1', '2026-01', 500, 100, 100)`,
		`INSERT INTO fin_fund_income_groups(id, customer_name, year_month, settlement_amount, received_amount, invoice_amount) VALUES
			(1, '四川其妙科技有限公司', '2026-02', 300, 500, 500)`,
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
	metrics := []compoundSourceMetric{{Key: "contract_revenue.unpaid", Label: "未付款", Source: compoundSourceContractRevenue}}
	values, rows, err := engine.collectCompoundMetricValues(context.Background(), metrics, "2026-01", "2026-03", "%四川其妙%")
	if err != nil {
		t.Fatalf("collect compound values: %v", err)
	}
	if got := rows[compoundSourceContractRevenue]; got != 2 {
		t.Fatalf("contract revenue row count = %d, want 2", got)
	}
	if got := anyToFloat64(values["contract_revenue.unpaid"]); got != 200 {
		t.Fatalf("compound revenue unpaid = %.2f, want net unpaid 200.00", got)
	}
}
