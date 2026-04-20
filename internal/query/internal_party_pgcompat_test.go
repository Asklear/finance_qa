package query

import (
	"strings"
	"testing"
)

func TestLedgerVoucherOrderByClauseAvoidsSQLiteOnlyColumns(t *testing.T) {
	clause := strings.ToLower(ledgerVoucherOrderByClause())
	if strings.Contains(clause, "rowid") {
		t.Fatalf("ledger order clause must not depend on rowid: %s", clause)
	}
	if !strings.Contains(clause, "voucher_no") || !strings.Contains(clause, "account_code") {
		t.Fatalf("ledger order clause should stay deterministic by voucher fields: %s", clause)
	}
}
