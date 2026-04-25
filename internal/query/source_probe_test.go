package query

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProbeContractRevenueCanAnswerQ1(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "probe-contract-revenue.sqlite")
	seedProbeContract(t, engine, "C001", "测试客户有限公司", "年度服务合同")
	for _, period := range []string{"2026-01", "2026-02", "2026-03"} {
		seedProbeFundIncome(t, engine, "C001", period, 100, 80, 100)
	}

	rewrite := RewriteBossQuery("2026年Q1收入是多少？", probeAnchor())
	probe := firstProbeResult(engine.ProbeBossSources(ctx, rewrite), BossSourceContractAggregate)

	if !probe.CanAnswer {
		t.Fatalf("CanAnswer = false, reason=%s", probe.MissingReason)
	}
	if probe.CoverageStatus != CoverageFull {
		t.Fatalf("CoverageStatus = %s, want %s", probe.CoverageStatus, CoverageFull)
	}
	if probe.RowCount != 3 {
		t.Fatalf("RowCount = %d, want 3", probe.RowCount)
	}
	if !containsString(probe.PrimaryTables, "fin_fund_income") {
		t.Fatalf("PrimaryTables = %#v, want fin_fund_income", probe.PrimaryTables)
	}
}

func TestProbeContractRevenueMissingPeriodFallsBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "probe-contract-missing.sqlite")
	seedProbeContract(t, engine, "C001", "测试客户有限公司", "年度服务合同")
	seedProbeFundIncome(t, engine, "C001", "2026-02", 100, 80, 100)

	rewrite := RewriteBossQuery("2026年3月收入是多少？", probeAnchor())
	probe := firstProbeResult(engine.ProbeBossSources(ctx, rewrite), BossSourceContractAggregate)

	if probe.CanAnswer {
		t.Fatalf("CanAnswer = true, want false")
	}
	if probe.CoverageStatus != CoverageMissing {
		t.Fatalf("CoverageStatus = %s, want %s", probe.CoverageStatus, CoverageMissing)
	}
	if !strings.Contains(probe.MissingReason, "2026-03") {
		t.Fatalf("MissingReason = %q, want requested period", probe.MissingReason)
	}
}

func TestProbeContractProfitNeedsRevenueAndCost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "probe-contract-profit.sqlite")
	seedProbeContract(t, engine, "C001", "测试客户有限公司", "年度服务合同")
	seedProbeFundIncome(t, engine, "C001", "2026-03", 100, 80, 100)

	rewrite := RewriteBossQuery("2026年3月利润多少？", probeAnchor())
	probe := firstProbeResult(engine.ProbeBossSources(ctx, rewrite), BossSourceContractAggregate)

	if probe.CanAnswer {
		t.Fatalf("CanAnswer = true, want false")
	}
	if probe.CoverageStatus != CoverageMissing {
		t.Fatalf("CoverageStatus = %s, want %s", probe.CoverageStatus, CoverageMissing)
	}
	if !strings.Contains(probe.MissingReason, "成本") {
		t.Fatalf("MissingReason = %q, want missing cost coverage", probe.MissingReason)
	}
}

func TestProbeExplicitBankBypassesContractFirst(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := newProbeTestEngine(t, "probe-bank-explicit.sqlite")
	if _, err := engine.db.ExecContext(ctx, `
INSERT INTO bank_statement(company, transaction_date, credit_amount, debit_amount, summary, counterparty_name)
VALUES (?, '2026-03-06', 2131000, 0, '结算款', '辽宁金程信息科技有限公司')
`, engine.Company); err != nil {
		t.Fatalf("seed bank statement: %v", err)
	}

	rewrite := RewriteBossQuery("2026年3月银行卡实际到账多少？", probeAnchor())
	probes := engine.ProbeBossSources(ctx, rewrite)
	if len(probes) == 0 {
		t.Fatalf("expected bank probe")
	}
	first := probes[0]
	if first.Source != BossSourceBankStatement {
		t.Fatalf("first source = %s, want %s", first.Source, BossSourceBankStatement)
	}
	if containsString(first.PrimaryTables, "fin_fund_income") {
		t.Fatalf("explicit bank probe should not use contract primary table: %#v", first.PrimaryTables)
	}
	if !first.CanAnswer {
		t.Fatalf("bank probe CanAnswer=false, reason=%s", first.MissingReason)
	}
}

func newProbeTestEngine(t *testing.T, name string) *Engine {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), name)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

func probeAnchor() time.Time {
	return time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC)
}

func seedProbeContract(t *testing.T, engine *Engine, contractID, customerName, content string) {
	t.Helper()
	if _, err := engine.db.Exec(`
INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
VALUES (?, ?, ?)
`, contractID, customerName, content); err != nil {
		t.Fatalf("seed contract: %v", err)
	}
}

func seedProbeFundIncome(t *testing.T, engine *Engine, contractID, period string, settlement, received, invoice float64) {
	t.Helper()
	if _, err := engine.db.Exec(`
INSERT INTO fin_fund_income(contract_id, year_month, settlement_amount, received_amount, invoice_amount)
VALUES (?, ?, ?, ?, ?)
`, contractID, period, settlement, received, invoice); err != nil {
		t.Fatalf("seed fund income: %v", err)
	}
}

func firstProbeResult(probes []SourceProbeResult, source string) SourceProbeResult {
	for _, probe := range probes {
		if probe.Source == source {
			return probe
		}
	}
	return SourceProbeResult{}
}
