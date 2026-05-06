package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestTableExistsAndColumnsExistForSQLite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "table-introspection.sqlite")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := sqlDB.Exec(`CREATE TABLE sample_table (id INTEGER PRIMARY KEY, name TEXT, status TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	ok, err := TableExists(ctx, sqlDB, "sample_table")
	if err != nil {
		t.Fatalf("table exists: %v", err)
	}
	if !ok {
		t.Fatal("table should exist")
	}

	ok, err = ColumnsExist(ctx, sqlDB, "sample_table", []string{"id", "name"})
	if err != nil {
		t.Fatalf("columns exist: %v", err)
	}
	if !ok {
		t.Fatal("columns should exist")
	}

	ok, err = ColumnsExist(ctx, sqlDB, "sample_table", []string{"id", "missing"})
	if err != nil {
		t.Fatalf("columns exist missing: %v", err)
	}
	if ok {
		t.Fatal("missing column should not be reported as present")
	}

	ok, err = TableExists(ctx, sqlDB, "missing_table")
	if err != nil {
		t.Fatalf("missing table exists: %v", err)
	}
	if ok {
		t.Fatal("missing table should not exist")
	}
}
