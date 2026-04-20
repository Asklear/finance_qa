package query_test

import (
	"strings"
	"testing"

	"financeqa/internal/query"
)

func TestClassifyCounterpartyFromEvidence(t *testing.T) {
	tests := []struct {
		name         string
		counterparty string
		evidence     []query.LedgerEvidence
		wantRole     query.CounterpartyRole
	}{
		{
			name:         "customer by receivable collection and sales evidence",
			counterparty: "金程",
			evidence: []query.LedgerEvidence{
				{Source: "bank_statement", Counterparty: "金程", CreditAmount: 1130, Summary: "历史应收回款"},
				{Source: "journal", Counterparty: "金程", AccountCode: "1122", AccountName: "应收账款", Summary: "回款冲销"},
				{Source: "journal", Counterparty: "金程", AccountCode: "6001", AccountName: "主营业务收入", Summary: "销售收入"},
			},
			wantRole: query.CounterpartyCustomer,
		},
		{
			name:         "supplier by payable and cost evidence",
			counterparty: "林悦",
			evidence: []query.LedgerEvidence{
				{Source: "bank_statement", Counterparty: "林悦", DebitAmount: 1130, Summary: "供应商付款"},
				{Source: "journal", Counterparty: "林悦", AccountCode: "2202", AccountName: "应付账款", Summary: "结算供应商"},
				{Source: "journal", Counterparty: "林悦", AccountCode: "5001", AccountName: "主营业务成本", Summary: "采购成本"},
			},
			wantRole: query.CounterpartySupplier,
		},
		{
			name:         "employee by payroll and reimbursement evidence",
			counterparty: "汇智",
			evidence: []query.LedgerEvidence{
				{Source: "journal", Counterparty: "汇智", AccountCode: "2211", AccountName: "应付职工薪酬", Summary: "工资"},
				{Source: "journal", Counterparty: "汇智", AccountCode: "6601", AccountName: "管理费用", Summary: "报销差旅"},
			},
			wantRole: query.CounterpartyEmployee,
		},
		{
			name:         "mixed when customer and supplier evidence both exist",
			counterparty: "某混合往来",
			evidence: []query.LedgerEvidence{
				{Source: "journal", Counterparty: "某混合往来", AccountCode: "1122", AccountName: "应收账款", Summary: "销售回款"},
				{Source: "journal", Counterparty: "某混合往来", AccountCode: "2202", AccountName: "应付账款", Summary: "采购结算"},
			},
			wantRole: query.CounterpartyMixed,
		},
		{
			name:         "unknown when no meaningful evidence",
			counterparty: "无名对手方",
			evidence:     nil,
			wantRole:     query.CounterpartyUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := query.ClassifyCounterparty(tc.counterparty, tc.evidence)
			if got.Role != tc.wantRole {
				t.Fatalf("ClassifyCounterparty role = %s, want %s; scores=%v signals=%v", got.Role, tc.wantRole, got.Scores, got.Signals)
			}
			if tc.wantRole != query.CounterpartyUnknown && got.Confidence <= 0 {
				t.Fatalf("expected positive confidence, got %.3f", got.Confidence)
			}
		})
	}
}

func TestClassifyCounterpartyUsesLedgerEvidenceNotNetFlowOnly(t *testing.T) {
	// 这组证据的净流向是正的，但分类仍然要看分录科目，不是只看净流向。
	evidence := []query.LedgerEvidence{
		{Source: "bank_statement", Counterparty: "飞未", CreditAmount: 1000, Summary: "回款"},
		{Source: "journal", Counterparty: "飞未", AccountCode: "1122", AccountName: "应收账款", Summary: "历史应收回款"},
		{Source: "journal", Counterparty: "飞未", AccountCode: "2202", AccountName: "应付账款", Summary: "采购结算"},
	}
	got := query.ClassifyCounterparty("飞未", evidence)
	if got.Role != query.CounterpartyMixed {
		t.Fatalf("expected mixed classification driven by ledger evidence, got %s", got.Role)
	}
	if !containsAny(strings.Join(got.Signals, ","), []string{"customer", "supplier"}) {
		t.Fatalf("expected both customer and supplier signals, got %v", got.Signals)
	}
}

