package analysis

import (
	"strings"
	"testing"
)

func TestVoucherCashOrderByClauseAvoidsSQLiteOnlyColumns(t *testing.T) {
	clause := strings.ToLower(voucherCashOrderByClause())
	if strings.Contains(clause, "rowid") {
		t.Fatalf("cash bridge order clause must not depend on rowid: %s", clause)
	}
	if !strings.Contains(clause, "voucher_no") || !strings.Contains(clause, "account_code") {
		t.Fatalf("cash bridge order clause should stay deterministic by voucher fields: %s", clause)
	}
}
