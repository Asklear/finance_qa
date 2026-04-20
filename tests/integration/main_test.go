package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func runCLI(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	// use go run to execute the main package
	cmd := exec.Command(resolveGoBinary(), append([]string{"run", "../../cmd/financeqa/main.go"}, args...)...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return exitCode, stdout.String(), stderr.String()
}

func resolveGoBinary() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	if _, err := os.Stat("/opt/homebrew/bin/go"); err == nil {
		return "/opt/homebrew/bin/go"
	}
	return "go"
}

func sqlBootstrap(dbPath string) error {
	exitCode, _, stderr := runCLI("init-db", "--db", dbPath)
	if exitCode != 0 {
		return fmt.Errorf("init-db failed: %s", stderr)
	}
	return nil
}

func TestRunInitDBCommandCreatesDatabase(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "initdb.sqlite")

	exitCode, _, stderr := runCLI("init-db", "--db", dbPath)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", exitCode, stderr)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var count int
	err = sqlDB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='journal'`).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected journal table to exist")
	}
}

func TestRunUnknownCommandReturnsNonZero(t *testing.T) {
	t.Parallel()

	exitCode, _, _ := runCLI("unknown")
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for unknown command")
	}
}

func TestRunQueryCommandReturnsAnswer(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "query.sqlite")
	if err := seedQueryDB(dbPath); err != nil {
		t.Fatalf("seed query db: %v", err)
	}

	exitCode, stdout, stderr := runCLI("query", "--db", dbPath, "--company", "模拟财务", "2026年2月收入是多少")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "\"现金流入\": 1500") {
		t.Errorf("stdout should include income answer, got %s", stdout)
	}
}

func TestRunHostDataCommandReturnsPayload(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "hostdata.sqlite")
	if err := seedQueryDB(dbPath); err != nil {
		t.Fatalf("seed query db: %v", err)
	}

	exitCode, stdout, stderr := runCLI("host-data", "--db", dbPath, "--company", "模拟财务", "--from", "2026-02", "--to", "2026-02")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "\"llm_payload\"") {
		t.Fatalf("stdout should include llm_payload, got %s", stdout)
	}
	if !strings.Contains(stdout, "\"answer_method\": \"llm_payload\"") {
		t.Fatalf("stdout should include answer_method llm_payload, got %s", stdout)
	}
}

func TestRunImportCommandLoadsFixture(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "config-show.sqlite")
	if err := sqlBootstrap(dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	src := filepath.Join("..", "testdata", "交易查询，模拟财务科技有限公司，125922640010001，人民币，20260101-20260228，共93笔_20260401121229.xlsx")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	exitCode, stdout, stderr := runCLI("import", "--db", dbPath, src)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "\"recordCount\": 93") {
		t.Fatalf("stdout should include import summary, got %s", stdout)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var count int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM bank_statement`).Scan(&count); err != nil {
		t.Fatalf("count imported bank_statement rows: %v", err)
	}
	if count != 93 {
		t.Fatalf("imported rows = %d, want 93", count)
	}
}

