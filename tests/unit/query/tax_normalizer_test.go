package query_test

import (
	"strings"
	"testing"

	"financeqa/internal/query"
)

func TestNormalizeTaxOutputWithHistoricalReceivableCollection(t *testing.T) {
	evidence := []query.LedgerEvidence{
		{Source: "bank_statement", Counterparty: "金程", CreditAmount: 1130, Summary: "历史应收回款"},
		{Source: "journal", Counterparty: "金程", AccountCode: "1122", AccountName: "应收账款", Summary: "回款冲销"},
		{Source: "journal", Counterparty: "金程", AccountCode: "6001", AccountName: "主营业务收入", CreditAmount: 1000, Summary: "销售收入"},
		{Source: "journal", Counterparty: "金程", AccountCode: "222101", AccountName: "应交税费-销项税", CreditAmount: 130, Summary: "销项税"},
	}

	report := query.NormalizeTax("金程", evidence)
	if report.Role != query.CounterpartyCustomer {
		t.Fatalf("role = %s, want customer", report.Role)
	}
	if !report.Output.Included {
		t.Fatalf("output breakdown should be included for customer-side evidence")
	}
	if report.Output.CashAmount != 1130 {
		t.Fatalf("cash amount = %.2f, want 1130", report.Output.CashAmount)
	}
	if report.Output.AccrualAmount != 1000 {
		t.Fatalf("accrual amount = %.2f, want 1000", report.Output.AccrualAmount)
	}
	if report.Output.TaxAmount != 130 {
		t.Fatalf("tax amount = %.2f, want 130", report.Output.TaxAmount)
	}
	if !strings.Contains(report.Output.DifferenceReason, "历史应收回款") {
		t.Fatalf("difference reason should explain historical receivable collection, got %q", report.Output.DifferenceReason)
	}
	if strings.Contains(report.Output.DifferenceReason, "结算月份") {
		t.Fatalf("difference reason must not hard-code settlement month, got %q", report.Output.DifferenceReason)
	}
}

func TestNormalizeTaxInputForSupplierCostEvidence(t *testing.T) {
	evidence := []query.LedgerEvidence{
		{Source: "bank_statement", Counterparty: "林悦", DebitAmount: 1130, Summary: "供应商付款"},
		{Source: "journal", Counterparty: "林悦", AccountCode: "2202", AccountName: "应付账款", Summary: "供应商结算"},
		{Source: "journal", Counterparty: "林悦", AccountCode: "5001", AccountName: "主营业务成本", DebitAmount: 1000, Summary: "采购成本"},
		{Source: "journal", Counterparty: "林悦", AccountCode: "222102", AccountName: "应交税费-进项税", DebitAmount: 130, Summary: "进项税"},
	}

	report := query.NormalizeTax("林悦", evidence)
	if report.Role != query.CounterpartySupplier {
		t.Fatalf("role = %s, want supplier", report.Role)
	}
	if report.Output.Included {
		t.Fatalf("supplier/cost evidence must not enter output-income difference list")
	}
	if report.Input.CashAmount != 1130 {
		t.Fatalf("input cash amount = %.2f, want 1130", report.Input.CashAmount)
	}
	if report.Input.AccrualAmount != 1000 {
		t.Fatalf("input accrual amount = %.2f, want 1000", report.Input.AccrualAmount)
	}
	if report.Input.TaxAmount != 130 {
		t.Fatalf("input tax amount = %.2f, want 130", report.Input.TaxAmount)
	}
	if !strings.Contains(report.Input.DifferenceReason, "供应商付款") && !strings.Contains(report.Input.DifferenceReason, "成本") {
		t.Fatalf("difference reason should explain supplier/cost side, got %q", report.Input.DifferenceReason)
	}
}

func TestNormalizeOutputTaxPrefersTaxDifferenceOverGenericSettlementStory(t *testing.T) {
	evidence := []query.LedgerEvidence{
		{Source: "bank_statement", Counterparty: "飞未", CreditAmount: 1130, Summary: "回款"},
		{Source: "journal", Counterparty: "飞未", AccountCode: "6001", AccountName: "营业收入", CreditAmount: 1000, Summary: "销售收入"},
		{Source: "journal", Counterparty: "飞未", AccountCode: "222101", AccountName: "应交税费-销项税", CreditAmount: 130, Summary: "销项税"},
	}

	out := query.NormalizeOutputTax("飞未", evidence)
	if !out.Included {
		t.Fatalf("output tax normalization should include customer-side evidence")
	}
	if out.TaxAmount != 130 {
		t.Fatalf("tax amount = %.2f, want 130", out.TaxAmount)
	}
	if !strings.Contains(out.DifferenceReason, "销项税额") {
		t.Fatalf("difference reason should point to output VAT, got %q", out.DifferenceReason)
	}
}
