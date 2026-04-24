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
