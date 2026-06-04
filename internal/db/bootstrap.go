package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"financeqa/internal/support"

	_ "modernc.org/sqlite"
)

// Open returns a PostgreSQL database handle for the provided DSN.
func Open(_ context.Context, dbPath string) (*sql.DB, error) {
	dsn := strings.TrimSpace(dbPath)
	if dsn == "" {
		dsn = support.DefaultDBPath("")
	}
	if dsn == "" {
		return nil, errors.New("database is not configured; set FINANCEQA_DB, FINANCEQA_PG_DSN, or PGHOST/PGUSER/PGDATABASE")
	}
	if looksLikeSQLitePath(dsn) {
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, fmt.Errorf("open sqlite db: %w", err)
		}
		return db, nil
	}
	if !looksLikeDSN(dsn) {
		return nil, fmt.Errorf("unsupported db target: %s", dsn)
	}
	dsn = ensureSearchPath(dsn)
	db, err := sql.Open(PGCompatDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres db: %w", err)
	}
	return db, nil
}

// Bootstrap ensures compatibility views exist on PostgreSQL.
func Bootstrap(ctx context.Context, dbPath string) error {
	if looksLikeSQLitePath(strings.TrimSpace(dbPath)) {
		db, err := sql.Open("sqlite", strings.TrimSpace(dbPath))
		if err != nil {
			return fmt.Errorf("open sqlite db: %w", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.ExecContext(ctx, TypeScriptCompatibleSchema); err != nil {
			return fmt.Errorf("bootstrap sqlite schema: %w", err)
		}
		if err := ensureSQLiteColumn(ctx, db, "balance_detail", "opening_period", "TEXT"); err != nil {
			return err
		}
		sqliteColumnUpgrades := []struct {
			tableName  string
			columnName string
			columnType string
		}{
			{tableName: "fin_contracts", columnName: "contract_start_date", columnType: "TEXT"},
			{tableName: "fin_contracts", columnName: "contract_end_date", columnType: "TEXT"},
			{tableName: "fin_contracts", columnName: "settlement_cycle", columnType: "TEXT"},
			{tableName: "fin_contracts", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "fin_cost_settlements", columnName: "invoice_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_cost_settlements", columnName: "paid_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_cost_settlements", columnName: "invoice_open_offset_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_cost_settlements", columnName: "invoice_open_offset_reason", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "source_report_type", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "source_sheet_name", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "contract_start_date", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "contract_end_date", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "settlement_cycle", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "settlement_unit_price", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "source_cell_notes", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "fin_fund_income", columnName: "source_report_type", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "source_sheet_name", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "quantity", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "remarks", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "invoice_open_offset_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_fund_income", columnName: "invoice_open_offset_reason", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "contract_start_date", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "contract_end_date", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "settlement_cycle", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "settlement_unit_price", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "source_cell_notes", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "fin_cost_settlement_groups", columnName: "source_report_type", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "source_sheet_name", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "source_start_row", columnType: "INTEGER"},
			{tableName: "fin_cost_settlement_groups", columnName: "source_end_row", columnType: "INTEGER"},
			{tableName: "fin_cost_settlement_groups", columnName: "merge_range", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "quantity", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "is_invoiced", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "invoice_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_cost_settlement_groups", columnName: "paid_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_cost_settlement_groups", columnName: "invoice_open_offset_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_cost_settlement_groups", columnName: "invoice_open_offset_reason", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "account_code", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "contract_start_date", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "contract_end_date", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "settlement_cycle", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "settlement_unit_price", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "source_cell_notes", columnType: "TEXT"},
			{tableName: "fin_cost_settlement_groups", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "fin_cost_settlement_group_members", columnName: "source_row_number", columnType: "INTEGER"},
			{tableName: "fin_cost_settlement_group_members", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "fin_fund_income_groups", columnName: "source_report_type", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "source_sheet_name", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "source_start_row", columnType: "INTEGER"},
			{tableName: "fin_fund_income_groups", columnName: "source_end_row", columnType: "INTEGER"},
			{tableName: "fin_fund_income_groups", columnName: "merge_range", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "quantity", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "received_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_fund_income_groups", columnName: "is_invoiced", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "invoice_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_fund_income_groups", columnName: "remarks", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "invoice_open_offset_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_fund_income_groups", columnName: "invoice_open_offset_reason", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "contract_start_date", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "contract_end_date", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "settlement_cycle", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "settlement_unit_price", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "source_cell_notes", columnType: "TEXT"},
			{tableName: "fin_fund_income_groups", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "fin_fund_income_group_members", columnName: "source_row_number", columnType: "INTEGER"},
			{tableName: "fin_fund_income_group_members", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "contract_main", columnName: "extension_data", columnType: "TEXT"},
			{tableName: "contract_main", columnName: "custom_metrics", columnType: "TEXT"},
			{tableName: "contract_main", columnName: "processed_at", columnType: "TIMESTAMP"},
			{tableName: "contract_main", columnName: "ocr_engine", columnType: "TEXT"},
			{tableName: "contract_main", columnName: "total_pages", columnType: "INTEGER"},
			{tableName: "contract_main", columnName: "sub_category", columnType: "TEXT"},
			{tableName: "contract_main", columnName: "feishu_root_token", columnType: "TEXT"},
			{tableName: "contract_main", columnName: "feishu_relative_path", columnType: "TEXT"},
			{tableName: "contract_main", columnName: "feishu_folder_path", columnType: "TEXT"},
			{tableName: "contract_main", columnName: "feishu_relation_key", columnType: "TEXT"},
			{tableName: "contract_main", columnName: "created_at", columnType: "TIMESTAMP"},
			{tableName: "contract_main", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "contract_invoices", columnName: "storage_key", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "feishu_file_token", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "feishu_root_token", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "feishu_parent_token", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "feishu_relative_path", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "feishu_folder_path", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "feishu_slot_key", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "feishu_file_name", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "feishu_relation_key", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "file_size", columnType: "INTEGER"},
			{tableName: "contract_invoices", columnName: "sync_status", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "ocr_status", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "ocr_engine", columnType: "TEXT"},
			{tableName: "contract_invoices", columnName: "processed_at", columnType: "TIMESTAMP"},
			{tableName: "contract_invoices", columnName: "total_pages", columnType: "INTEGER"},
			{tableName: "contract_invoices", columnName: "feishu_deleted_at", columnType: "TIMESTAMP"},
			{tableName: "contract_invoices", columnName: "last_seen_at", columnType: "TIMESTAMP"},
			{tableName: "contract_invoices", columnName: "created_at", columnType: "TIMESTAMP"},
			{tableName: "contract_invoices", columnName: "updated_at", columnType: "TIMESTAMP"},
			{tableName: "contract_pages", columnName: "has_table", columnType: "INTEGER"},
			{tableName: "contract_pages", columnName: "has_signature", columnType: "INTEGER"},
			{tableName: "contract_pages", columnName: "ocr_confidence", columnType: "REAL"},
			{tableName: "contract_pages", columnName: "created_at", columnType: "TIMESTAMP"},
			{tableName: "contract_pages", columnName: "updated_at", columnType: "TIMESTAMP"},
		}
		for _, upgrade := range sqliteColumnUpgrades {
			if upgrade.tableName == "contract_main" || upgrade.tableName == "contract_invoices" || upgrade.tableName == "contract_pages" {
				exists, err := sqliteTableExists(ctx, db, upgrade.tableName)
				if err != nil {
					return err
				}
				if !exists {
					continue
				}
			}
			if err := ensureSQLiteColumn(ctx, db, upgrade.tableName, upgrade.columnName, upgrade.columnType); err != nil {
				return err
			}
		}
		if err := dropDeprecatedSQLiteContractColumns(ctx, db); err != nil {
			return err
		}
		if err := backfillSQLiteUpdatedAt(ctx, db, sqliteFinanceUpdatedAtTables()); err != nil {
			return err
		}
		if err := backfillSQLiteContractInvoices(ctx, db); err != nil {
			return err
		}
		if err := backfillSQLiteContractPages(ctx, db); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_balance_detail_opening_period ON balance_detail(company, opening_period)`); err != nil {
			return fmt.Errorf("create sqlite idx_balance_detail_opening_period: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_bank_statement_company_date_credit ON bank_statement(company, transaction_date, credit_amount)`); err != nil {
			return fmt.Errorf("create sqlite idx_bank_statement_company_date_credit: %w", err)
		}
		if err := EnsureDefaultTableSourceMetadata(ctx, db, strings.TrimSpace(dbPath), sqliteSourceMetadataTableNames()); err != nil {
			return fmt.Errorf("bootstrap sqlite default source metadata: %w", err)
		}
		if err := EnsureStructuredTableSourceMetadata(ctx, db, strings.TrimSpace(dbPath), sqliteSourceMetadataTableNames()); err != nil {
			return fmt.Errorf("bootstrap sqlite structured source metadata: %w", err)
		}
		if err := EnsureDefaultSchemaComments(ctx, db, strings.TrimSpace(dbPath), sqliteBootstrapTableNames()); err != nil {
			return fmt.Errorf("bootstrap sqlite schema comments: %w", err)
		}
		return nil
	}

	db, err := Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres db: %w", err)
	}

	schema := effectiveSchema(ctx, db, dbPath)

	ddls := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_contracts (
			contract_id VARCHAR(32) PRIMARY KEY,
			customer_name VARCHAR(255) NOT NULL,
			contract_content VARCHAR(255),
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_revenue_settlements (
			id BIGSERIAL PRIMARY KEY,
			contract_id VARCHAR(32) NOT NULL REFERENCES %s.fin_contracts(contract_id),
			year_month VARCHAR(7) NOT NULL,
			quantity NUMERIC(18,2),
			settlement_amount NUMERIC(18,2) NOT NULL,
			is_invoiced VARCHAR(16),
			invoice_amount NUMERIC(18,2),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_cost_settlements (
			id BIGSERIAL PRIMARY KEY,
			contract_id VARCHAR(32) NOT NULL REFERENCES %s.fin_contracts(contract_id),
			year_month VARCHAR(7) NOT NULL,
			source_report_type TEXT,
			source_sheet_name TEXT,
			quantity VARCHAR(64),
			settlement_amount NUMERIC(18,2) NOT NULL,
			is_invoiced VARCHAR(16),
			invoice_amount NUMERIC(18,2),
			paid_amount NUMERIC(18,2),
			invoice_open_offset_amount NUMERIC(18,2),
			invoice_open_offset_reason TEXT,
			account_code VARCHAR(64),
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			source_cell_notes JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_cost_settlement_groups (
			id BIGSERIAL PRIMARY KEY,
			customer_name TEXT NOT NULL,
			year_month VARCHAR(7) NOT NULL,
			source_report_type TEXT,
			source_sheet_name TEXT,
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			quantity TEXT,
			settlement_amount NUMERIC(18,2),
			is_invoiced VARCHAR(16),
			invoice_amount NUMERIC(18,2),
			paid_amount NUMERIC(18,2),
			invoice_open_offset_amount NUMERIC(18,2),
			invoice_open_offset_reason TEXT,
			account_code VARCHAR(64),
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			source_cell_notes JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_cost_settlement_group_members (
			id BIGSERIAL PRIMARY KEY,
			group_id BIGINT NOT NULL REFERENCES %s.fin_cost_settlement_groups(id) ON DELETE CASCADE,
			contract_id VARCHAR(32) NOT NULL REFERENCES %s.fin_contracts(contract_id),
			source_row_number INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema, schema, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_fund_income (
			id BIGSERIAL PRIMARY KEY,
			contract_id VARCHAR(32) NOT NULL REFERENCES %s.fin_contracts(contract_id),
			year_month VARCHAR(7) NOT NULL,
			source_report_type TEXT,
			source_sheet_name TEXT,
			quantity TEXT,
			settlement_amount NUMERIC(18,2),
			received_amount NUMERIC(18,2),
			is_invoiced VARCHAR(16),
			invoice_amount NUMERIC(18,2),
			remarks TEXT,
			invoice_open_offset_amount NUMERIC(18,2),
			invoice_open_offset_reason TEXT,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			source_cell_notes JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_fund_income_groups (
			id BIGSERIAL PRIMARY KEY,
			customer_name TEXT NOT NULL,
			year_month VARCHAR(7) NOT NULL,
			source_report_type TEXT,
			source_sheet_name TEXT,
			source_start_row INTEGER,
			source_end_row INTEGER,
			merge_range TEXT,
			quantity TEXT,
			settlement_amount NUMERIC(18,2),
			received_amount NUMERIC(18,2),
			is_invoiced VARCHAR(16),
			invoice_amount NUMERIC(18,2),
			remarks TEXT,
			invoice_open_offset_amount NUMERIC(18,2),
			invoice_open_offset_reason TEXT,
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			source_cell_notes JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_fund_income_group_members (
			id BIGSERIAL PRIMARY KEY,
			group_id BIGINT NOT NULL REFERENCES %s.fin_fund_income_groups(id) ON DELETE CASCADE,
			contract_id VARCHAR(32) NOT NULL REFERENCES %s.fin_contracts(contract_id),
			source_row_number INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema, schema, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.feishu_sync_sources (
			id BIGSERIAL PRIMARY KEY,
			source_type TEXT NOT NULL,
			source_token TEXT NOT NULL,
			source_url TEXT,
			display_name TEXT,
			parent_token TEXT,
			sync_mode TEXT NOT NULL DEFAULT 'active_scan',
			sync_status TEXT NOT NULL DEFAULT 'active',
			last_revision TEXT,
			last_content_hash TEXT,
			last_event_at TIMESTAMP,
			next_scan_at TIMESTAMP,
			last_sync_at TIMESTAMP,
			last_success_at TIMESTAMP,
			error_message TEXT,
			metadata_json JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(source_type, source_token)
		)`, schema),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.fin_file_mappings (
			id BIGSERIAL PRIMARY KEY,
			table_type VARCHAR(64) NOT NULL,
			period VARCHAR(32) NOT NULL,
			company VARCHAR(255),
			storage_key VARCHAR(1024) NOT NULL,
			file_name VARCHAR(255) NOT NULL,
			description TEXT,
			file_size BIGINT,
			source_file_hash VARCHAR(64),
			source_version_id VARCHAR(512),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_contracts_name ON %s.fin_contracts(customer_name, contract_content)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_revenue_settlements_contract_period ON %s.fin_revenue_settlements(contract_id, year_month)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_cost_settlements_contract_period ON %s.fin_cost_settlements(contract_id, year_month)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_cost_settlement_groups_period ON %s.fin_cost_settlement_groups(customer_name, year_month)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_cost_settlement_group_members_group ON %s.fin_cost_settlement_group_members(group_id)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_cost_settlement_group_members_contract ON %s.fin_cost_settlement_group_members(contract_id)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_fund_income_contract_period ON %s.fin_fund_income(contract_id, year_month)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_fund_income_groups_period ON %s.fin_fund_income_groups(customer_name, year_month)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_fund_income_group_members_group ON %s.fin_fund_income_group_members(group_id)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_fund_income_group_members_contract ON %s.fin_fund_income_group_members(contract_id)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_feishu_sync_sources_status ON %s.feishu_sync_sources(sync_status, next_scan_at)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_files_table_type ON %s.fin_file_mappings(table_type)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_files_period ON %s.fin_file_mappings(period)`, schema),
		fmt.Sprintf(`COMMENT ON TABLE %s.fin_revenue_settlements IS 'DEPRECATED: 暂停使用，合同收入统一以 fin_fund_income 为准；代码已停止读取该表'`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_balance_detail ADD COLUMN IF NOT EXISTS opening_period TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_file_mappings ADD COLUMN IF NOT EXISTS source_file_hash TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_file_mappings ADD COLUMN IF NOT EXISTS source_version_id TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS linked_contract_main_id`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS document_kind`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS relative_path`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS jsonl_path`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS file_modified_at`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS file_version`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS tags`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS remarks`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main DROP COLUMN IF EXISTS feishu_modified_time`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices DROP COLUMN IF EXISTS file_path`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices DROP COLUMN IF EXISTS internal_notes`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices DROP COLUMN IF EXISTS payment_batch`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_file_token TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_root_token TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_parent_token TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_relative_path TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_folder_path TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_slot_key TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_file_name TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_deleted_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS feishu_relation_key TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS file_size BIGINT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS sync_status TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS ocr_status TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS created_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.contract_main SET created_at = COALESCE(created_at, last_seen_at, processed_at, CURRENT_TIMESTAMP), updated_at = COALESCE(updated_at, processed_at, last_seen_at, created_at, CURRENT_TIMESTAMP) WHERE created_at IS NULL OR updated_at IS NULL`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS extension_data JSONB`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS custom_metrics JSONB`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS processed_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS ocr_engine TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS total_pages INTEGER`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_main ADD COLUMN IF NOT EXISTS sub_category TEXT`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_main') IS NOT NULL THEN CREATE INDEX IF NOT EXISTS idx_contract_main_feishu_token ON %[1]s.contract_main(feishu_file_token); END IF; END $$`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_main') IS NOT NULL THEN CREATE INDEX IF NOT EXISTS idx_contract_main_feishu_slot ON %[1]s.contract_main(feishu_slot_key); END IF; END $$`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_main') IS NOT NULL THEN CREATE INDEX IF NOT EXISTS idx_contract_main_feishu_relation ON %[1]s.contract_main(feishu_root_token, feishu_relation_key); END IF; END $$`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_main') IS NOT NULL THEN CREATE INDEX IF NOT EXISTS idx_contract_main_file_hash ON %[1]s.contract_main(file_hash); END IF; END $$`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS storage_key TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_file_token TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_root_token TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_parent_token TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_relative_path TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_folder_path TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_slot_key TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_file_name TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_relation_key TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS file_size BIGINT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS sync_status TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS ocr_status TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS ocr_engine TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS processed_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS total_pages INTEGER`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS feishu_deleted_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS created_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_invoices ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.contract_invoices SET created_at = COALESCE(created_at, last_seen_at, processed_at, CURRENT_TIMESTAMP), updated_at = COALESCE(updated_at, processed_at, last_seen_at, created_at, CURRENT_TIMESTAMP) WHERE created_at IS NULL OR updated_at IS NULL`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_invoices') IS NOT NULL THEN CREATE INDEX IF NOT EXISTS idx_contract_invoices_feishu_token ON %[1]s.contract_invoices(feishu_file_token); END IF; END $$`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_invoices') IS NOT NULL THEN CREATE INDEX IF NOT EXISTS idx_contract_invoices_feishu_slot ON %[1]s.contract_invoices(feishu_slot_key); END IF; END $$`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_invoices') IS NOT NULL THEN CREATE INDEX IF NOT EXISTS idx_contract_invoices_feishu_relation ON %[1]s.contract_invoices(feishu_root_token, feishu_relation_key); END IF; END $$`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_invoices') IS NOT NULL THEN CREATE INDEX IF NOT EXISTS idx_contract_invoices_file_hash ON %[1]s.contract_invoices(file_hash); END IF; END $$`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_contracts ADD COLUMN IF NOT EXISTS contract_start_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_contracts ADD COLUMN IF NOT EXISTS contract_end_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_contracts ADD COLUMN IF NOT EXISTS settlement_cycle TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_contracts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_contracts ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.fin_contracts SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE updated_at IS NULL`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS invoice_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS paid_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS invoice_open_offset_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS invoice_open_offset_reason TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS source_report_type TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS source_sheet_name TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS contract_start_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS contract_end_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS settlement_cycle TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS settlement_unit_price TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS source_cell_notes JSONB`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.fin_cost_settlements SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE updated_at IS NULL`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS source_report_type TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS source_sheet_name TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS quantity TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS remarks TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS invoice_open_offset_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS invoice_open_offset_reason TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS contract_start_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS contract_end_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS settlement_cycle TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS settlement_unit_price TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS source_cell_notes JSONB`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.fin_fund_income SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE updated_at IS NULL`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS source_report_type TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS source_sheet_name TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS source_start_row INTEGER`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS source_end_row INTEGER`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS merge_range TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS quantity TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS is_invoiced TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS invoice_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS paid_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS invoice_open_offset_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS invoice_open_offset_reason TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS account_code TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS contract_start_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS contract_end_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS settlement_cycle TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS settlement_unit_price TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS source_cell_notes JSONB`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_groups ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.fin_cost_settlement_groups SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE updated_at IS NULL`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_group_members ADD COLUMN IF NOT EXISTS source_row_number INTEGER`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_group_members ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlement_group_members ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.fin_cost_settlement_group_members SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE updated_at IS NULL`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS source_report_type TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS source_sheet_name TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS source_start_row INTEGER`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS source_end_row INTEGER`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS merge_range TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS quantity TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS received_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS is_invoiced TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS invoice_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS remarks TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS invoice_open_offset_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS invoice_open_offset_reason TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS contract_start_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS contract_end_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS settlement_cycle TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS settlement_unit_price TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS source_cell_notes JSONB`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_groups ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.fin_fund_income_groups SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE updated_at IS NULL`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_group_members ADD COLUMN IF NOT EXISTS source_row_number INTEGER`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_group_members ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income_group_members ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`UPDATE %s.fin_fund_income_group_members SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE updated_at IS NULL`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_pages ADD COLUMN IF NOT EXISTS has_table BOOLEAN`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_pages ADD COLUMN IF NOT EXISTS has_signature BOOLEAN`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_pages ADD COLUMN IF NOT EXISTS ocr_confidence NUMERIC`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_pages ADD COLUMN IF NOT EXISTS created_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_pages ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_pages ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.contract_pages ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_pages') IS NOT NULL THEN UPDATE %[1]s.contract_pages SET plain_text = COALESCE(NULLIF(BTRIM(plain_text), ''), markdown_text) WHERE plain_text IS NULL OR BTRIM(plain_text) = ''; END IF; END $$`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_pages') IS NOT NULL THEN UPDATE %[1]s.contract_pages SET markdown_text = COALESCE(NULLIF(BTRIM(markdown_text), ''), plain_text) WHERE markdown_text IS NULL OR BTRIM(markdown_text) = ''; END IF; END $$`, schema),
		fmt.Sprintf(`DO $$ BEGIN IF to_regclass('%[1]s.contract_pages') IS NOT NULL THEN UPDATE %[1]s.contract_pages SET has_table = COALESCE(has_table, false), has_signature = COALESCE(has_signature, false), ocr_confidence = COALESCE(ocr_confidence, 0), created_at = COALESCE(created_at, CURRENT_TIMESTAMP), updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE has_table IS NULL OR has_signature IS NULL OR ocr_confidence IS NULL OR created_at IS NULL OR updated_at IS NULL; END IF; END $$`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_balance_detail_opening_period ON %s.fin_balance_detail(company, opening_period)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_bank_statement_company_date_credit ON %s.fin_bank_statement(company, transaction_date, credit_amount)`, schema),
		fmt.Sprintf(`SET search_path TO %s, public`, schema),
	}
	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("apply postgres bootstrap sql: %w", err)
		}
	}
	if err := EnsureDefaultTableSourceMetadata(ctx, db, dbPath, postgresSourceMetadataTableNames(schema)); err != nil {
		return fmt.Errorf("bootstrap postgres default source metadata: %w", err)
	}
	if err := EnsureStructuredTableSourceMetadata(ctx, db, dbPath, postgresSourceMetadataTableNames(schema)); err != nil {
		return fmt.Errorf("bootstrap postgres structured source metadata: %w", err)
	}
	if err := EnsureDefaultSchemaComments(ctx, db, dbPath, postgresBootstrapTableNames(schema)); err != nil {
		return fmt.Errorf("bootstrap postgres schema comments: %w", err)
	}
	return nil
}

func ensureSQLiteColumn(ctx context.Context, db *sql.DB, tableName, columnName, columnType string) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+tableName+`)`)
	if err != nil {
		return fmt.Errorf("inspect sqlite table %s: %w", tableName, err)
	}

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan sqlite table info %s: %w", tableName, err)
		}
		if strings.EqualFold(strings.TrimSpace(name), columnName) {
			_ = rows.Close()
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate sqlite table info %s: %w", tableName, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close sqlite table info %s: %w", tableName, err)
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, columnName, columnType)); err != nil {
		return fmt.Errorf("add sqlite column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func sqliteTableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name = ?`, tableName).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect sqlite table %s existence: %w", tableName, err)
	}
	return count > 0, nil
}

func sqliteFinanceUpdatedAtTables() []string {
	return []string{
		"fin_contracts",
		"fin_cost_settlements",
		"fin_cost_settlement_groups",
		"fin_cost_settlement_group_members",
		"fin_fund_income",
		"fin_fund_income_groups",
		"fin_fund_income_group_members",
	}
}

func backfillSQLiteUpdatedAt(ctx context.Context, db *sql.DB, tableNames []string) error {
	for _, tableName := range tableNames {
		exists, err := sqliteTableExists(ctx, db, tableName)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		updatedAtValue := "CURRENT_TIMESTAMP"
		hasCreatedAt, err := sqliteColumnExists(ctx, db, tableName, "created_at")
		if err != nil {
			return err
		}
		if hasCreatedAt {
			updatedAtValue = "COALESCE(created_at, CURRENT_TIMESTAMP)"
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET updated_at = COALESCE(updated_at, %s) WHERE updated_at IS NULL`, tableName, updatedAtValue)); err != nil {
			return fmt.Errorf("backfill sqlite %s.updated_at: %w", tableName, err)
		}
	}
	return nil
}

func sqliteColumnExists(ctx context.Context, db *sql.DB, tableName, columnName string) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+tableName+`)`)
	if err != nil {
		return false, fmt.Errorf("inspect sqlite table %s: %w", tableName, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan sqlite table info %s: %w", tableName, err)
		}
		if strings.EqualFold(strings.TrimSpace(name), columnName) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate sqlite table info %s: %w", tableName, err)
	}
	return false, nil
}

func backfillSQLiteContractInvoices(ctx context.Context, db *sql.DB) error {
	exists, err := sqliteTableExists(ctx, db, "contract_invoices")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	createdSources := []string{"created_at"}
	updatedSources := []string{"updated_at"}
	for _, columnName := range []string{"last_seen_at", "processed_at"} {
		ok, err := sqliteColumnExists(ctx, db, "contract_invoices", columnName)
		if err != nil {
			return err
		}
		if ok {
			createdSources = append(createdSources, columnName)
			updatedSources = append(updatedSources, columnName)
		}
	}
	createdSources = append(createdSources, "CURRENT_TIMESTAMP")
	updatedSources = append(updatedSources, "created_at", "CURRENT_TIMESTAMP")
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`UPDATE contract_invoices SET created_at = COALESCE(%s), updated_at = COALESCE(%s) WHERE created_at IS NULL OR updated_at IS NULL`, strings.Join(createdSources, ", "), strings.Join(updatedSources, ", "))); err != nil {
		return fmt.Errorf("backfill sqlite contract_invoices timestamps: %w", err)
	}
	return nil
}

func backfillSQLiteContractPages(ctx context.Context, db *sql.DB) error {
	exists, err := sqliteTableExists(ctx, db, "contract_pages")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	statements := []string{}
	hasPlainText, err := sqliteColumnExists(ctx, db, "contract_pages", "plain_text")
	if err != nil {
		return err
	}
	hasMarkdownText, err := sqliteColumnExists(ctx, db, "contract_pages", "markdown_text")
	if err != nil {
		return err
	}
	if hasPlainText && hasMarkdownText {
		statements = append(statements,
			`UPDATE contract_pages SET plain_text = COALESCE(NULLIF(TRIM(plain_text), ''), markdown_text) WHERE plain_text IS NULL OR TRIM(plain_text) = ''`,
			`UPDATE contract_pages SET markdown_text = COALESCE(NULLIF(TRIM(markdown_text), ''), plain_text) WHERE markdown_text IS NULL OR TRIM(markdown_text) = ''`,
		)
	}
	statements = append(statements, `UPDATE contract_pages SET has_table = COALESCE(has_table, 0), has_signature = COALESCE(has_signature, 0), ocr_confidence = COALESCE(ocr_confidence, 0), created_at = COALESCE(created_at, CURRENT_TIMESTAMP), updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP) WHERE has_table IS NULL OR has_signature IS NULL OR ocr_confidence IS NULL OR created_at IS NULL OR updated_at IS NULL`)
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("backfill sqlite contract_pages: %w", err)
		}
	}
	return nil
}

func dropDeprecatedSQLiteContractColumns(ctx context.Context, db *sql.DB) error {
	drops := []struct {
		tableName string
		columns   []string
	}{
		{
			tableName: "contract_main",
			columns: []string{
				"linked_contract_main_id",
				"document_kind",
				"relative_path",
				"jsonl_path",
				"file_modified_at",
				"file_version",
				"tags",
				"remarks",
				"feishu_modified_time",
			},
		},
		{
			tableName: "contract_invoices",
			columns: []string{
				"file_path",
				"internal_notes",
				"payment_batch",
			},
		},
	}
	for _, drop := range drops {
		exists, err := sqliteTableExists(ctx, db, drop.tableName)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		for _, columnName := range drop.columns {
			if err := dropSQLiteColumnIfExists(ctx, db, drop.tableName, columnName); err != nil {
				return err
			}
		}
	}
	return nil
}

func dropSQLiteColumnIfExists(ctx context.Context, db *sql.DB, tableName, columnName string) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+tableName+`)`)
	if err != nil {
		return fmt.Errorf("inspect sqlite table %s: %w", tableName, err)
	}

	found := false
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan sqlite table info %s: %w", tableName, err)
		}
		if strings.EqualFold(strings.TrimSpace(name), columnName) {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate sqlite table info %s: %w", tableName, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close sqlite table info %s: %w", tableName, err)
	}
	if !found {
		return nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, tableName, columnName)); err != nil {
		return fmt.Errorf("drop sqlite column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func looksLikeDSN(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return strings.Contains(s, "host=") && strings.Contains(s, "dbname=")
}

func looksLikeSQLitePath(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return strings.HasSuffix(s, ".db") || strings.HasSuffix(s, ".sqlite") || strings.HasSuffix(s, ".sqlite3")
}

func ensureSearchPath(dsn string) string {
	s := strings.ToLower(dsn)
	if strings.Contains(s, "search_path=") {
		return dsn
	}
	schema := strings.TrimSpace(os.Getenv("FINANCEQA_PG_SCHEMA"))
	if schema == "" {
		return dsn
	}
	return dsn + " search_path=" + schema + ",public"
}

func effectiveSchema(ctx context.Context, db *sql.DB, dsn string) string {
	if schema := strings.TrimSpace(os.Getenv("FINANCEQA_PG_SCHEMA")); schema != "" {
		return schema
	}
	if schema := schemaFromDSN(dsn); schema != "" {
		return schema
	}
	var schema string
	if err := db.QueryRowContext(ctx, `SELECT CURRENT_SCHEMA()`).Scan(&schema); err == nil {
		schema = strings.TrimSpace(schema)
		if schema != "" {
			return schema
		}
	}
	return "public"
}

func schemaFromDSN(dsn string) string {
	for _, part := range strings.Fields(strings.TrimSpace(dsn)) {
		lower := strings.ToLower(part)
		if !strings.HasPrefix(lower, "search_path=") {
			continue
		}
		value := strings.TrimSpace(part[len("search_path="):])
		if value == "" {
			return ""
		}
		first := strings.Split(value, ",")[0]
		return strings.Trim(first, `"`)
	}
	return ""
}
