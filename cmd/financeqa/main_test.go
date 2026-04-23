package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunHelpShowsUsage(t *testing.T) {
	code, stdout, stderr := runCLIForTest(t, "help")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "financeqa - PostgreSQL CLI") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunInitDBCreatesSQLiteSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cli.sqlite")
	code, _, stderr := runCLIForTest(t, "init-db", "--db", dbPath)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='balance_sheet'`).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected balance_sheet table to exist")
	}
}

func TestRunQueryRequiresQuestion(t *testing.T) {
	code, _, stderr := runCLIForTest(t, "query")
	if code != 2 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "query requires a natural language question") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunDimensionsRequiresSubcommand(t *testing.T) {
	code, _, stderr := runCLIForTest(t, "dimensions")
	if code != 2 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "dimensions requires a subcommand") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunDimensionsListReturnsJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dims.sqlite")
	code, _, stderr := runCLIForTest(t, "init-db", "--db", dbPath)
	if code != 0 {
		t.Fatalf("init-db code = %d, stderr = %s", code, stderr)
	}

	code, stdout, stderr := runCLIForTest(t, "dimensions", "list", "--db", dbPath)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal stdout: %v\nstdout=%s", err, stdout)
	}
	if _, ok := payload["data"]; !ok {
		t.Fatalf("missing data field in %v", payload)
	}
}

func TestRunDimensionsExportPackageCreatesFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "export.sqlite")
	outputPath := filepath.Join(t.TempDir(), "dimensions.json")
	code, _, stderr := runCLIForTest(t, "init-db", "--db", dbPath)
	if code != 0 {
		t.Fatalf("init-db code = %d, stderr = %s", code, stderr)
	}

	code, stdout, stderr := runCLIForTest(t, "dimensions", "export-package", "--db", dbPath, "--output", outputPath)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected export file: %v", err)
	}
	if !strings.Contains(stdout, `"output"`) {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunDimensionsPreviewImportReadsJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "preview.sqlite")
	inputPath := filepath.Join(t.TempDir(), "dimensions.json")
	code, _, stderr := runCLIForTest(t, "init-db", "--db", dbPath)
	if code != 0 {
		t.Fatalf("init-db code = %d, stderr = %s", code, stderr)
	}
	if err := os.WriteFile(inputPath, []byte(`[]`), 0o644); err != nil {
		t.Fatalf("write preview file: %v", err)
	}

	code, stdout, stderr := runCLIForTest(t, "dimensions", "preview-import", "--db", dbPath, "--type", "dimensions", "--file", inputPath)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "{") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func runCLIForTest(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	t.Setenv("FINANCEQA_DB", "")
	t.Setenv("FINANCEQA_PG_DSN", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
