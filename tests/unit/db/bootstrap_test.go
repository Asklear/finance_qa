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

	dbPath := filepath.Join(t.TempDir(), "finance.db")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	mustHaveTables := []string{
		"uploaded_files",
		"file_registry",
		"entities",
		"balance_sheet",
		"income_statement",
		"balance_detail",
		"journal",
		"bank_statement",
		"budget",
		"dimensions",
		"dimension_members",
		"dimension_mappings",
		"fact_financials",
		"mapping_rules",
		"allocation_rules",
		"allocation_executions",
		"smart_mapping_learnings",
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

	dbPath := filepath.Join(t.TempDir(), "finance.db")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var count int
	err = sqlDB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='index' AND name = 'idx_smart_learnings_keywords'`).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master index: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected index idx_smart_learnings_keywords to exist")
	}
}
