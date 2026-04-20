package query

import (
	"strings"
	"testing"
	"time"
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

func TestInternalBranchTransferLedgerQueryCastsVoucherDateToText(t *testing.T) {
	query := strings.ToLower(internalBranchTransferLedgerQuery())
	if strings.Contains(query, "trim(voucher_date)") {
		t.Fatalf("voucher_date must not be trimmed as text in pg-compatible sql: %s", query)
	}
	if !strings.Contains(query, "cast(voucher_date as text)") {
		t.Fatalf("voucher_date should be cast to text for cross-db scanning: %s", query)
	}
}

func TestParseAnchorDateValueSupportsRFC3339DateStrings(t *testing.T) {
	got, ok := parseAnchorDateValue("2026-03-31T00:00:00Z")
	if !ok {
		t.Fatalf("expected RFC3339 date string to parse")
	}
	want := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parsed anchor = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestCounterpartyNameCandidatesQueryAvoidsDistinctLengthOrderingTrap(t *testing.T) {
	query := strings.ToLower(counterpartyNameCandidatesQuery())
	if strings.Contains(query, "select distinct counterparty_name") && strings.Contains(query, "order by length(counterparty_name)") {
		t.Fatalf("pg-compatible counterparty lookup query must avoid DISTINCT + ORDER BY LENGTH trap: %s", query)
	}
	if !strings.Contains(query, "union") {
		t.Fatalf("counterparty lookup should combine compatible candidate sources: %s", query)
	}
}

func TestTrimEntityNoiseSuffixesRemovesQuestionVerbFragments(t *testing.T) {
	if got := trimEntityNoiseSuffixes("梁梦瑶报"); got != "梁梦瑶" {
		t.Fatalf("trimmed entity = %q, want %q", got, "梁梦瑶")
	}
	if got := trimEntityNoiseSuffixes("金程到账"); got != "金程" {
		t.Fatalf("trimmed entity = %q, want %q", got, "金程")
	}
}
