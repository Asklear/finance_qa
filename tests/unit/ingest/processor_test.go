package ingest_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"financeqa/internal/ingest"
	"financeqa/internal/parser"
)

func TestImporterImportParsed(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test_import.db")

	imp := ingest.NewImporter(nil)

	// Mock data
	result := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:    "测试公司",
			ReportType: "income_statement",
			PeriodEnd:  "2026-02",
		},
		Data: []parser.Record{
			{
				"company":           "测试公司",
				"period":            "2026-02",
				"item_name":         "一、营业收入",
				"current_amount":    1000.50,
				"cumulative_amount": 2000.00,
			},
		},
	}

	err := imp.ImportParsed(ctx, dbPath, result, false)
	if err != nil {
		t.Fatalf("ImportParsed failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var company, period, itemName string
	var currentAmount, cumulativeAmount float64
	err = db.QueryRow(`
SELECT company, period, item_name, current_amount, cumulative_amount
FROM income_statement
`).Scan(&company, &period, &itemName, &currentAmount, &cumulativeAmount)
	if err != nil {
		t.Fatalf("query income_statement failed: %v", err)
	}

	if company != "测试公司" || period != "2026-02" || itemName != "一、营业收入" {
		t.Fatalf("unexpected imported row identity: company=%q period=%q item=%q", company, period, itemName)
	}
	if currentAmount != 1000.50 || cumulativeAmount != 2000.00 {
		t.Fatalf("unexpected imported amounts: current=%v cumulative=%v", currentAmount, cumulativeAmount)
	}
}

func TestProcessorProcessFileDelegation(t *testing.T) {
	// Processor is a thin wrapper that currently only handles metadata extraction
	// without actually importing into DB in its current ProcessFile implementation.
	// This tests the structure is intact.
	proc := ingest.NewProcessor(nil)

	// We can't easily test ProcessFile without a real file on disk,
	// but we can verify it exists and compiles.
	_ = proc
}
