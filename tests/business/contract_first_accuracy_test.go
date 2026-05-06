//go:build accuracy

package business

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"testing"
	"time"

	"financeqa/tests/testutil"
)

func TestContractFirstCompanyQ1TotalsMatchIndependentSQL(t *testing.T) {
	t.Parallel()
	testutil.RunLiveDBCase(t, func() {
		engine := testutil.RequireLiveDBEngine(t, testutil.DefaultBusinessCompany)
		db := testutil.RequireLiveSQLDB(t)

		expected := queryCompanyContractTruth(t, db, "2026-01", "2026-03")
		start := time.Now()
		res := engine.Query("2026年第一季度收入、成本、利润分别是多少")
		t.Logf("elapsed=%s success=%v sql=%d logs=%d", time.Since(start).Round(time.Millisecond), res.Success, len(res.ExecutedSQL), len(res.CalculationLogs))

		requireSuccessfulTrace(t, res.Success, res.Message, len(res.ExecutedSQL), len(res.CalculationLogs))
		accountView := requireMap(t, res.Data["account_view"], "data.account_view")
		assertAmount(t, "account_view.营收", readFloat(t, accountView, "营收"), expected.RevenueSettlement)
		assertAmount(t, "account_view.合同成本", readFloat(t, accountView, "合同成本"), expected.CostSettlement)
		assertAmount(t, "account_view.利润", readFloat(t, accountView, "利润"), expected.Profit)
		assertAmount(t, "contract_summary.revenue_settlement", readFloat(t, requireMap(t, res.Data["contract_summary"], "data.contract_summary"), "revenue_settlement"), expected.RevenueSettlement)
		if !strings.Contains(res.Message, "老板口径先看合同/项目汇总") {
			t.Fatalf("message should state contract/project first perspective, got: %s", res.Message)
		}
		requireSourceLineage(t, res.Data, res.Message)
	})
}

func TestContractFirstMonthlyTotalsMatchIndependentSQL(t *testing.T) {
	t.Parallel()
	testutil.RunLiveDBCase(t, func() {
		engine := testutil.RequireLiveDBEngine(t, testutil.DefaultBusinessCompany)
		db := testutil.RequireLiveSQLDB(t)

		expected := queryCompanyContractTruth(t, db, "2026-02", "2026-02")
		res := engine.Query("2026年2月收入、成本、利润分别是多少")

		requireSuccessfulTrace(t, res.Success, res.Message, len(res.ExecutedSQL), len(res.CalculationLogs))
		accountView := requireMap(t, res.Data["account_view"], "data.account_view")
		assertAmount(t, "account_view.营收", readFloat(t, accountView, "营收"), expected.RevenueSettlement)
		assertAmount(t, "account_view.合同成本", readFloat(t, accountView, "合同成本"), expected.CostSettlement)
		assertAmount(t, "account_view.利润", readFloat(t, accountView, "利润"), expected.Profit)
		requireSourceLineage(t, res.Data, res.Message)
	})
}

func TestCustomerContractEntityTotalsMatchIndependentSQL(t *testing.T) {
	t.Parallel()
	testutil.RunLiveDBCase(t, func() {
		engine := testutil.RequireLiveDBEngine(t, testutil.DefaultBusinessCompany)
		db := testutil.RequireLiveSQLDB(t)

		entity := "飞未云科（深圳）技术有限公司"
		expected := queryCustomerContractTruth(t, db, "2026-01", "2026-05", entity)
		res := engine.Query("飞未云科2026年累计销售额多少？")

		requireSuccessfulTrace(t, res.Success, res.Message, len(res.ExecutedSQL), len(res.CalculationLogs))
		if got, _ := res.Data["role"].(string); got != "customer_contract" {
			t.Fatalf("role=%q, want customer_contract", got)
		}
		accountView := requireMap(t, res.Data["account_view"], "data.account_view")
		cashView := requireMap(t, res.Data["cash_view"], "data.cash_view")
		assertAmount(t, "account_view.settlement_amount", readFloat(t, accountView, "settlement_amount"), expected.RevenueSettlement)
		assertAmount(t, "account_view.invoice_amount", readFloat(t, accountView, "invoice_amount"), expected.RevenueInvoiced)
		assertAmount(t, "cash_view.received_amount", readFloat(t, cashView, "received_amount"), expected.RevenueReceived)
		requireSourceLineage(t, res.Data, res.Message)
	})
}

