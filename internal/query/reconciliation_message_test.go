package query

import (
	"strings"
	"testing"

	"financeqa/internal/accounting"
	"financeqa/internal/analysis"
)

func TestComposeBossReconciliationMessageUsesBridgeBreakdownAndNoHardcodedMonth(t *testing.T) {
	book := monthlyBookView{
		Revenue:   3106310.34,
		TotalCost: 2815018.91,
		NetProfit: 291291.55,
		Profit:    291291.55,
	}
	cash := &accounting.CashPerspective{
		Income:  2613554.53,
		Expense: 3247413.86,
		Net:     -633859.33,
	}
	bridge := &analysis.ProfitCashBridge{
		NetProfit:                   291291.55,
		Depreciation:                5810.76,
		ARIncrease:                  -252444.13,
		PrepaymentIncrease:          -126310.27,
		OtherReceivableIncrease:     141902.74,
		APIncrease:                  -269589.37,
		AdvanceReceiptIncrease:      -872178.58,
		PayrollIncrease:             31754.22,
		TaxBalanceIncrease:          -37003.11,
		FixedAssetPurchasePrincipal: 20796.46,
		EstimatedOperatingCash:      -633859.33,
		OperatingCashNet:            -405700.47,
		ExcludedCashNet:             -228158.86,
	}
	highlights := []counterpartySnapshot{
		{
			Name:            "南京林悦智能科技有限公司",
			ComparisonBasis: "supplier_payment_or_cost",
			BankOut:         1915915.19,
			BookCost:        447000.00,
			InputVAT:        31137.75,
		},
	}

	engine := &Engine{}
	msg := engine.composeBossReconciliationMessage("2026-03", book, "income_statement", cash, bridge, highlights)
	if !strings.Contains(msg, "利润调现金桥") {
		t.Fatalf("message should include bridge breakdown, got: %s", msg)
	}
	if !strings.Contains(msg, "其他应收款变动 -141902.74") {
		t.Fatalf("message should disclose other receivable bridge item, got: %s", msg)
	}
	if !strings.Contains(msg, "固定资产购置 -20796.46") {
		t.Fatalf("message should disclose fixed asset bridge item, got: %s", msg)
	}
	if strings.Contains(msg, "2 月付款") {
		t.Fatalf("message should not hardcode month label, got: %s", msg)
	}
	if strings.Contains(msg, "再看金额较大的对手方例子") {
		t.Fatalf("message should omit legacy highlight examples when bridge already closes the gap, got: %s", msg)
	}
	if strings.Contains(msg, "南京林悦智能科技有限公司") {
		t.Fatalf("message should stay focused on bridge explanation when bridge already closes the gap, got: %s", msg)
	}
}

func TestComposeBossReconciliationMessageDisclosesResidualGapBeforeHighlights(t *testing.T) {
	t.Setenv("FINANCEQA_RECONCILIATION_RESIDUAL_GAP_ESCALATION_AMOUNT", "100000")

	book := monthlyBookView{
		Revenue:   2485230.69,
		TotalCost: 2487987.85,
		NetProfit: -2756.97,
		Profit:    -2756.97,
	}
	cash := &accounting.CashPerspective{
		Income:  3940965.11,
		Expense: 2508796.67,
		Net:     1432168.44,
	}
	bridge := &analysis.ProfitCashBridge{
		NetProfit:              -2756.97,
		Depreciation:           6986.10,
		EstimatedOperatingCash: 2012410.96,
		OperatingCashNet:       1480206.35,
		ExcludedCashNet:        -47999.81,
		BankNetCash:            1432168.44,
	}
	highlights := []counterpartySnapshot{
		{
			Name:            "南京林悦智能科技有限公司",
			ComparisonBasis: "supplier_payment_or_cost",
			BankOut:         1648855.82,
			BookCost:        3383396.23,
			InputVAT:        116603.77,
		},
	}

	engine := &Engine{}
	msg := engine.composeBossReconciliationMessage("2026-02", book, "income_statement", cash, bridge, highlights)
	if !strings.Contains(msg, "当前这版利润调现金桥和银行卡净额还差") {
		t.Fatalf("message should disclose unresolved residual gap, got: %s", msg)
	}
	if !strings.Contains(msg, "当前只能先解释已识别部分") {
		t.Fatalf("message should downgrade conclusion when residual exceeds threshold, got: %s", msg)
	}
	if !strings.Contains(msg, "待核对") {
		t.Fatalf("message should explicitly mark unresolved residual for follow-up, got: %s", msg)
	}
	if !strings.Contains(msg, "再看金额较大的对手方例子") {
		t.Fatalf("message should still include highlights when residual remains, got: %s", msg)
	}
}

func TestComposeBossReconciliationMessageKeepsNormalResidualWordingBelowThreshold(t *testing.T) {
	t.Setenv("FINANCEQA_RECONCILIATION_RESIDUAL_GAP_ESCALATION_AMOUNT", "1000000")

	book := monthlyBookView{
		Revenue:   2485230.69,
		TotalCost: 2487987.85,
		NetProfit: -2756.97,
		Profit:    -2756.97,
	}
	cash := &accounting.CashPerspective{
		Income:  3940965.11,
		Expense: 2508796.67,
		Net:     1432168.44,
	}
	bridge := &analysis.ProfitCashBridge{
		NetProfit:              -2756.97,
		Depreciation:           6986.10,
		EstimatedOperatingCash: 2012410.96,
		OperatingCashNet:       1480206.35,
		ExcludedCashNet:        -47999.81,
		BankNetCash:            1432168.44,
	}
	highlights := []counterpartySnapshot{
		{
			Name:            "南京林悦智能科技有限公司",
			ComparisonBasis: "supplier_payment_or_cost",
			BankOut:         1648855.82,
			BookCost:        3383396.23,
			InputVAT:        116603.77,
		},
	}

	engine := &Engine{}
	msg := engine.composeBossReconciliationMessage("2026-02", book, "income_statement", cash, bridge, highlights)
	if strings.Contains(msg, "当前只能先解释已识别部分") {
		t.Fatalf("message should keep normal residual wording below threshold, got: %s", msg)
	}
	if strings.Contains(msg, "待核对") {
		t.Fatalf("message should not escalate unresolved residual below threshold, got: %s", msg)
	}
	if !strings.Contains(msg, "当前这版利润调现金桥和银行卡净额还差") {
		t.Fatalf("message should still disclose unresolved residual gap, got: %s", msg)
	}
}
