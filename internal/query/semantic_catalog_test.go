package query

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	dbpkg "financeqa/internal/db"

	_ "modernc.org/sqlite"
)

func TestSemanticCatalogMapsFundIncomeToRevenueReceiptsInvoice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "semantic-catalog-fund.sqlite")
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	if err := dbpkg.UpsertTableSourceMetadata(ctx, rawDB, dbPath, "fin_fund_income", dbpkg.TableSourceMetadata{
		Version:      "v1",
		Display:      "《优集资金收入计算表 - 副本.xlsx》的【26年Q1收入明细】",
		LogicalLabel: "fin_fund_income",
		FileNames:    []string{"优集资金收入计算表 - 副本.xlsx"},
		SheetNames:   []string{"26年Q1收入明细"},
		ReportTypes:  []string{"contract_fund_income"},
		Description:  "合同资金收入与回款明细，记录合同维度的账面结算、实际到账与开票情况。",
	}); err != nil {
		t.Fatalf("seed table metadata: %v", err)
	}
	if err := dbpkg.UpsertColumnComment(ctx, rawDB, dbPath, "fin_fund_income", "settlement_amount", "老板口径账面结算收入金额"); err != nil {
		t.Fatalf("seed column comment: %v", err)
	}

	catalog := engine.BuildSemanticCatalog(ctx, []string{"fin_fund_income"})
	profile := catalog.Profiles["fin_fund_income"]

	assertCatalogMetric(t, profile, BossMetricRevenue)
	assertCatalogMetric(t, profile, BossMetricReceipts)
	assertCatalogMetric(t, profile, BossMetricInvoice)
	assertCatalogCapability(t, profile, "contract_dimension")
	assertCatalogCapability(t, profile, "customer_dimension")
	if profile.Deprecated {
		t.Fatalf("fin_fund_income should be eligible")
	}
	if got := profile.Fields["settlement_amount"]; got != "老板口径账面结算收入金额" {
		t.Fatalf("settlement_amount meaning = %q", got)
	}
	if !containsString(profile.SourceDocuments, "《优集资金收入计算表 - 副本.xlsx》的【26年Q1收入明细】") {
		t.Fatalf("source documents = %#v", profile.SourceDocuments)
	}
}

func TestSemanticCatalogMarksRevenueSettlementsDeprecated(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "semantic-catalog-deprecated.sqlite")
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	catalog := engine.BuildSemanticCatalog(ctx, []string{"fin_revenue_settlements"})
	profile := catalog.Profiles["fin_revenue_settlements"]

	if !profile.Deprecated {
		t.Fatalf("fin_revenue_settlements should be deprecated")
	}
	if profile.Eligible {
		t.Fatalf("deprecated source should not be eligible")
	}
	if !strings.Contains(strings.Join(profile.Notes, "；"), "DEPRECATED") {
		t.Fatalf("deprecated note missing: %#v", profile.Notes)
	}
}

func TestSemanticCatalogUsesColumnCommentsForSettlementMeaning(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "semantic-catalog-fields.sqlite")
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	catalog := engine.BuildSemanticCatalog(ctx, []string{"fin_fund_income", "fin_cost_settlements"})

	fund := catalog.Profiles["fin_fund_income"]
	cost := catalog.Profiles["fin_cost_settlements"]
	if got := fund.Fields["settlement_amount"]; got != "账面结算收入金额" {
		t.Fatalf("fund settlement_amount meaning = %q", got)
	}
	if got := cost.Fields["settlement_amount"]; got != "成本结算金额" {
		t.Fatalf("cost settlement_amount meaning = %q", got)
	}
	assertCatalogMetric(t, fund, BossMetricRevenue)
	assertCatalogMetric(t, cost, BossMetricCost)
}

func assertCatalogMetric(t *testing.T, profile SourceCapabilityProfile, metric BossMetric) {
	t.Helper()
	for _, got := range profile.Metrics {
		if got == metric {
			return
		}
	}
	t.Fatalf("%s metrics = %#v, want %s", profile.Table, profile.Metrics, metric)
}

func assertCatalogCapability(t *testing.T, profile SourceCapabilityProfile, capability string) {
	t.Helper()
	for _, got := range profile.Capabilities {
		if got == capability {
			return
		}
	}
	t.Fatalf("%s capabilities = %#v, want %s", profile.Table, profile.Capabilities, capability)
}
