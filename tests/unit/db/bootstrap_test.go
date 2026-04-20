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
