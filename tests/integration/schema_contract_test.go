package integration_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	dbpkg "financeqa/internal/db"
	"financeqa/internal/support"
)

func TestFinanceDBSchemaContract(t *testing.T) {
	if os.Getenv("FINANCEQA_RUN_LIVE_DB_TESTS") != "1" {
		t.Skip("set FINANCEQA_RUN_LIVE_DB_TESTS=1 to run live database schema contract")
	}

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	_ = support.LoadDotEnv(filepath.Join(root, ".env"))
	_ = support.LoadDotEnv("/root/finance_qa/.env")
	dbPath := support.DefaultDBPath(root)
	if dbPath == "" {
		t.Skip("database is not configured; skipping schema contract test")
	}

	db, err := dbpkg.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open configured db: %v", err)
	}
	defer db.Close()

	required := map[string][]string{
		"income_statement": {"company", "period", "item_name", "current_amount"},
		"journal":          {"company", "period", "voucher_date", "account_code", "account_name", "summary", "direction", "amount", "debit_amount", "credit_amount", "counterparty"},
		"bank_statement":   {"company", "transaction_date", "debit_amount", "credit_amount", "summary", "counterparty_name"},
		"balance_sheet":    {"company", "period", "account_code", "account_name", "opening_balance", "closing_balance"},
	}

	for table, cols := range required {
		got := tableColumns(t, db, table)
		for _, c := range cols {
			if _, ok := got[c]; !ok {
				t.Fatalf("schema contract mismatch: table=%s missing column=%s (got=%v)", table, c, mapKeys(got))
			}
		}
	}
}

func tableColumns(t *testing.T, db *sql.DB, table string) map[string]struct{} {
	t.Helper()
	rows, err := db.Query("SELECT * FROM " + table + " LIMIT 0")
	if err != nil {
		t.Fatalf("probe columns for %s: %v", table, err)
	}
	defer rows.Close()

	cols := map[string]struct{}{}
	names, err := rows.Columns()
	if err != nil {
		t.Fatalf("read columns for %s: %v", table, err)
	}
	for _, name := range names {
		cols[name] = struct{}{}
	}
	if len(cols) == 0 {
		t.Fatalf("table %s not found or has no columns", table)
	}
	return cols
}

func mapKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
