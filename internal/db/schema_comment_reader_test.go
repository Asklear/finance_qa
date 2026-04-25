package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLoadTableColumnCommentsSQLite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "column-comments.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, `
CREATE TABLE meta_column_comments (
	table_name TEXT NOT NULL,
	column_name TEXT NOT NULL,
	comment TEXT,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (table_name, column_name)
)`); err != nil {
		t.Fatalf("create meta_column_comments: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO meta_column_comments(table_name, column_name, comment)
VALUES ('fin_fund_income', 'settlement_amount', '老板口径收入结算金额')
`); err != nil {
		t.Fatalf("seed column comment: %v", err)
	}

	comments, err := LoadTableColumnComments(ctx, db, dbPath, []string{"fin_fund_income"})
	if err != nil {
		t.Fatalf("load column comments: %v", err)
	}

	if got := comments["fin_fund_income"]["settlement_amount"]; got != "老板口径收入结算金额" {
		t.Fatalf("settlement_amount comment = %q", got)
	}
	if got := comments["fin_fund_income"]["received_amount"]; got != "实际回款金额" {
		t.Fatalf("received_amount should fall back to default comment, got %q", got)
	}
}

func TestLoadTableColumnCommentsReturnsDefaultsWhenMetadataMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "missing-column-comments.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	comments, err := LoadTableColumnComments(ctx, db, dbPath, []string{"fin_cost_settlements", "unknown_table"})
	if err != nil {
		t.Fatalf("load column comments without metadata table: %v", err)
	}

	if got := comments["fin_cost_settlements"]["settlement_amount"]; got != "成本结算金额" {
		t.Fatalf("fin_cost_settlements.settlement_amount default comment = %q", got)
	}
	if _, ok := comments["unknown_table"]; ok {
		t.Fatalf("unknown table should not get synthetic comments: %#v", comments["unknown_table"])
	}
}
