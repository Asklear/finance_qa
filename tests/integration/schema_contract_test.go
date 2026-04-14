package integration_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFinanceDBSchemaContract(t *testing.T) {
	dbPath, err := filepath.Abs("../../finance.db")
	if err != nil {
		t.Fatalf("resolve finance.db path: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open finance.db: %v", err)
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
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("pragma table_info(%s): %v", table, err)
	}
	defer rows.Close()

	cols := map[string]struct{}{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma row for %s: %v", table, err)
		}
		cols[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err for %s: %v", table, err)
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