func TestSupplierContractEntityTotalsMatchIndependentSQL(t *testing.T) {
	t.Parallel()
	testutil.RunLiveDBCase(t, func() {
		engine := testutil.RequireLiveDBEngine(t, testutil.DefaultBusinessCompany)
		db := testutil.RequireLiveSQLDB(t)

		entity := "南京林悦智能科技有限公司"
		expected := querySupplierContractTruth(t, db, "2026-03", "2026-03", entity)
		res := engine.Query("南京林悦智能科技有限公司3月应付账款多少？")

		requireSuccessfulTrace(t, res.Success, res.Message, len(res.ExecutedSQL), len(res.CalculationLogs))
		if got, _ := res.Data["role"].(string); got != "supplier_contract" {
			t.Fatalf("role=%q, want supplier_contract", got)
		}
		accountView := requireMap(t, res.Data["account_view"], "data.account_view")
		cashView := requireMap(t, res.Data["cash_view"], "data.cash_view")
		assertAmount(t, "account_view.contract_cost", readFloat(t, accountView, "contract_cost"), expected.CostSettlement)
		assertAmount(t, "cash_view.cash_paid_amount", readFloat(t, cashView, "cash_paid_amount"), expected.CostPaid)
		requireSourceLineage(t, res.Data, res.Message)
	})
}

type contractTruth struct {
	RevenueSettlement float64
	RevenueReceived   float64
	RevenueInvoiced   float64
	CostSettlement    float64
	CostPaid          float64
	CostInvoiced      float64
	Profit            float64
}

func queryCompanyContractTruth(t *testing.T, db *sql.DB, from, to string) contractTruth {
	t.Helper()

	revenue := queryFundIncomeTruth(t, db, from, to, "")
	cost := queryCostSettlementTruth(t, db, from, to, "")
	revenue.CostSettlement = cost.CostSettlement
	revenue.CostPaid = cost.CostPaid
	revenue.CostInvoiced = cost.CostInvoiced
	revenue.Profit = round2(revenue.RevenueSettlement - revenue.CostSettlement)
	return revenue
}

func queryCustomerContractTruth(t *testing.T, db *sql.DB, from, to, entity string) contractTruth {
	t.Helper()
	return queryFundIncomeTruth(t, db, from, to, "%"+entity+"%")
}

func querySupplierContractTruth(t *testing.T, db *sql.DB, from, to, entity string) contractTruth {
	t.Helper()
	cost := queryCostSettlementTruth(t, db, from, to, "%"+entity+"%")
	var paid float64
	if err := db.QueryRowContext(context.Background(), `
SELECT COALESCE(SUM(debit_amount), 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND counterparty_name LIKE ?
  AND transaction_date BETWEEN ? AND ?
`, testutil.DefaultBusinessCompany, testutil.DefaultBusinessCompany, "%"+entity+"%", from+"-01", monthEndDay(to)).Scan(&paid); err != nil {
		t.Fatalf("query supplier cash paid: %v", err)
	}
	cost.CostPaid = round2(paid)
	return cost
}

func queryFundIncomeTruth(t *testing.T, db *sql.DB, from, to, like string) contractTruth {
	t.Helper()

	var out contractTruth
	args := []any{from, to}
	filter := ""
	if strings.TrimSpace(like) != "" {
		filter = ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}
	if err := db.QueryRowContext(context.Background(), `
SELECT COALESCE(SUM(f.settlement_amount), 0),
       COALESCE(SUM(f.received_amount), 0),
       COALESCE(SUM(f.invoice_amount), 0)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.year_month BETWEEN ? AND ?`+filter, args...).Scan(&out.RevenueSettlement, &out.RevenueReceived, &out.RevenueInvoiced); err != nil {
		t.Fatalf("query direct fund income truth: %v", err)
	}

	groupArgs := []any{from, to}
	groupFilter := ""
	if strings.TrimSpace(like) != "" {
		groupFilter = ` AND g.customer_name LIKE ?`
		groupArgs = append(groupArgs, like)
	}
	var groupSettlement, groupReceived, groupInvoice float64
	if err := db.QueryRowContext(context.Background(), `
SELECT COALESCE(SUM(g.settlement_amount), 0),
       COALESCE(SUM(g.received_amount), 0),
       COALESCE(SUM(g.invoice_amount), 0)
FROM fin_fund_income_groups g
WHERE g.year_month BETWEEN ? AND ?`+groupFilter, groupArgs...).Scan(&groupSettlement, &groupReceived, &groupInvoice); err != nil {
		t.Fatalf("query grouped fund income truth: %v", err)
	}

	out.RevenueSettlement = round2(out.RevenueSettlement + groupSettlement)
	out.RevenueReceived = round2(out.RevenueReceived + groupReceived)
	out.RevenueInvoiced = round2(out.RevenueInvoiced + groupInvoice)
	return out
}

