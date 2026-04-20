package openitems

import (
	"strings"
	"testing"
)

func TestJournalColumnProbeQueryUsesSelectLimitZero(t *testing.T) {
	query, err := journalColumnProbeQuery("journal")
	if err != nil {
		t.Fatalf("journalColumnProbeQuery error: %v", err)
	}
	if got := strings.TrimSpace(query); got != "SELECT * FROM journal LIMIT 0" {
		t.Fatalf("probe query = %q, want SELECT * FROM journal LIMIT 0", got)
	}
	if strings.Contains(strings.ToLower(query), "pragma") {
		t.Fatalf("probe query should be cross-dialect, got %q", query)
	}
}

func TestOpenItemsJournalOrderByClauseAvoidsSQLiteOnlyColumns(t *testing.T) {
	clause := strings.ToLower(openItemsJournalOrderByClause())
	if strings.Contains(clause, "rowid") {
		t.Fatalf("order clause must not depend on rowid: %s", clause)
	}
	if !strings.Contains(clause, "voucher_no") || !strings.Contains(clause, "account_code") {
		t.Fatalf("order clause should still stabilize by business columns: %s", clause)
	}
}
