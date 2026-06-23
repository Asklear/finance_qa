package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
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
		"updated_at",
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
		"updated_at",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_fund_income",
		"source_report_type",
		"source_sheet_name",
		"quantity",
		"contract_start_date",
		"contract_end_date",
		"settlement_cycle",
		"settlement_unit_price",
		"updated_at",
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
		"updated_at",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_cost_settlement_group_members",
		"source_row_number",
		"updated_at",
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
		"updated_at",
	)
	assertSQLiteColumnsExist(t, sqlDB, "fin_fund_income_group_members",
		"source_row_number",
		"updated_at",
	)
}

func TestBootstrapBackfillsContractPagesMetadataOnLegacySQLiteTable(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "bootstrap-contract-pages.sqlite")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := sqlDB.Exec(`CREATE TABLE contract_pages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER,
		page_num INTEGER,
		page_number INTEGER,
		markdown_text TEXT,
		plain_text TEXT,
		raw_ocr_json TEXT,
		has_images INTEGER,
		word_count INTEGER,
		char_count INTEGER,
		UNIQUE(contract_id, page_num)
	)`); err != nil {
		t.Fatalf("seed legacy contract_pages: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO contract_pages(contract_id, page_num, page_number, markdown_text, plain_text, has_images, word_count, char_count)
VALUES (10, 0, 1, '## 页面正文', '', 1, 0, 0)`); err != nil {
		t.Fatalf("seed legacy contract_pages row: %v", err)
	}

	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	assertSQLiteColumnsExist(t, sqlDB, "contract_pages",
		"has_table",
		"has_signature",
		"ocr_confidence",
		"created_at",
		"updated_at",
	)

	var plainText, markdownText, createdAt, updatedAt string
	var hasTable, hasSignature int
	var confidence float64
	if err := sqlDB.QueryRow(`
SELECT plain_text, markdown_text, has_table, has_signature, ocr_confidence,
       COALESCE(CAST(created_at AS TEXT), ''), COALESCE(CAST(updated_at AS TEXT), '')
FROM contract_pages
WHERE contract_id = 10 AND page_num = 0
`).Scan(&plainText, &markdownText, &hasTable, &hasSignature, &confidence, &createdAt, &updatedAt); err != nil {
		t.Fatalf("load backfilled contract_pages row: %v", err)
	}
	if plainText != "## 页面正文" || markdownText != "## 页面正文" {
		t.Fatalf("text backfill plain=%q markdown=%q", plainText, markdownText)
	}
	if hasTable != 0 || hasSignature != 0 || confidence != 0 || strings.TrimSpace(createdAt) == "" || strings.TrimSpace(updatedAt) == "" {
		t.Fatalf("metadata backfill hasTable=%d hasSignature=%d confidence=%v createdAt=%q updatedAt=%q", hasTable, hasSignature, confidence, createdAt, updatedAt)
	}
}

func TestBootstrapAddsOCRColumnsToLegacyContractMainSQLiteTable(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "bootstrap-ocr-contract-main.sqlite")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := sqlDB.Exec(`CREATE TABLE contract_main (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_name TEXT,
		storage_key TEXT,
		file_hash TEXT,
		ocr_status TEXT,
		document_kind TEXT,
		linked_contract_main_id INTEGER,
		relative_path TEXT,
		jsonl_path TEXT,
		file_modified_at TIMESTAMP,
		file_version INTEGER,
		tags TEXT,
		remarks TEXT,
		feishu_modified_time TIMESTAMP
	)`); err != nil {
		t.Fatalf("seed legacy contract_main: %v", err)
	}

	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	assertSQLiteColumnsExist(t, sqlDB, "contract_main",
		"extension_data",
		"custom_metrics",
		"processed_at",
		"ocr_engine",
		"total_pages",
		"feishu_root_token",
		"feishu_relative_path",
		"feishu_folder_path",
		"feishu_relation_key",
		"feishu_modified_time",
		"created_at",
		"updated_at",
	)
	for _, columnName := range []string{
		"document_kind",
		"linked_contract_main_id",
		"relative_path",
		"jsonl_path",
		"file_modified_at",
		"file_version",
		"tags",
		"remarks",
	} {
		assertSQLiteColumnMissing(t, sqlDB, "contract_main", columnName)
	}
}

func TestBootstrapBackfillsInvoiceSystemTimestampsOnLegacySQLiteTable(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "bootstrap-contract-invoice-timestamps.sqlite")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := sqlDB.Exec(`CREATE TABLE contract_invoices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER NOT NULL,
		invoice_number TEXT NOT NULL,
		file_name TEXT,
		storage_key TEXT,
		file_hash TEXT,
		sync_status TEXT,
		ocr_status TEXT,
		last_seen_at TIMESTAMP,
		processed_at TIMESTAMP
	)`); err != nil {
		t.Fatalf("seed legacy contract_invoices: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO contract_invoices(contract_id, invoice_number, file_name, last_seen_at)
VALUES (10, 'pending:test', 'invoice.pdf', '2026-05-01 10:00:00')`); err != nil {
		t.Fatalf("seed legacy invoice row: %v", err)
	}

	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	assertSQLiteColumnsExist(t, sqlDB, "contract_invoices", "created_at", "updated_at")

	var createdAt, updatedAt string
	if err := sqlDB.QueryRow(`
SELECT COALESCE(CAST(created_at AS TEXT), ''), COALESCE(CAST(updated_at AS TEXT), '')
FROM contract_invoices
WHERE invoice_number = 'pending:test'
`).Scan(&createdAt, &updatedAt); err != nil {
		t.Fatalf("load invoice timestamps: %v", err)
	}
	if strings.TrimSpace(createdAt) == "" || strings.TrimSpace(updatedAt) == "" {
		t.Fatalf("invoice timestamps createdAt=%q updatedAt=%q", createdAt, updatedAt)
	}
}