func TestClassifyCounterpartyRecognizesSupplierFromPrepaymentServiceFeeAndInputTax(t *testing.T) {
	evidence := []query.LedgerEvidence{
		{Source: "journal", Counterparty: "汇智", AccountCode: "112301", AccountName: "预付账款", Summary: "转账南京汇智互娱教育科技有限公司", DebitAmount: 53750},
		{Source: "journal", Counterparty: "汇智", AccountCode: "66022304", AccountName: "服务费", Summary: "收到南京汇智互娱教育科技有限公司发票", DebitAmount: 50707.55},
		{Source: "journal", Counterparty: "汇智", AccountCode: "22210101", AccountName: "进项税额", Summary: "收到南京汇智互娱教育科技有限公司发票", DebitAmount: 3042.45},
	}

	got := query.ClassifyCounterparty("汇智", evidence)
	if got.Role != query.CounterpartySupplier {
		t.Fatalf("expected supplier classification from prepayment/service fee/input tax evidence, got %s; scores=%v signals=%v", got.Role, got.Scores, got.Signals)
	}
	if !containsAny(strings.Join(got.Signals, ","), []string{"supplier_strong", "supplier"}) {
		t.Fatalf("expected supplier signal, got %v", got.Signals)
	}
}

func TestCounterpartyClassifierUsesConfigurableRoleLexicon(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "counterparty": {
    "roles": {
      "customer": ["客证"],
      "supplier": ["供证"],
      "employee": ["员证"]
    }
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	tests := []struct {
		name     string
		evidence []query.LedgerEvidence
		want     query.CounterpartyRole
	}{
		{
			name: "customer",
			evidence: []query.LedgerEvidence{
				{Source: "journal", Counterparty: "甲方", Summary: "客证"},
			},
			want: query.CounterpartyCustomer,
		},
		{
			name: "supplier",
			evidence: []query.LedgerEvidence{
				{Source: "journal", Counterparty: "乙方", Summary: "供证"},
			},
			want: query.CounterpartySupplier,
		},
		{
			name: "employee",
			evidence: []query.LedgerEvidence{
				{Source: "journal", Counterparty: "丙方", Summary: "员证"},
			},
			want: query.CounterpartyEmployee,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := query.ClassifyCounterparty("示例主体", tc.evidence)
			if got.Role != tc.want {
				t.Fatalf("ClassifyCounterparty role = %s, want %s; scores=%v signals=%v", got.Role, tc.want, got.Scores, got.Signals)
			}
		})
	}
}

func TestCounterpartyClassifierUsesConfigurableTaxLexicon(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "counterparty": {
    "roles": {
      "customer": ["客证"],
      "supplier": ["供证"]
    },
    "tax": {
      "output": ["销证"],
      "input": ["进证"]
    }
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	report := query.NormalizeTax("示例主体", []query.LedgerEvidence{
		{Source: "journal", Counterparty: "示例主体", Summary: "客证", CreditAmount: 100},
		{Source: "journal", Counterparty: "示例主体", Summary: "销证", CreditAmount: 13},
		{Source: "journal", Counterparty: "示例主体", Summary: "供证", DebitAmount: 80},
		{Source: "journal", Counterparty: "示例主体", Summary: "进证", DebitAmount: 8},
	})

	if report.Output.TaxAmount != 13 {
		t.Fatalf("output tax = %v, want 13", report.Output.TaxAmount)
	}
	if report.Input.TaxAmount != 8 {
		t.Fatalf("input tax = %v, want 8", report.Input.TaxAmount)
	}
	if !containsAny(strings.Join(report.Output.Signals, ","), []string{"output_tax:销证"}) {
		t.Fatalf("expected configurable output tax signal, got %v", report.Output.Signals)
	}
	if !containsAny(strings.Join(report.Input.Signals, ","), []string{"input_tax:进证"}) {
		t.Fatalf("expected configurable input tax signal, got %v", report.Input.Signals)
	}
}

func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
