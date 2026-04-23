package db

import (
	"strings"
	"testing"
)

func TestRewriteSQL_RewritesLogicalTablesToFinTables(t *testing.T) {
	query := `
SELECT *
FROM journal j
JOIN bank_statement b ON b.counterparty_name = j.counterparty
LEFT JOIN balance_sheet bs ON bs.account_code = j.account_code
LEFT JOIN income_statement i ON i.period = bs.period
LEFT JOIN balance_detail bd ON bd.account_code = j.account_code
LEFT JOIN table_idempotency_policies p ON p.table_name = 'journal'
LEFT JOIN dimensions d ON d.company = bs.company
LEFT JOIN dimension_members dm ON dm.dimension_id = d.id
LEFT JOIN mapping_rules mr ON mr.dimension_id = d.id
WHERE j.company = ? AND b.transaction_date BETWEEN ? AND ?
`

	rewritten := rewriteSQL(query)

	wants := []string{
		"FROM fin_journal j",
		"JOIN fin_bank_statement b",
		"LEFT JOIN fin_balance_sheet bs",
		"LEFT JOIN fin_income_statement i",
		"LEFT JOIN fin_balance_detail bd",
		"LEFT JOIN fin_table_idempotency_policies p",
		"LEFT JOIN fin_dimensions d",
		"LEFT JOIN fin_dimension_members dm",
		"LEFT JOIN fin_mapping_rules mr",
		"j.company = $1",
		"BETWEEN $2 AND $3",
	}
	for _, want := range wants {
		if !strings.Contains(rewritten, want) {
			t.Fatalf("rewriteSQL() missing %q in rewritten SQL:\n%s", want, rewritten)
		}
	}
}

func TestRewriteSQL_DoesNotDoublePrefixFinTables(t *testing.T) {
	query := `SELECT * FROM fin_journal WHERE company = ?`
	rewritten := rewriteSQL(query)
	if strings.Count(rewritten, "fin_journal") != 1 {
		t.Fatalf("rewriteSQL() double-prefixed fin_journal: %s", rewritten)
	}
}
