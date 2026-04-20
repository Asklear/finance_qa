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

	schema := os.Getenv("FINANCEQA_PG_SCHEMA")
	if schema == "" {
		schema = "tenant_uhub"
	}

	ddls := []string{
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.balance_sheet AS SELECT * FROM %s.fin_balance_sheet`, schema, schema),
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.income_statement AS SELECT * FROM %s.fin_income_statement`, schema, schema),
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.balance_detail AS SELECT * FROM %s.fin_balance_detail`, schema, schema),
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.journal AS SELECT * FROM %s.fin_journal`, schema, schema),
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.bank_statement AS SELECT * FROM %s.fin_bank_statement`, schema, schema),
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.dimensions AS SELECT * FROM %s.fin_dimensions`, schema, schema),
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.dimension_members AS SELECT * FROM %s.fin_dimension_members`, schema, schema),
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.mapping_rules AS SELECT * FROM %s.fin_mapping_rules`, schema, schema),
		fmt.Sprintf(`CREATE OR REPLACE VIEW %s.table_idempotency_policies AS SELECT * FROM %s.fin_table_idempotency_policies`, schema, schema),
		fmt.Sprintf(`SET search_path TO %s, public`, schema),
	}
	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("apply postgres bootstrap sql: %w", err)
		}
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
	schema := os.Getenv("FINANCEQA_PG_SCHEMA")
	if schema == "" {
		schema = "tenant_uhub"
	}
	return dsn + " search_path=" + schema + ",public"
}
