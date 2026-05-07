package ingest

import (
	"reflect"
	"testing"

	"financeqa/internal/parser"
)

func TestImportHelperSanitizeColumnsDedupesAndWhitelists(t *testing.T) {
	t.Parallel()

	got := sanitizeColumns([]string{
		" summary ",
		"company",
		"not_allowed",
		"company",
		"credit_amount",
		"",
	})
	want := []string{"company", "credit_amount", "summary"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sanitizeColumns() = %#v, want %#v", got, want)
	}
}

func TestImportHelperNormalizedRecordKeyHandlesCommonValueTypes(t *testing.T) {
	t.Parallel()

	row := parser.Record{
		"company":        " 测试公司 ",
		"current_amount": 12,
		"credit_amount":  float32(3.5),
	}

	got := normalizedRecordKey(row, []string{"company", "current_amount", "credit_amount"})
	want := "测试公司\x1f3.500000\x1f12.000000"
	if got != want {
		t.Fatalf("normalizedRecordKey() = %q, want %q", got, want)
	}
}

func TestImportHelperNormalizeKeyValueHandlesScalarTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input any
		want  string
	}{
		{input: nil, want: ""},
		{input: []byte(" raw "), want: "raw"},
		{input: true, want: "1"},
		{input: false, want: "0"},
		{input: uint32(7), want: "7.000000"},
	}
	for _, tc := range cases {
		if got := normalizeKeyValue(tc.input); got != tc.want {
			t.Fatalf("normalizeKeyValue(%#v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestImportHelperRecordDateRangeUsesFallbackFieldsAndSkipsEmptyValues(t *testing.T) {
	t.Parallel()

	rows := []parser.Record{
		{"voucher_date": "", "date": "2026-03-02"},
		{"voucher_date": nil, "date": "2026-03-01"},
		{"voucher_date": "2026-03-05", "date": "2026-03-04"},
		{"voucher_date": "<nil>", "date": ""},
	}

	minDate, maxDate := recordDateRange(rows, "voucher_date", "date")
	if minDate != "2026-03-01" || maxDate != "2026-03-05" {
		t.Fatalf("recordDateRange() = %q to %q, want 2026-03-01 to 2026-03-05", minDate, maxDate)
	}
}

func TestImportHelperDefaultDedupeColumnsCoversKnownTables(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"income_statement": {"company", "period", "item_name"},
		"balance_sheet":    {"company", "period", "account_name"},
		"balance_detail":   {"company", "period", "account_code"},
	}
	for tableName, want := range cases {
		got := defaultDedupeColumns(tableName)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("defaultDedupeColumns(%q) = %#v, want %#v", tableName, got, want)
		}
	}
	if got := defaultDedupeColumns("unknown"); got != nil {
		t.Fatalf("defaultDedupeColumns(unknown) = %#v, want nil", got)
	}
}
