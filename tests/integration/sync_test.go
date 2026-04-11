package integration_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/ingest"

	_ "modernc.org/sqlite"
)

func TestImporterSyncDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join("..", "testdata", "交易查询，模拟财务科技有限公司，125922640010001，人民币，20260101-20260228，共93笔_20260401121229.xlsx")
	dst := filepath.Join(dir, filepath.Base(src))
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "finance.db")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	manager := dimensions.NewManager(dimensions.NewSQLiteRepository(sqlDB))

	importer := ingest.NewImporter(manager)
	summary, err := importer.SyncDirectory(context.Background(), dbPath, dir, false)
	if err != nil {
		t.Fatalf("SyncDirectory failed: %v", err)
	}
	if len(summary.Processed) != 1 {
		t.Fatalf("processed = %d, want 1", len(summary.Processed))
	}

	sqlDB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var count int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM bank_statement`).Scan(&count); err != nil {
		t.Fatalf("count bank_statement: %v", err)
	}
	if count != 93 {
		t.Fatalf("rows = %d, want 93", count)
	}
}
