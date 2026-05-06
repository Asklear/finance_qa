package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/feishu"
	"financeqa/internal/feishusync"
	"financeqa/internal/ingest"

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

func TestRunInitDBRedactsDatabasePasswordInOutput(t *testing.T) {
	got := redactDBTargetForCLI("host=pg.example.com port=5432 user=finance password=super-secret dbname=bossagent search_path=tenant_uhub,public")
	if strings.Contains(got, "super-secret") {
		t.Fatalf("redacted db target should not contain password: %q", got)
	}
	if !strings.Contains(got, "password=<redacted>") {
		t.Fatalf("redacted db target should preserve password marker, got %q", got)
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

func TestRunFeishuRequiresSubcommand(t *testing.T) {
	code, _, stderr := runCLIForTest(t, "feishu")
	if code != 2 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "feishu requires a subcommand") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunFeishuOAuthURLPrintsAuthorizationURL(t *testing.T) {
	t.Setenv("FEISHU_APP_ID", "cli_test")
	t.Setenv("FEISHU_APP_SECRET", "secret")
	t.Setenv("FINANCEQA_PG_DSN", "")

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"feishu", "oauth-url", "--redirect-uri", "http://127.0.0.1:8787/feishu/oauth/callback", "--state", "state-1"}, &out, &errOut)
	stdout := out.String()
	stderr := errOut.String()
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "https://open.feishu.cn/open-apis/authen/v1/index?") ||
		!strings.Contains(stdout, "app_id=cli_test") ||
		!strings.Contains(stdout, "redirect_uri=http%3A%2F%2F127.0.0.1%3A8787%2Ffeishu%2Foauth%2Fcallback") ||
		!strings.Contains(stdout, "state=state-1") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunOCRRequiresSubcommand(t *testing.T) {
	code, _, stderr := runCLIForTest(t, "ocr")
	if code != 2 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "ocr requires a subcommand") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestBuildSQLAuditSectionsIncludesContractGroupRows(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (contract_id TEXT, year_month TEXT, settlement_amount REAL, received_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_fund_income_groups (customer_name TEXT, year_month TEXT, settlement_amount REAL, received_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (contract_id TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlement_groups (customer_name TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_bank_statement (transaction_date TEXT, credit_amount REAL, debit_amount REAL)`,
		`CREATE TABLE fin_income_statement (period TEXT, item_name TEXT, current_amount REAL)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C1','飞未云科测试','收入合同'), ('C2','南京林悦测试','成本合同')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount) VALUES ('C1','2026-03',100,80,90)`,
		`INSERT INTO fin_fund_income_groups(customer_name, year_month, settlement_amount, received_amount, invoice_amount) VALUES ('飞未云科测试','2026-03',7,6,5)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, settlement_amount, paid_amount, invoice_amount) VALUES ('C2','2026-03',40,30,20)`,
		`INSERT INTO fin_cost_settlement_groups(customer_name, year_month, settlement_amount, paid_amount, invoice_amount) VALUES ('南京林悦测试','2026-03',4,3,2)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	sections, err := buildSQLAuditSections(context.Background(), db)
	if err != nil {
		t.Fatalf("buildSQLAuditSections: %v", err)
	}
	contractMarch := auditSectionByNameForTest(t, sections, "contract_2026_03")
	assertAuditAmountForTest(t, contractMarch, "settlement_amount", 107)
	assertAuditAmountForTest(t, contractMarch, "cash_amount", 86)
	assertAuditAmountForTest(t, contractMarch, "invoice_amount", 95)
	if contractMarch.Rows != 2 {
		t.Fatalf("contract rows = %d, want 2", contractMarch.Rows)
	}
	costMarch := auditSectionByNameForTest(t, sections, "cost_2026_03")
	assertAuditAmountForTest(t, costMarch, "settlement_amount", 44)
	assertAuditAmountForTest(t, costMarch, "cash_amount", 33)
	assertAuditAmountForTest(t, costMarch, "invoice_amount", 22)
	if costMarch.Rows != 2 {
		t.Fatalf("cost rows = %d, want 2", costMarch.Rows)
	}
}

func TestRunOCRProcessFileRequiresFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ocr.sqlite")
	code, _, stderr := runCLIForTest(t, "ocr", "process-file", "--db", dbPath)
	if code != 2 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "--file is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunOCRProcessPendingRequiresGeminiKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	dbPath := filepath.Join(t.TempDir(), "ocr.sqlite")
	code, _, stderr := runCLIForTest(t, "ocr", "process-pending", "--db", dbPath)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "GEMINI_API_KEY is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestGeminiConfigReadsGoogleBaseURL(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "https://api.example.test")

	config, err := geminiConfigFromEnv()
	if err != nil {
		t.Fatalf("geminiConfigFromEnv: %v", err)
	}
	if config.BaseURL != "https://api.example.test/v1beta" {
		t.Fatalf("BaseURL = %q", config.BaseURL)
	}
}

func TestGeminiConfigKeepsVersionedBaseURL(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "https://api.example.test/v1beta")

	config, err := geminiConfigFromEnv()
	if err != nil {
		t.Fatalf("geminiConfigFromEnv: %v", err)
	}
	if config.BaseURL != "https://api.example.test/v1beta" {
		t.Fatalf("BaseURL = %q", config.BaseURL)
	}
}

func TestOCRConcurrencyDefaultsFromEnv(t *testing.T) {
	t.Setenv("OCR_WORKER_CONCURRENCY", "3")
	if got := defaultOCRWorkerConcurrency(); got != 3 {
		t.Fatalf("defaultOCRWorkerConcurrency = %d", got)
	}
}

func TestRunFeishuSeedSources(t *testing.T) {
	t.Setenv("FEISHU_SYNC_SOURCES_JSON", `[
		{"source_type":"finance_workbook","source_token":"workbook-token","source_url":"https://example.feishu.cn/file/workbook-token","display_name":"飞书财务表格","metadata_json":{"oss_prefix":"tenant/uhub/finance"}},
		{"source_type":"pdf_folder","source_token":"folder-token","source_url":"https://example.feishu.cn/drive/folder/folder-token","display_name":"飞书 PDF 文件夹","metadata_json":{"oss_prefix":"tenant/uhub/contract"}}
	]`)
	dbPath := filepath.Join(t.TempDir(), "feishu.sqlite")
	code, _, stderr := runCLIForTest(t, "init-db", "--db", dbPath)
	if code != 0 {
		t.Fatalf("init-db code = %d, stderr = %s", code, stderr)
	}

	code, stdout, stderr := runCLIForTest(t, "feishu", "seed-sources", "--db", dbPath)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "workbook-token") {
		t.Fatalf("stdout = %q", stdout)
	}

	code, stdout, stderr = runCLIForTest(t, "feishu", "sources", "--db", dbPath)
	if code != 0 {
		t.Fatalf("sources code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "folder-token") {
		t.Fatalf("sources stdout = %q", stdout)
	}
}

func TestRunFeishuSourcesListsSeededSources(t *testing.T) {
	t.Setenv("FEISHU_SYNC_SOURCES_JSON", `[
		{"source_type":"finance_workbook","source_token":"workbook-token"},
		{"source_type":"pdf_folder","source_token":"folder-token"}
	]`)
	dbPath := filepath.Join(t.TempDir(), "feishu-sources.sqlite")
	code, _, stderr := runCLIForTest(t, "feishu", "seed-sources", "--db", dbPath)
	if code != 0 {
		t.Fatalf("seed code = %d, stderr = %s", code, stderr)
	}

	code, stdout, stderr := runCLIForTest(t, "feishu", "sources", "--db", dbPath, "--source-type", "pdf_folder")
	if code != 0 {
		t.Fatalf("sources code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "folder-token") {
		t.Fatalf("sources stdout = %q", stdout)
	}
	if strings.Contains(stdout, "workbook-token") {
		t.Fatalf("source type filter should exclude workbook: %q", stdout)
	}
}

func TestRunFeishuSeedSourcesRequiresConfiguredSources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "feishu-missing-sources.sqlite")
	code, _, stderr := runCLIForTest(t, "feishu", "seed-sources", "--db", dbPath)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "FEISHU_SYNC_SOURCES_JSON") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunFeishuScanRequiresCredentials(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "feishu-scan.sqlite")
	code, _, stderr := runCLIForTest(t, "feishu", "scan", "--db", dbPath)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "FEISHU_APP_ID and FEISHU_APP_SECRET are required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunFeishuSyncOnceRequiresSourceToken(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "feishu-sync-once.sqlite")
	code, _, stderr := runCLIForTest(t, "feishu", "sync-once", "--db", dbPath)
	if code != 2 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "--source-token is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestNewFeishuRunnerUsesObjectStoreForPDFScan(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "feishu-oss.sqlite")
	code, _, stderr := runCLIForTest(t, "init-db", "--db", dbPath)
	if code != 0 {
		t.Fatalf("init-db code = %d, stderr = %s", code, stderr)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	createContractPDFStateTablesForCmdTest(t, sqlDB)

	repo := feishusync.NewRepository(sqlDB)
	src := feishusync.SyncSource{
		SourceType:   feishusync.SourceTypePDFFolder,
		SourceToken:  "folder-cmd",
		DisplayName:  "合同目录",
		SyncStatus:   feishusync.SyncStatusActive,
		MetadataJSON: `{"oss_prefix":"tenant/uhub/contract/优集客户合同"}`,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: feishusync.SourceTypePDFFolder})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources = %#v, want one source", sources)
	}

	store := &cmdRecordingObjectStore{uri: "tenant/uhub/contract/优集客户合同/合同A.pdf"}
	runner := newFeishuRunner(repo, &cmdFakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "file-cmd",
			Name:        "合同A.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-cmd",
			Size:        12,
			Revision:    "rev-1",
		}},
		downloads: map[string][]byte{"file-cmd": []byte("contract-body")},
	}, &cmdWorkbookImporter{}, dbPath, t.TempDir(), "测试公司", store)

	result, err := runner.scanSource(context.Background(), sources[0])
	if err != nil {
		t.Fatalf("scan source: %v", err)
	}
	if result.Created != 1 || result.OCRQueued != 1 {
		t.Fatalf("result = %#v, want created=1 ocrQueued=1", result)
	}
	if len(store.puts) != 1 {
		t.Fatalf("store puts = %#v, want one upload", store.puts)
	}
	state, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-cmd:合同a.pdf")
	if err != nil {
		t.Fatalf("find contract: %v", err)
	}
	if !ok {
		t.Fatalf("contract should be active")
	}
	if state.StorageKey != store.uri {
		t.Fatalf("storage key = %q, want %q", state.StorageKey, store.uri)
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
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	if _, ok := os.LookupEnv("FEISHU_SYNC_SOURCES_JSON"); !ok {
		t.Setenv("FEISHU_SYNC_SOURCES_JSON", "")
	}
	if _, ok := os.LookupEnv("FEISHU_SYNC_SOURCES_FILE"); !ok {
		t.Setenv("FEISHU_SYNC_SOURCES_FILE", "")
	}
	t.Setenv("OSS_ACCESS_KEY_ID", "")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "")
	t.Setenv("OSS_BUCKET", "")
	t.Setenv("OSS_ENDPOINT", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func auditSectionByNameForTest(t *testing.T, sections []auditSection, name string) auditSection {
	t.Helper()
	for _, section := range sections {
		if section.Name == name {
			return section
		}
	}
	t.Fatalf("missing audit section %q in %#v", name, sections)
	return auditSection{}
}

func assertAuditAmountForTest(t *testing.T, section auditSection, name string, want float64) {
	t.Helper()
	for _, amount := range section.Amounts {
		if amount.Name == name {
			if amount.Value != want {
				t.Fatalf("%s.%s = %v, want %v", section.Name, name, amount.Value, want)
			}
			return
		}
	}
	t.Fatalf("missing amount %q in %#v", name, section.Amounts)
}

func createContractPDFStateTablesForCmdTest(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := []string{
		`CREATE TABLE contract_categories (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			code TEXT NOT NULL,
			sort_order INTEGER
		)`,
		`INSERT INTO contract_categories(id, name, code, sort_order) VALUES (1, '客户合同', 'customer', 1)`,
		`CREATE TABLE contract_main (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL UNIQUE,
			file_name TEXT,
			file_hash TEXT,
			storage_key TEXT,
			feishu_file_token TEXT,
			feishu_root_token TEXT,
			feishu_parent_token TEXT,
			feishu_relative_path TEXT,
			feishu_folder_path TEXT,
			feishu_slot_key TEXT,
			feishu_file_name TEXT,
			feishu_relation_key TEXT,
			category_id INTEGER NOT NULL,
			file_size INTEGER,
				sync_status TEXT,
				ocr_status TEXT,
				feishu_deleted_at TIMESTAMP,
				last_seen_at TIMESTAMP,
				created_at TIMESTAMP,
				updated_at TIMESTAMP
			)`,
		`CREATE TABLE contract_duplicate_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT,
			source_file_token TEXT,
			existing_contract_id INTEGER,
			target_contract_id INTEGER,
			file_hash TEXT,
			old_file_hash TEXT,
			slot_key TEXT,
			message TEXT,
			metadata_json TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec fixture sql: %v\n%s", err, stmt)
		}
	}
}

type cmdFakeFeishuClient struct {
	files     []feishu.DriveFile
	downloads map[string][]byte
}

func (c *cmdFakeFeishuClient) ListFolderFiles(_ context.Context, _ string) ([]feishu.DriveFile, error) {
	return c.files, nil
}

func (c *cmdFakeFeishuClient) GetFileMetadata(_ context.Context, fileToken string) (feishu.DriveFile, error) {
	for _, file := range c.files {
		if file.Token == fileToken {
			return file, nil
		}
	}
	return feishu.DriveFile{}, nil
}

func (c *cmdFakeFeishuClient) DownloadFile(_ context.Context, fileToken, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, c.downloads[fileToken], 0o644)
}

func (c *cmdFakeFeishuClient) ExportToXLSX(_ context.Context, fileToken, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, c.downloads[fileToken], 0o644)
}

type cmdRecordingObjectStore struct {
	uri  string
	puts []cmdObjectStorePut
}

type cmdObjectStorePut struct {
	localPath   string
	key         string
	contentType string
}

func (s *cmdRecordingObjectStore) PutFile(_ context.Context, localPath, key, contentType string) (string, error) {
	s.puts = append(s.puts, cmdObjectStorePut{localPath: localPath, key: key, contentType: contentType})
	return s.uri, nil
}

type cmdWorkbookImporter struct{}

func (cmdWorkbookImporter) ImportFileWithOptions(context.Context, string, string, ingest.ImportOptions) (ingest.ImportSummary, error) {
	return ingest.ImportSummary{}, nil
}