func TestRunDimensionsCommands(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "dimensions.sqlite")
	if err := sqlBootstrap(dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	exitCode, _, stderr := runCLI("dimensions", "add-dimension", "--db", dbPath, "--code", "product", "--name", "Product", "--type", "product")
	if exitCode != 0 {
		t.Fatalf("add-dimension exit=%d stderr=%s", exitCode, stderr)
	}

	exitCode, _, stderr = runCLI("dimensions", "add-member", "--db", dbPath, "--dimension", "product", "--code", "P001", "--name", "SaaS")
	if exitCode != 0 {
		t.Fatalf("add-member exit=%d stderr=%s", exitCode, stderr)
	}

	exitCode, stdout, stderr := runCLI("dimensions", "list", "--db", dbPath)
	if exitCode != 0 {
		t.Fatalf("dimensions list exit=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "\"code\": \"product\"") {
		t.Fatalf("expected dimensions list output, got %s", stdout)
	}
}

func TestRunSyncImportViaDirectory(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "import.sqlite")
	if err := sqlBootstrap(dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	dir := t.TempDir()
	src := filepath.Join("..", "testdata", "交易查询，模拟财务科技有限公司，125922640010001，人民币，20260101-20260228，共93笔_20260401121229.xlsx")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	dst := filepath.Join(dir, filepath.Base(src))
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	exitCode, stdout, stderr := runCLI("sync", "--db", dbPath, dir)
	if exitCode != 0 {
		t.Fatalf("sync exit=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "\"recordCount\": 93") {
		t.Fatalf("expected sync output, got %s", stdout)
	}
}

func TestRunSyncSkipsHiddenAndUnsupportedFiles(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "sync.sqlite")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".ignored.xlsx"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write hidden fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write unsupported fixture: %v", err)
	}

	exitCode, stdout, stderr := runCLI("sync", "--db", dbPath, dir)
	if exitCode != 0 {
		t.Fatalf("sync exit=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "\"processed\": []") {
		t.Fatalf("expected empty processed list, got %s", stdout)
	}
}

func TestRunDimensionsImportExportCommands(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "host-payload.sqlite")
	if err := sqlBootstrap(dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	exitCode, _, stderr := runCLI("dimensions", "add-dimension", "--db", dbPath, "--code", "product", "--name", "Product", "--type", "product")
	if exitCode != 0 {
		t.Fatalf("add-dimension exit=%d stderr=%s", exitCode, stderr)
	}

	exitCode, _, stderr = runCLI("dimensions", "add-member", "--db", dbPath, "--dimension", "product", "--code", "P001", "--name", "SaaS")
	if exitCode != 0 {
		t.Fatalf("add-member exit=%d stderr=%s", exitCode, stderr)
	}

	exportPath := filepath.Join(t.TempDir(), "dimensions.json")
	exitCode, _, stderr = runCLI("dimensions", "export-package", "--db", dbPath, "--output", exportPath)
	if exitCode != 0 {
		t.Fatalf("export-package exit=%d stderr=%s", exitCode, stderr)
	}
	exported, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read export package: %v", err)
	}
	if !strings.Contains(string(exported), "\"code\": \"product\"") {
		t.Fatalf("expected exported package to include dimension, got %s", string(exported))
	}

	importDBPath := filepath.Join(t.TempDir(), "import.db")
	if err := sqlBootstrap(importDBPath); err != nil {
		t.Fatalf("bootstrap import db: %v", err)
	}
	importDimsPath := filepath.Join(t.TempDir(), "import-dimensions.json")
	if err := os.WriteFile(importDimsPath, []byte(`[
  {"code":"region","name":"Region","type":"custom","isHierarchical":true,"isActive":true}
]`), 0o644); err != nil {
		t.Fatalf("write dimensions import file: %v", err)
	}

	exitCode, stdout, stderr := runCLI("dimensions", "import-dimensions", "--db", importDBPath, "--file", importDimsPath)
	if exitCode != 0 {
		t.Fatalf("import-dimensions exit=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "\"createdCount\": 1") {
		t.Fatalf("expected import report, got %s", stdout)
	}

	previewMembersPath := filepath.Join(t.TempDir(), "preview-members.json")
	if err := os.WriteFile(previewMembersPath, []byte(`[
  {"code":"CN","name":"China","level":1,"path":"CN","isActive":true,"sortOrder":1}
]`), 0o644); err != nil {
		t.Fatalf("write preview members file: %v", err)
	}

	exitCode, stdout, stderr = runCLI("dimensions", "preview-import", "--db", importDBPath, "--type", "members", "--dimension", "region", "--file", previewMembersPath)
	if exitCode != 0 {
		t.Fatalf("preview-import exit=%d stderr=%s", exitCode, stderr)
	}

	var preview map[string]any
	if err := json.Unmarshal([]byte(stdout), &preview); err != nil {
		t.Fatalf("unmarshal preview output: %v", err)
	}
	if valid, _ := preview["valid"].(bool); !valid {
		t.Fatalf("expected preview to be valid, got %s", stdout)
	}
}

func seedQueryDB(dbPath string) error {
	if err := sqlBootstrap(dbPath); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
INSERT INTO balance_sheet (company, period, account_name, opening_balance, closing_balance) VALUES
  ('模拟财务科技有限公司','2026-02','货币资金',100,150);
INSERT INTO bank_statement (company, transaction_date, credit_amount, debit_amount, counterparty_name, summary) VALUES
  ('模拟财务科技有限公司','2026-02-10',1000,0,'客户A','回款'),
  ('模拟财务科技有限公司','2026-02-12',500,50,'客户C','手续费');
`)
	return err
}
