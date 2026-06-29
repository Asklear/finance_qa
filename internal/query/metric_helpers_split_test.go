package query

import (
	"testing"

	"financeqa/internal/accounting"
)

func TestDetectRequestedMetricsFallsBackToSingleCoreMetric(t *testing.T) {
	if got := detectRequestedMetrics("2026年3月净利润是多少"); len(got) != 1 || got[0] != "利润" {
		t.Fatalf("detectRequestedMetrics(net profit) = %v, want [利润]", got)
	}
	if got := detectRequestedMetrics("2026年3月收入、成本、利润分别是多少"); len(got) != 3 || got[0] != "收入" || got[1] != "成本" || got[2] != "利润" {
		t.Fatalf("detectRequestedMetrics(multi) = %v, want [收入 成本 利润]", got)
	}
}

func TestDetectRequestedMetricsMapsCustomerUnpaidPhrasesToContractARAP(t *testing.T) {
	cases := []struct {
		question string
		want     string
	}{
		{question: "从25年10月到今年4月底 客户未付款金额多少", want: "已开票未回款"},
		{question: "从2025年10月起至今，已开票但还没回款的项目金额是多少？", want: "已开票未回款"},
		{question: "含未开票未付款多少", want: "应收"},
		{question: "那其妙三四月份的应收未收账款是多少", want: "应收"},
	}

	for _, tc := range cases {
		if got := detectRequestedMetrics(tc.question); len(got) != 1 || got[0] != tc.want {
			t.Fatalf("detectRequestedMetrics(%q) = %v, want [%s]", tc.question, got, tc.want)
		}
	}
}

func TestDetectRequestedMetricsSeparatesProjectPayableAggregateFromInvoiceRoster(t *testing.T) {
	cases := []struct {
		question string
		want     string
	}{
		{question: "按项目成本口径，从2025年10月起到上一个完整自然月月底未付款合计多少？", want: "应付"},
		{question: "25年至26年未付款的项目及对应金额有哪些？", want: "已收票未付款"},
		{question: "按项目口径应付未付还有多少？", want: "应付"},
		{question: "供应商已收票但未付款还有多少？", want: "已收票未付款"},
	}

	for _, tc := range cases {
		if got := detectRequestedMetrics(tc.question); len(got) != 1 || got[0] != tc.want {
			t.Fatalf("detectRequestedMetrics(%q) = %v, want [%s]", tc.question, got, tc.want)
		}
	}
}

func TestShouldUseHRBreakdownRequiresBreakdownSignals(t *testing.T) {
	cfg := getRuleConfig()
	if !shouldUseHRBreakdown("2026年3月人力成本多少？工资、社保、公积金分别是多少？", cfg) {
		t.Fatalf("shouldUseHRBreakdown() = false, want true for explicit breakdown question")
	}
	if shouldUseHRBreakdown("2026年3月利润多少？", cfg) {
		t.Fatalf("shouldUseHRBreakdown() = true, want false for unrelated question")
	}
}

func TestPickMetricValueUsesExpectedCashAndAccrualFields(t *testing.T) {
	dual := &accounting.DualPerspective{
		Cash: accounting.CashPerspective{
			Income:  100,
			Expense: 70,
			Net:     30,
		},
		Accrual: accounting.AccrualPerspective{
			Revenue:   110,
			TotalCost: 80,
			Profit:    30,
		},
	}

	cash, accrual := pickMetricValue("利润", dual)
	if cash != 30 || accrual != 30 {
		t.Fatalf("pickMetricValue(利润) = %v/%v, want 30/30", cash, accrual)
	}
	cash, accrual = pickMetricValue("成本", dual)
	if cash != 70 || accrual != 80 {
		t.Fatalf("pickMetricValue(成本) = %v/%v, want 70/80", cash, accrual)
	}
	cash, accrual = pickMetricValue("收入", dual)
	if cash != 100 || accrual != 110 {
		t.Fatalf("pickMetricValue(收入) = %v/%v, want 100/110", cash, accrual)
	}
}