func queryCostSettlementTruth(t *testing.T, db *sql.DB, from, to, like string) contractTruth {
	t.Helper()

	var out contractTruth
	args := []any{from, to}
	filter := ""
	if strings.TrimSpace(like) != "" {
		filter = ` AND (c.customer_name LIKE ? OR c.contract_content LIKE ?)`
		args = append(args, like, like)
	}
	if err := db.QueryRowContext(context.Background(), `
SELECT COALESCE(SUM(cs.settlement_amount), 0),
       COALESCE(SUM(cs.paid_amount), 0),
       COALESCE(SUM(cs.invoice_amount), 0)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE cs.year_month BETWEEN ? AND ?`+filter, args...).Scan(&out.CostSettlement, &out.CostPaid, &out.CostInvoiced); err != nil {
		t.Fatalf("query direct cost settlement truth: %v", err)
	}

	groupArgs := []any{from, to}
	groupFilter := ""
	if strings.TrimSpace(like) != "" {
		groupFilter = ` AND g.customer_name LIKE ?`
		groupArgs = append(groupArgs, like)
	}
	var groupSettlement, groupPaid, groupInvoice float64
	if err := db.QueryRowContext(context.Background(), `
SELECT COALESCE(SUM(g.settlement_amount), 0),
       COALESCE(SUM(g.paid_amount), 0),
       COALESCE(SUM(g.invoice_amount), 0)
FROM fin_cost_settlement_groups g
WHERE g.year_month BETWEEN ? AND ?`+groupFilter, groupArgs...).Scan(&groupSettlement, &groupPaid, &groupInvoice); err != nil {
		t.Fatalf("query grouped cost settlement truth: %v", err)
	}

	out.CostSettlement = round2(out.CostSettlement + groupSettlement)
	out.CostPaid = round2(out.CostPaid + groupPaid)
	out.CostInvoiced = round2(out.CostInvoiced + groupInvoice)
	return out
}

func requireSuccessfulTrace(t *testing.T, success bool, message string, sqlCount, logCount int) {
	t.Helper()

	if !success {
		t.Fatalf("query failed: %s", message)
	}
	if sqlCount == 0 || logCount == 0 {
		t.Fatalf("missing trace bundle: sql=%d logs=%d", sqlCount, logCount)
	}
}

func requireSourceLineage(t *testing.T, data map[string]any, message string) {
	t.Helper()

	sourceNote, _ := data["source_note"].(string)
	updateNote, _ := data["source_update_note"].(string)
	if strings.TrimSpace(sourceNote) == "" || !strings.Contains(message, "来源：") {
		t.Fatalf("missing source note: source_note=%q message=%s", sourceNote, message)
	}
	if strings.TrimSpace(updateNote) == "" || !strings.Contains(message, "来源更新时间：") {
		t.Fatalf("missing source update note: source_update_note=%q message=%s", updateNote, message)
	}
}

func requireMap(t *testing.T, v any, label string) map[string]any {
	t.Helper()

	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s expected map, got %T (%v)", label, v, v)
	}
	return m
}

func readFloat(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()

	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q in %#v", key, m)
	}
	switch num := v.(type) {
	case float64:
		return num
	case int:
		return float64(num)
	default:
		t.Fatalf("key %q expected number, got %T (%v)", key, v, v)
		return 0
	}
}

func assertAmount(t *testing.T, label string, got, want float64) {
	t.Helper()

	if math.Abs(got-want) > 0.01 {
		t.Fatalf("%s = %.2f, want %.2f", label, got, want)
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func monthEndDay(period string) string {
	parts := strings.Split(period, "-")
	if len(parts) != 2 {
		return period
	}
	switch parts[1] {
	case "01", "03", "05", "07", "08", "10", "12":
		return period + "-31"
	case "04", "06", "09", "11":
		return period + "-30"
	default:
		return period + "-28"
	}
}
