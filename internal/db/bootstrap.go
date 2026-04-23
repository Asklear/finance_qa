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
			{tableName: "fin_cost_settlements", columnName: "invoice_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_cost_settlements", columnName: "paid_amount", columnType: "DECIMAL(18,2)"},
			{tableName: "fin_cost_settlements", columnName: "source_report_type", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "source_sheet_name", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "contract_start_date", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "contract_end_date", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "settlement_cycle", columnType: "TEXT"},
			{tableName: "fin_cost_settlements", columnName: "settlement_unit_price", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "source_report_type", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "source_sheet_name", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "quantity", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "contract_start_date", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "contract_end_date", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "settlement_cycle", columnType: "TEXT"},
			{tableName: "fin_fund_income", columnName: "settlement_unit_price", columnType: "TEXT"},
		}
		for _, upgrade := range sqliteColumnUpgrades {
			if err := ensureSQLiteColumn(ctx, db, upgrade.tableName, upgrade.columnName, upgrade.columnType); err != nil {
				return err
			}
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
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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
			account_code VARCHAR(64),
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema, schema),
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
			contract_start_date TEXT,
			contract_end_date TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, schema, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_contracts_name ON %s.fin_contracts(customer_name, contract_content)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_revenue_settlements_contract_period ON %s.fin_revenue_settlements(contract_id, year_month)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_cost_settlements_contract_period ON %s.fin_cost_settlements(contract_id, year_month)`, schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_fin_fund_income_contract_period ON %s.fin_fund_income(contract_id, year_month)`, schema),
		fmt.Sprintf(`COMMENT ON TABLE %s.fin_revenue_settlements IS 'DEPRECATED: 暂停使用，合同收入统一以 fin_fund_income 为准；代码已停止读取该表'`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_balance_detail ADD COLUMN IF NOT EXISTS opening_period TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_contracts ADD COLUMN IF NOT EXISTS contract_start_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_contracts ADD COLUMN IF NOT EXISTS contract_end_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_contracts ADD COLUMN IF NOT EXISTS settlement_cycle TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS invoice_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS paid_amount NUMERIC(18,2)`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS source_report_type TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS source_sheet_name TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS contract_start_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS contract_end_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS settlement_cycle TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_cost_settlements ADD COLUMN IF NOT EXISTS settlement_unit_price TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS source_report_type TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS source_sheet_name TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS quantity TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS contract_start_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS contract_end_date TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS settlement_cycle TEXT`, schema),
		fmt.Sprintf(`ALTER TABLE IF EXISTS %s.fin_fund_income ADD COLUMN IF NOT EXISTS settlement_unit_price TEXT`, schema),
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
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan sqlite table info %s: %w", tableName, err)
		}
		if strings.EqualFold(strings.TrimSpace(name), columnName) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate sqlite table info %s: %w", tableName, err)
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, columnName, columnType)); err != nil {
		return fmt.Errorf("add sqlite column %s.%s: %w", tableName, columnName, err)
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
