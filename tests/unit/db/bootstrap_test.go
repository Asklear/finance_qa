package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"financeqa/internal/db"
)

func TestBootstrapInitializesTypeScriptCompatibleSchema(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "bootstrap.sqlite")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	mustHaveTables := []string{
		"balance_sheet",
		"income_statement",
		"balance_detail",
		"journal",
		"bank_statement",
		"table_idempotency_policies",
		"dimensions",
		"dimension_members",
		"mapping_rules",
	}

	for _, table := range mustHaveTables {
		table := table
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			var count int
			err := sqlDB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&count)
			if err != nil {
				t.Fatalf("query sqlite_master: %v", err)
			}
			if count != 1 {
				t.Fatalf("expected table %q to exist", table)
			}
		})
	}
}

func TestBootstrapCreatesKnownIndex(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "bootstrap-index.sqlite")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var count int
	err = sqlDB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='index' AND name = 'idx_journal_date'`).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master index: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected index idx_journal_date to exist")
	}
}

func TestBootstrapSeedsIdempotencyPolicies(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "bootstrap-policy.sqlite")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var count int
	err = sqlDB.QueryRow(`SELECT COUNT(1) FROM table_idempotency_policies WHERE enabled = 1`).Scan(&count)
	if err != nil {
		t.Fatalf("query policies failed: %v", err)
	}
	if count < 5 {
		t.Fatalf("expected at least 5 seeded policies, got %d", count)
	}

	var mode string
	err = sqlDB.QueryRow(`SELECT update_mode FROM table_idempotency_policies WHERE table_name='balance_detail'`).Scan(&mode)
	if err != nil {
		t.Fatalf("query balance_detail policy failed: %v", err)
	}
	if mode != "incremental_latest" {
		t.Fatalf("balance_detail update_mode=%s, want incremental_latest", mode)
	}
}

func TestBootstrapCreatesFeishuSyncSources(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "feishu-sync.sqlite")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	assertSQLiteTableExists(t, sqlDB, "feishu_sync_sources")
	assertSQLiteColumnsExist(t, sqlDB, "feishu_sync_sources",
		"source_type",
		"source_token",
		"source_url",
		"sync_status",
		"last_content_hash",
		"metadata_json",
	)
}

func TestBootstrapAddsContractExtensionColumnsToLegacySQLiteTables(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "bootstrap-contract-extensions.sqlite")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	legacyDDL := []string{
		`CREATE TABLE fin_contracts (
			contract_id TEXT PRIMARY KEY,
			customer_name TEXT NOT NULL,
			contract_content TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE fin_cost_settlements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT NOT NULL,
			year_month TEXT NOT NULL,
			quantity TEXT,
			settlement_amount DECIMAL(18,2) NOT NULL,
			is_invoiced TEXT,
			account_code TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE fin_fund_income (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT NOT NULL,
			year_month TEXT NOT NULL,
			settlement_amount DECIMAL(18,2),
			received_amount DECIMAL(18,2),
			is_invoiced TEXT,
			invoice_amount DECIMAL(18,2),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE fin_cost_settlement_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_name TEXT NOT NULL,
			year_month TEXT NOT NULL,
			settlement_amount DECIMAL(18,2),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE fin_cost_settlement_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL,
			contract_id TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE fin_fund_income_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_name TEXT NOT NULL,
			year_month TEXT NOT NULL,
			settlement_amount DECIMAL(18,2),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE fin_fund_income_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL,
			contract_id TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, ddl := range legacyDDL {
		if _, err := sqlDB.Exec(ddl); err != nil {
			t.Fatalf("seed legacy ddl failed: %v", err)
		}
	}

	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	assertSQLiteColumnsExist(t, sqlDB, "fin_contracts",
		"contract_start_date",
		"contract_end_date",
		"settlement_cycle",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_cost_settlements",
		"source_report_type",
		"source_sheet_name",
		"contract_start_date",
		"contract_end_date",
		"settlement_cycle",
		"settlement_unit_price",
		"invoice_amount",
		"paid_amount",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_fund_income",
		"source_report_type",
		"source_sheet_name",
		"quantity",
		"contract_start_date",
		"contract_end_date",
		"settlement_cycle",
		"settlement_unit_price",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_cost_settlement_groups",
		"source_report_type",
		"source_sheet_name",
		"source_start_row",
		"source_end_row",
		"merge_range",
		"quantity",
		"is_invoiced",
		"invoice_amount",
		"paid_amount",
		"account_code",
		"contract_start_date",
		"contract_end_date",
		"settlement_cycle",
		"settlement_unit_price",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_cost_settlement_group_members",
		"source_row_number",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_fund_income_groups",
		"source_report_type",
		"source_sheet_name",
		"source_start_row",
		"source_end_row",
		"merge_range",
		"quantity",
		"received_amount",
		"is_invoiced",
		"invoice_amount",
		"contract_start_date",
		"contract_end_date",
		"settlement_cycle",
		"settlement_unit_price",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_fund_income_group_members",
		"source_row_number",
	)
}

func assertSQLiteTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()

	var count int
	err := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master for table %s: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("expected table %q to exist", table)
	}
}

func assertSQLiteColumnsExist(t *testing.T, db *sql.DB, table string, columns ...string) {
	t.Helper()

	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info %s: %v", table, err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info %s: %v", table, err)
	}

	for _, column := range columns {
		if !existing[column] {
			t.Fatalf("column %s.%s should exist after bootstrap", table, column)
		}
	}
}