func TestBootstrapSeedsCommentsForContractOCRTables(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "bootstrap-contract-comments.sqlite")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := sqlDB.Exec(`CREATE TABLE contract_main (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_name TEXT,
		storage_key TEXT,
		file_hash TEXT,
		feishu_file_token TEXT,
		feishu_root_token TEXT,
		feishu_relative_path TEXT,
		sync_status TEXT,
		ocr_status TEXT
	)`); err != nil {
		t.Fatalf("seed contract_main: %v", err)
	}
	if _, err := sqlDB.Exec(`CREATE TABLE contract_invoices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER,
		invoice_number TEXT,
		storage_key TEXT,
		file_hash TEXT,
		feishu_file_token TEXT,
		feishu_relation_key TEXT,
		sync_status TEXT,
		ocr_status TEXT
	)`); err != nil {
		t.Fatalf("seed contract_invoices: %v", err)
	}

	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	assertSQLiteColumnComment(t, sqlDB, "contract_main", "feishu_file_token", "飞书文件 token")
	assertSQLiteColumnComment(t, sqlDB, "contract_main", "sub_category", "合同子分类")
	assertSQLiteColumnComment(t, sqlDB, "contract_invoices", "feishu_relation_key", "合同/发票目录关联 key")
	assertSQLiteColumnComment(t, sqlDB, "contract_invoices", "ocr_status", "OCR 处理状态")
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

func assertSQLiteColumnComment(t *testing.T, db *sql.DB, table, column, wantContains string) {
	t.Helper()

	var comment string
	err := db.QueryRow(`
SELECT comment
FROM meta_column_comments
WHERE table_name = ? AND column_name = ?
`, table, column).Scan(&comment)
	if err != nil {
		t.Fatalf("load column comment %s.%s: %v", table, column, err)
	}
	if !strings.Contains(comment, wantContains) {
		t.Fatalf("comment for %s.%s = %q, want to contain %q", table, column, comment, wantContains)
	}
}

func assertSQLiteColumnMissing(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()

	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma table_info %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info %s: %v", table, err)
		}
		if name == column {
			t.Fatalf("expected %s.%s to be absent", table, column)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info %s: %v", table, err)
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
