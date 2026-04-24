package query

import "testing"

func TestBuildCounterpartyAuditContextUsesSnapshotRoleAndComparisonFlags(t *testing.T) {
	classification := CounterpartyClassification{
		Confidence: 0.82,
		Signals:    []string{"bank_credit"},
	}
	snap := counterpartySnapshot{
		Role:             "customer",
		BankIn:           1200,
		RevenueNet:       1000,
		OutputVAT:        200,
		ComparisonBasis:  "vat_gap_only",
		DifferenceReason: "银行回款和账上收入差额主要是销项税。",
	}
	taxReport := TaxNormalizationReport{
		Counterparty: "飞未云科",
		Role:         CounterpartyCustomer,
	}

	ctx := buildCounterpartyAuditContext("飞未云科2026年2月收入多少", "飞未云科", "2026-02", "2026-02", snap, classification, taxReport, nil, false)

	if ctx.role != "customer" {
		t.Fatalf("role = %q, want %q", ctx.role, "customer")
	}
	if ctx.roleLabel != "（识别为[customer]）" {
		t.Fatalf("roleLabel = %q, want %q", ctx.roleLabel, "（识别为[customer]）")
	}
	if ctx.periodLabel != "2026-02" {
		t.Fatalf("periodLabel = %q, want %q", ctx.periodLabel, "2026-02")
	}
	if ctx.receiptPeriodLabel != "2026-02 " {
		t.Fatalf("receiptPeriodLabel = %q, want %q", ctx.receiptPeriodLabel, "2026-02 ")
	}
	if got := ctx.resultData["comparison_basis"]; got != "vat_gap_only" {
		t.Fatalf("comparison_basis = %v, want vat_gap_only", got)
	}
	taxBasis, ok := ctx.resultData["tax_basis"].(map[string]any)
	if !ok {
		t.Fatalf("tax_basis missing: %+v", ctx.resultData)
	}
	if got := taxBasis["comparison_allowed"]; got != true {
		t.Fatalf("comparison_allowed = %v, want true", got)
	}
	if len(ctx.logs) == 0 {
		t.Fatalf("expected logs to be populated")
	}
	if len(ctx.sqls) != 2 {
		t.Fatalf("sqls len = %d, want 2", len(ctx.sqls))
	}
}

func TestBuildCounterpartyReceiptsMessageIncludesHistoricalAndSubPeriodHints(t *testing.T) {
	ctx := counterpartyAuditContext{
		entity:             "辽宁金程信息科技有限公司",
		roleLabel:          "（识别为[customer]）",
		receiptPeriodLabel: "今年",
		snap: counterpartySnapshot{
			ComparisonBasis: "historical_receipt_and_current_revenue",
		},
	}

	got := buildCounterpartyReceiptsMessage(ctx, 7100, "2026-03", 2100)
	want := "[辽宁金程信息科技有限公司]（识别为[customer]）今年到账（银行含税口径） 7100.00 元；其中3月到账 2100.00 元。数据库能确认这类到账包含历史应收回款因素，不能直接当成当期新收入。"
	if got != want {
		t.Fatalf("buildCounterpartyReceiptsMessage() = %q, want %q", got, want)
	}
}

func TestBuildCounterpartyAuditContextUsesRetroYearRangeLabels(t *testing.T) {
	classification := CounterpartyClassification{Role: CounterpartyCustomer}
	ctx := buildCounterpartyAuditContext(
		"飞未云科这个主体目前更像客户、供应商还是混合往来？",
		"飞未云科",
		"2026-04",
		"2026-04",
		counterpartySnapshot{Role: "customer", RevenueNet: 1000},
		classification,
		TaxNormalizationReport{},
		nil,
		true,
	)

	if ctx.periodLabel != "2026-01~2026-04" {
		t.Fatalf("periodLabel = %q, want %q", ctx.periodLabel, "2026-01~2026-04")
	}
	if ctx.receiptPeriodLabel != "2026-01~2026-04 " {
		t.Fatalf("receiptPeriodLabel = %q, want %q", ctx.receiptPeriodLabel, "2026-01~2026-04 ")
	}
}
