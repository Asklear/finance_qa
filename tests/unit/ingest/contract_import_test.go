package ingest_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/ingest"

	"github.com/xuri/excelize/v2"
	_ "modernc.org/sqlite"
)

func TestImportFileWithOptions_ContractRevenueCostWorkbookLoadsFinTables(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-revenue-cost.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集收入、支出月度计算表-end.xlsx")
	if err := createRevenueCostWorkbook(workbook); err != nil {
		t.Fatalf("create revenue-cost workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	summary, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	})
	if err != nil {
		t.Fatalf("import contract revenue-cost workbook failed: %v", err)
	}
	if summary.RecordCount != 2 {
		t.Fatalf("record_count = %d, want 2", summary.RecordCount)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_contracts`, 2)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 1)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlements`, 1)

	var settlement float64
	if err := db.QueryRow(`SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2025-01'`).Scan(&settlement); err != nil {
		t.Fatalf("sum fund settlement: %v", err)
	}
	if settlement != 1000 {
		t.Fatalf("settlement_amount = %.2f, want 1000", settlement)
	}
}

func TestImportFileWithOptions_ContractFundWorkbookLoadsFundIncome(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-fund.sqlite")
	revenueCost := filepath.Join(t.TempDir(), "优集收入、支出月度计算表-end.xlsx")
	fundWorkbook := filepath.Join(t.TempDir(), "优集资金收入计算表 - 副本.xlsx")
	if err := createRevenueCostWorkbook(revenueCost); err != nil {
		t.Fatalf("create revenue-cost workbook: %v", err)
	}
	if err := createFundWorkbook(fundWorkbook); err != nil {
		t.Fatalf("create fund workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, revenueCost, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("seed contract dimension failed: %v", err)
	}
	summary, err := imp.ImportFileWithOptions(ctx, dbPath, fundWorkbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	})
	if err != nil {
		t.Fatalf("import fund workbook failed: %v", err)
	}
	if summary.RecordCount != 2 {
		t.Fatalf("record_count = %d, want 2", summary.RecordCount)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 3)

	var received float64
	if err := db.QueryRow(`SELECT COALESCE(SUM(received_amount),0) FROM fin_fund_income WHERE year_month='2025-10'`).Scan(&received); err != nil {
		t.Fatalf("sum fund income: %v", err)
	}
	if received != 1234 {
		t.Fatalf("received_amount = %.2f, want 1234", received)
	}

	var januarySettlement float64
	if err := db.QueryRow(`SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2025-01'`).Scan(&januarySettlement); err != nil {
		t.Fatalf("sum january settlement: %v", err)
	}
	if januarySettlement != 1000 {
		t.Fatalf("january settlement_amount = %.2f, want 1000", januarySettlement)
	}

	var comment string
	if err := db.QueryRow(`SELECT comment FROM meta_table_comments WHERE table_name='fin_fund_income'`).Scan(&comment); err != nil {
		t.Fatalf("load fin_fund_income table comment: %v", err)
	}
	if !strings.Contains(comment, "优集资金收入计算表-副本.xlsx") {
		t.Fatalf("fund income table comment should contain workbook name, got %q", comment)
	}
	if !strings.Contains(comment, "25年Q4收入明细") || !strings.Contains(comment, "26年Q1收入明细") {
		t.Fatalf("fund income table comment should contain quarter sheets, got %q", comment)
	}
}

func TestImportFileWithOptions_ContractFundWorkbookFullReplacePreservesRevenueWorkbookRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-fund-full-replace-scoped.sqlite")
	revenueCost := filepath.Join(t.TempDir(), "优集收入、支出月度计算表-end.xlsx")
	fundWorkbook := filepath.Join(t.TempDir(), "优集资金收入计算表 - 副本.xlsx")
	if err := createRevenueCostWorkbook(revenueCost); err != nil {
		t.Fatalf("create revenue-cost workbook: %v", err)
	}
	if err := createFundWorkbook(fundWorkbook); err != nil {
		t.Fatalf("create fund workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, revenueCost, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import revenue-cost workbook failed: %v", err)
	}
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, fundWorkbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import fund workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 3)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2025-01'`, 1000)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(received_amount),0) FROM fin_fund_income WHERE year_month='2025-10'`, 1234)
}

func TestImportFileWithOptions_ContractWorkbookPersistsExtendedContractFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-extended-fields.sqlite")
	revenueCost := filepath.Join(t.TempDir(), "优集收入、支出月度计算表-end.xlsx")
	fundWorkbook := filepath.Join(t.TempDir(), "优集资金收入计算表 - 副本.xlsx")
	if err := createRevenueCostWorkbook(revenueCost); err != nil {
		t.Fatalf("create revenue-cost workbook: %v", err)
	}
	if err := createFundWorkbook(fundWorkbook); err != nil {
		t.Fatalf("create fund workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, revenueCost, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import revenue-cost workbook failed: %v", err)
	}
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, fundWorkbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import fund workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertRowString(t, db, `
SELECT contract_start_date, contract_end_date, settlement_cycle
FROM fin_contracts
WHERE customer_name='辽宁金程信息科技有限公司' AND contract_content='行业商品数据采购合同-A01'
`, "2025-01-01", "2026-03-31", "月结")

	assertRowString(t, db, `
SELECT quantity, contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price
FROM fin_fund_income
WHERE year_month='2025-01'
`, "10", "2025-01-01", "2025-12-31", "月结", "100")

	assertRowStringFloat(t, db, `
SELECT quantity, contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price, invoice_amount, paid_amount
FROM fin_cost_settlements
WHERE year_month='2025-01'
`, "1人月", "2025-01-01", "2025-12-31", "月结", "50", 888, 666)

	assertRowString(t, db, `
SELECT quantity, contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price
FROM fin_fund_income
WHERE year_month='2026-01'
`, "12", "2026-01-01", "2026-03-31", "月结", "100")
}

func TestImportFileWithOptions_ContractRevenueCostWorkbookInfersYearMonthAndCarriesForwardCustomer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-revenue-cross-year.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集收入、支出月度计算表-end.xlsx")
	if err := createCrossYearRevenueCostWorkbook(workbook); err != nil {
		t.Fatalf("create cross-year revenue-cost workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import cross-year contract revenue-cost workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_contracts`, 4)

	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2026-01'`, 1800)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2025-10'`, 500)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2026-10'`, 1000)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2025-01'`, 0)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2025-10'`, 0)
}

func TestImportFileWithOptions_ContractRevenueCostWorkbookUsesSheetMonthDefaultBeforeContractPeriod(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-revenue-cost-sheet-month-default.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集收入成本计算表-表头归期.xlsx")
	if err := createRevenueCostWorkbookWithCrossPeriodFebruaryRows(workbook); err != nil {
		t.Fatalf("create sheet month default workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import sheet month default workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2026-02'`, 60)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2027-02'`, 0)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2026-02'`, 60)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2027-02'`, 0)
}

func TestImportFileWithOptions_ContractRevenueCostWorkbookUsesExplicitYearMonthHeaderBeforeContractPeriod(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-revenue-cost-explicit-year-month.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集收入成本计算表-年份月份表头.xlsx")
	if err := createRevenueCostWorkbookWithExplicitYearMonthHeader(workbook); err != nil {
		t.Fatalf("create explicit year month workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import explicit year month workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2026-02'`, 30)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2027-02'`, 0)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2026-02'`, 30)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2027-02'`, 0)
}

func TestImportFileWithOptions_ContractFundWorkbookSupportsAnyQuarterSheetAndCarryForward(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-fund-q2.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集资金收入计算表-q2.xlsx")
	if err := createAnyQuarterFundWorkbook(workbook); err != nil {
		t.Fatalf("create any-quarter fund workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import any-quarter fund workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_contracts`, 2)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2027-04'`, 400)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2027-05'`, 500)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(received_amount),0) FROM fin_fund_income WHERE year_month='2027-05'`, 500)
}

func TestImportFileWithOptions_ContractCostWorkbookCarriesForwardMergedContractContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-cost-carry-content.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集成本计算表-合并内容.xlsx")
	if err := createCostWorkbookWithBlankContinuedContent(workbook); err != nil {
		t.Fatalf("create cost workbook with blank continued content: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import cost workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlements`, 3)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2026-01'`, 100)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2026-02'`, 200)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2026-03'`, 300)
}

func TestImportFileWithOptions_ContractFundWorkbookCarriesForwardMergedContractContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-fund-carry-content.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集资金收入计算表-合并内容.xlsx")
	if err := createFundWorkbookWithBlankContinuedContent(workbook); err != nil {
		t.Fatalf("create fund workbook with blank continued content: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import fund workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 3)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2026-01'`, 100)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income WHERE year_month='2026-02'`, 200)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(received_amount),0) FROM fin_fund_income WHERE year_month='2026-03'`, 300)
}

func TestImportFileWithOptions_ContractFundWorkbookDoesNotDuplicateMergedMonthlyAmounts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-fund-carry-amounts.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集资金收入计算表-合并金额.xlsx")
	if err := createFundWorkbookWithMergedMonthlyAmounts(workbook); err != nil {
		t.Fatalf("create fund workbook with merged monthly amounts: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import fund workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_contracts`, 4)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 0)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income_groups`, 3)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income_group_members`, 12)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income_groups WHERE year_month='2026-01'`, 100)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income_groups WHERE year_month='2026-02'`, 200)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(received_amount),0) FROM fin_fund_income_groups WHERE year_month='2026-03'`, 300)
	assertYearMonthAmount(t, db, `
SELECT COALESCE(SUM(fi.settlement_amount),0)
FROM fin_fund_income fi
JOIN fin_contracts fc ON fc.contract_id = fi.contract_id
WHERE fc.contract_content = '数据采购合同-快手'
`, 0)
	assertYearMonthAmount(t, db, `
SELECT COALESCE(SUM(fi.settlement_amount),0)
FROM fin_fund_income fi
JOIN fin_contracts fc ON fc.contract_id = fi.contract_id
WHERE fc.contract_content = '数据采购合同-抖音'
`, 0)
	assertCount(t, db, `
SELECT COUNT(DISTINCT fc.contract_content)
FROM fin_fund_income_group_members gm
JOIN fin_contracts fc ON fc.contract_id = gm.contract_id
JOIN fin_fund_income_groups g ON g.id = gm.group_id
WHERE g.year_month = '2026-01'
  AND fc.contract_content LIKE '数据采购合同-%'
`, 4)
}

func TestImportFileWithOptions_ContractCostWorkbookDoesNotDuplicateMergedMonthlyAmounts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-cost-carry-amounts.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集成本计算表-合并金额.xlsx")
	if err := createCostWorkbookWithMergedMonthlyAmounts(workbook); err != nil {
		t.Fatalf("create cost workbook with merged monthly amounts: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import cost workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_contracts`, 4)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlements`, 0)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlement_groups`, 3)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlement_group_members`, 12)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-01'`, 100)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-02'`, 200)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(paid_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-03'`, 300)
	assertYearMonthAmount(t, db, `
SELECT COALESCE(SUM(cs.settlement_amount),0)
FROM fin_cost_settlements cs
JOIN fin_contracts fc ON fc.contract_id = cs.contract_id
WHERE fc.contract_content LIKE '外包服务合同-%'
`, 0)
	assertCount(t, db, `
SELECT COUNT(DISTINCT fc.contract_content)
FROM fin_cost_settlement_group_members gm
JOIN fin_contracts fc ON fc.contract_id = gm.contract_id
JOIN fin_cost_settlement_groups g ON g.id = gm.group_id
WHERE g.year_month = '2026-01'
  AND fc.contract_content LIKE '外包服务合同-%'
`, 4)
}

func TestImportFileWithOptions_ContractCostWorkbookDedupesMergedMembersAndSkipsUnmergedBlankContractPayment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-cost-blank-contract-payment.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集成本计算表-空合同付款.xlsx")
	if err := createCostWorkbookWithMergedContractAndUnmergedBlankPayment(workbook); err != nil {
		t.Fatalf("create cost workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import cost workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_contracts`, 1)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlements`, 0)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlement_groups`, 1)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlement_group_members`, 1)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-01'`, 100)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(paid_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-01'`, 0)
	assertCount(t, db, `
SELECT COUNT(1)
FROM fin_cost_settlements
WHERE paid_amount = 576127.10 OR settlement_amount = 576127.10 OR invoice_amount = 576127.10
`, 0)
}

func TestImportFileWithOptions_ContractCostWorkbookSeparatesDifferentMergedMetricRanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-cost-different-merged-metrics.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集成本计算表-不同金额合并范围.xlsx")
	if err := createCostWorkbookWithDifferentMergedMetricRanges(workbook); err != nil {
		t.Fatalf("create cost workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import cost workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlements`, 0)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlement_groups`, 3)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-01'`, 300)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(invoice_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-01'`, 200)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(paid_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-01'`, 150)
	assertCount(t, db, `
SELECT COUNT(1)
FROM fin_cost_settlement_group_members gm
JOIN fin_cost_settlement_groups g ON g.id = gm.group_id
WHERE g.invoice_amount = 200
`, 2)
	assertCount(t, db, `
SELECT COUNT(1)
FROM fin_cost_settlement_group_members gm
JOIN fin_cost_settlement_groups g ON g.id = gm.group_id
WHERE g.paid_amount = 150
`, 2)
}

func TestImportFileWithOptions_ContractFundWorkbookSeparatesDifferentMergedMetricRanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-fund-different-merged-metrics.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集资金收入计算表-不同金额合并范围.xlsx")
	if err := createFundWorkbookWithDifferentMergedMetricRanges(workbook); err != nil {
		t.Fatalf("create fund workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import fund workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 0)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income_groups`, 3)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income_groups WHERE year_month='2026-01'`, 300)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(invoice_amount),0) FROM fin_fund_income_groups WHERE year_month='2026-01'`, 200)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(received_amount),0) FROM fin_fund_income_groups WHERE year_month='2026-01'`, 150)
	assertCount(t, db, `
SELECT COUNT(1)
FROM fin_fund_income_group_members gm
JOIN fin_fund_income_groups g ON g.id = gm.group_id
WHERE g.invoice_amount = 200
`, 2)
	assertCount(t, db, `
SELECT COUNT(1)
FROM fin_fund_income_group_members gm
JOIN fin_fund_income_groups g ON g.id = gm.group_id
WHERE g.received_amount = 150
`, 2)
}

func TestImportFileWithOptions_ContractFundWorkbookIncrementalCleansOldMergedChildAmounts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-fund-merged-incremental-cleanup.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集资金收入计算表-合并金额.xlsx")
	if err := createFundWorkbookWithMergedMonthlyAmounts(workbook); err != nil {
		t.Fatalf("create fund workbook with merged monthly amounts: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("seed fund workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO fin_fund_income(
	contract_id, year_month, source_report_type, source_sheet_name,
	quantity, settlement_amount, received_amount, is_invoiced, invoice_amount
)
SELECT contract_id, '2026-01', 'contract_fund_income', '26年Q1收入明细',
       '/', 100, 100, '是', 100
FROM fin_contracts
WHERE customer_name = 'Yipit,LLC'
  AND contract_content LIKE '数据采购合同-%'
`); err != nil {
		_ = db.Close()
		t.Fatalf("seed old duplicated child amounts: %v", err)
	}
	_ = db.Close()

	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("incremental reimport fund workbook failed: %v", err)
	}

	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_contracts WHERE contract_content LIKE '合并金额组%'`, 0)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 0)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income_groups`, 3)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income_group_members`, 12)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_fund_income_groups WHERE year_month='2026-01'`, 100)
	assertYearMonthAmount(t, db, `
SELECT COALESCE(SUM(fi.settlement_amount),0)
FROM fin_fund_income fi
JOIN fin_contracts fc ON fc.contract_id = fi.contract_id
WHERE fc.contract_content LIKE '数据采购合同-%'
`, 0)
}

func TestImportFileWithOptions_ContractCostWorkbookIncrementalCleansOldMergedChildAmounts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-cost-merged-incremental-cleanup.sqlite")
	workbook := filepath.Join(t.TempDir(), "优集成本计算表-合并金额.xlsx")
	if err := createCostWorkbookWithMergedMonthlyAmounts(workbook); err != nil {
		t.Fatalf("create cost workbook with merged monthly amounts: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("seed cost workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO fin_cost_settlements(
	contract_id, year_month, source_report_type, source_sheet_name,
	quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code
)
SELECT contract_id, '2026-01', 'contract_revenue_cost', '成本-月度结算',
       '/', 100, '是', 100, 100, '640101'
FROM fin_contracts
WHERE customer_name = '上海合并供应商科技有限公司'
  AND contract_content LIKE '外包服务合同-%'
`); err != nil {
		_ = db.Close()
		t.Fatalf("seed old duplicated child cost amounts: %v", err)
	}
	_ = db.Close()

	if _, err := imp.ImportFileWithOptions(ctx, dbPath, workbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("incremental reimport cost workbook failed: %v", err)
	}

	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlements`, 0)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlement_groups`, 3)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlement_group_members`, 12)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlement_groups WHERE year_month='2026-01'`, 100)
	assertYearMonthAmount(t, db, `
SELECT COALESCE(SUM(cs.settlement_amount),0)
FROM fin_cost_settlements cs
JOIN fin_contracts fc ON fc.contract_id = cs.contract_id
WHERE fc.contract_content LIKE '外包服务合同-%'
`, 0)
}

func TestImportFileWithOptions_CostOnlyWorkbookDoesNotClearFundIncomeOnFullReplace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-cost-only-full-replace.sqlite")
	fundWorkbook := filepath.Join(t.TempDir(), "优集资金收入计算表 - 副本.xlsx")
	costOnlyWorkbook := filepath.Join(t.TempDir(), "优集成本计算表-仅成本.xlsx")
	if err := createFundWorkbook(fundWorkbook); err != nil {
		t.Fatalf("create fund workbook: %v", err)
	}
	if err := createCostOnlyWorkbook(costOnlyWorkbook); err != nil {
		t.Fatalf("create cost-only workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, fundWorkbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("seed fund workbook failed: %v", err)
	}
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, costOnlyWorkbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import cost-only workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 2)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlements`, 1)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(received_amount),0) FROM fin_fund_income WHERE year_month='2026-01'`, 1000)
}

func TestImportFileWithOptions_ContractFundWorkbookMergesTableSourceMetadataAcrossFiles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-fund-source-metadata.sqlite")
	firstWorkbook := filepath.Join(t.TempDir(), "优集资金收入计算表-2025Q4.xlsx")
	secondWorkbook := filepath.Join(t.TempDir(), "优集资金收入计算表-2026.xlsx")
	if err := createFundWorkbook(firstWorkbook); err != nil {
		t.Fatalf("create first fund workbook: %v", err)
	}
	if err := createAnyQuarterFundWorkbook(secondWorkbook); err != nil {
		t.Fatalf("create second fund workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, firstWorkbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import first fund workbook failed: %v", err)
	}
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, secondWorkbook, ingest.ImportOptions{
		Incremental:     true,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import second fund workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var comment string
	if err := db.QueryRow(`SELECT comment FROM meta_table_comments WHERE table_name='fin_fund_income'`).Scan(&comment); err != nil {
		t.Fatalf("load fin_fund_income table comment: %v", err)
	}
	if !strings.Contains(comment, "优集资金收入计算表-2025Q4.xlsx") || !strings.Contains(comment, "优集资金收入计算表-2026.xlsx") {
		t.Fatalf("fund income source metadata should keep both workbook names, got %q", comment)
	}
	if !strings.Contains(comment, "26年Q1收入明细") || !strings.Contains(comment, "27年Q2收入明细") {
		t.Fatalf("fund income source metadata should keep sheets from both imports, got %q", comment)
	}
}

func TestImportFileWithOptions_RevenueOnlyWorkbookDoesNotClearCostSettlementsOnFullReplace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contract-revenue-only-full-replace.sqlite")
	costOnlyWorkbook := filepath.Join(t.TempDir(), "优集成本计算表-仅成本.xlsx")
	revenueOnlyWorkbook := filepath.Join(t.TempDir(), "优集收入计算表-仅收入.xlsx")
	if err := createCostOnlyWorkbook(costOnlyWorkbook); err != nil {
		t.Fatalf("create cost-only workbook: %v", err)
	}
	if err := createRevenueOnlyWorkbook(revenueOnlyWorkbook); err != nil {
		t.Fatalf("create revenue-only workbook: %v", err)
	}

	imp := ingest.NewImporter(nil)
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, costOnlyWorkbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("seed cost workbook failed: %v", err)
	}
	if _, err := imp.ImportFileWithOptions(ctx, dbPath, revenueOnlyWorkbook, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import revenue-only workbook failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertCount(t, db, `SELECT COUNT(1) FROM fin_fund_income`, 1)
	assertCount(t, db, `SELECT COUNT(1) FROM fin_cost_settlements`, 1)
	assertYearMonthAmount(t, db, `SELECT COALESCE(SUM(settlement_amount),0) FROM fin_cost_settlements WHERE year_month='2025-01'`, 888)
}

func createRevenueCostWorkbook(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "收入-月度结算")
	if err := writeSheetRows(f, "收入-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "1月", "", "", ""},
		{"", "", "", "", "", "", "", "", "", ""},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2025-01-01", "2025-12-31", "月结", 100, 10, 1000, "是", 1000},
	}); err != nil {
		return err
	}
	if _, err := f.NewSheet("成本-月度结算"); err != nil {
		return err
	}
	if err := writeSheetRows(f, "成本-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "1月", "", "", "", ""},
		{"", "", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "付款金额"},
		{"南京林悦智能科技有限公司", "技术服务采购合同-LY01", "2025-01-01", "2025-12-31", "月结", 50, "640101", "1人月", 888, "是", 888, 666},
	}); err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createFundWorkbook(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "26年Q1收入明细")
	if err := writeSheetRows(f, "26年Q1收入明细", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "1月", "", "", "", ""},
		{"", "", "", "", "", "", "", "", "", "", ""},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2026-01-01", "2026-03-31", "月结", 100, 12, 1200, "是", 1200, 1000},
	}); err != nil {
		return err
	}
	if _, err := f.NewSheet("25年Q4收入明细"); err != nil {
		return err
	}
	if err := writeSheetRows(f, "25年Q4收入明细", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "10月", "11月", "12月"},
		{"", "", "", "", "", "", "收入金额", "收入金额", "收入金额"},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2025-10-01", "2025-12-31", "月结", 100, 1234, "", ""},
	}); err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createCrossYearRevenueCostWorkbook(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "收入-月度结算")
	if err := writeSheetRows(f, "收入-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "1月", "", "", "", "", "10月", "", "", ""},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "", "数量", "结算金额", "是否开票", "开票金额"},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2025-08-26", "2026-08-25", "月结", 100, 10, 1000, "是", 1000, "", 5, 500, "是", 500},
		{"", "行业商品数据采购合同-A02", "", "", "", 50, 8, 800, "否", 0, "", "", "", "", ""},
	}); err != nil {
		return err
	}
	if err := f.MergeCell("收入-月度结算", "A3", "A4"); err != nil {
		return err
	}
	if _, err := f.NewSheet("成本-月度结算"); err != nil {
		return err
	}
	if err := writeSheetRows(f, "成本-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "10月", "", ""},
		{"", "", "", "", "", "", "", "数量", "结算金额", "是否开票"},
		{"南京林悦智能科技有限公司", "技术服务采购合同-LY01", "2026-02-01", "2026-12-31", "月结", 50, "640101", "1人月", 600, "是"},
		{"", "技术服务采购合同-LY02", "", "", "", 50, "640101", "1人月", 400, "否"},
	}); err != nil {
		return err
	}
	if err := f.MergeCell("成本-月度结算", "A3", "A4"); err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createRevenueCostWorkbookWithCrossPeriodFebruaryRows(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "收入-月度结算")
	if err := writeSheetRows(f, "收入-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "2月", "", "", ""},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额"},
		{"表头归期客户A", "收入合同-2026-A", "2026-01-01", "2026-12-31", "月结", 1, 1, 10, "是", 10},
		{"表头归期客户B", "收入合同-2026-B", "2026-01-01", "2026-12-31", "月结", 1, 1, 20, "是", 20},
		{"表头归期客户C", "收入合同-跨期-C", "2026-03-01", "2027-02-28", "月结", 1, 1, 30, "是", 30},
	}); err != nil {
		return err
	}
	if _, err := f.NewSheet("成本-月度结算"); err != nil {
		return err
	}
	if err := writeSheetRows(f, "成本-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "2月", "", "", "", ""},
		{"", "", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "付款金额"},
		{"表头归期供应商A", "成本合同-2026-A", "2026-01-01", "2026-12-31", "月结", 1, "640101", 1, 10, "是", 10, 10},
		{"表头归期供应商B", "成本合同-2026-B", "2026-01-01", "2026-12-31", "月结", 1, "640101", 1, 20, "是", 20, 20},
		{"表头归期供应商C", "成本合同-跨期-C", "2026-03-01", "2027-02-28", "月结", 1, "640101", 1, 30, "是", 30, 30},
	}); err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createRevenueCostWorkbookWithExplicitYearMonthHeader(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "收入-月度结算")
	if err := writeSheetRows(f, "收入-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "2026年2月", "", "", ""},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额"},
		{"表头归期客户C", "收入合同-跨期-C", "2026-03-01", "2027-02-28", "月结", 1, 1, 30, "是", 30},
	}); err != nil {
		return err
	}
	if _, err := f.NewSheet("成本-月度结算"); err != nil {
		return err
	}
	if err := writeSheetRows(f, "成本-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "2026年2月", "", "", "", ""},
		{"", "", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "付款金额"},
		{"表头归期供应商C", "成本合同-跨期-C", "2026-03-01", "2027-02-28", "月结", 1, "640101", 1, 30, "是", 30, 30},
	}); err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createAnyQuarterFundWorkbook(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "27年Q2收入明细")
	if err := writeSheetRows(f, "27年Q2收入明细", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "4月", "", "", "", "", "5月", "", "", "", ""},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额"},
		{"飞未云科（深圳）技术有限公司", "全品类商品价格数据-京东", "2027-04-01", "2027-12-31", "月结", 100, 4, 400, "是", 400, 380, "", "", "", "", ""},
		{"", "全品类商品销量数据-京东", "", "", "", 80, "", "", "", "", "", 5, 500, "是", 500, 500},
	}); err != nil {
		return err
	}
	if err := f.MergeCell("27年Q2收入明细", "A3", "A4"); err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createCostOnlyWorkbook(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "成本-月度结算")
	if err := writeSheetRows(f, "成本-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "1月", "", ""},
		{"", "", "", "", "", "", "", "", "", ""},
		{"南京林悦智能科技有限公司", "技术服务采购合同-LY01", "2025-01-01", "2025-12-31", "月结", 50, "640101", "1人月", 888, "是"},
	}); err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createRevenueOnlyWorkbook(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "收入-月度结算")
	if err := writeSheetRows(f, "收入-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "1月", "", "", ""},
		{"", "", "", "", "", "", "", "", "", ""},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2025-01-01", "2025-12-31", "月结", 100, 10, 1000, "是", 1000},
	}); err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createCostWorkbookWithBlankContinuedContent(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "成本-月度结算")
	if err := writeSheetRows(f, "成本-月度结算", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "1月", "", "", "", "", "2月", "", "", "", "", "3月"},
		{"", "", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "付款金额", "数量", "结算金额", "是否开票", "开票金额", "付款金额", "数量", "结算金额", "是否开票", "开票金额", "付款金额"},
		{"南京众信数通智能科技有限公司", "电商市场调研服务", "2026/1/1", "2026/12/31", "月度", "固定金额", "640101", "/", 100, "是", 100, 0, "", "", "", "", "", "", "", "", "", ""},
		{"", "", "2026/1/1", "2026/12/31", "月度", "固定金额", "", "", "", "", "", "", "/", 200, "是", 200, 0, "", "", "", "", ""},
		{"", "", "2026/1/1", "2026/12/31", "月度", "固定金额", "", "", "", "", "", "", "", "", "", "", "", "/", 300, "否", 0, 0},
	}); err != nil {
		return err
	}
	for _, mergedRange := range [][2]string{{"A3", "A5"}, {"B3", "B5"}} {
		if err := f.MergeCell("成本-月度结算", mergedRange[0], mergedRange[1]); err != nil {
			return err
		}
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createFundWorkbookWithBlankContinuedContent(path string) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "26年Q1收入明细")
	if err := writeSheetRows(f, "26年Q1收入明细", [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "1月", "", "", "", "", "2月", "", "", "", "", "3月"},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额"},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2026/1/1", "2026/3/31", "月结", 100, 1, 100, "是", 100, 100, "", "", "", "", "", "", "", "", "", ""},
		{"", "", "2026/1/1", "2026/3/31", "月结", 100, "", "", "", "", "", 2, 200, "是", 200, 0, "", "", "", "", ""},
		{"", "", "2026/1/1", "2026/3/31", "月结", 100, "", "", "", "", "", "", "", "", "", "", 3, 300, "否", 0, 300},
	}); err != nil {
		return err
	}
	for _, mergedRange := range [][2]string{{"A3", "A5"}, {"B3", "B5"}} {
		if err := f.MergeCell("26年Q1收入明细", mergedRange[0], mergedRange[1]); err != nil {
			return err
		}
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createCostWorkbookWithMergedContractAndUnmergedBlankPayment(path string) error {
	f := excelize.NewFile()
	sheet := "成本-月度结算"
	f.SetSheetName("Sheet1", sheet)
	if err := writeSheetRows(f, sheet, [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "1月", "", "", "", ""},
		{"", "", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "付款金额"},
		{"南京林悦智能科技有限公司", "行业商品数据采购合同", "2026/1/1", "2026/12/31", "月度", "固定金额", "640101", "/", 100, "是", 100, 0},
		{"", "", "2026/1/1", "2026/12/31", "月度", "固定金额", "640101", "/", "", "是", "", ""},
		{"", "", "2026/1/1", "2026/12/31", "月度", "固定金额", "640101", "", "", "", "", 576127.10},
	}); err != nil {
		return err
	}
	for _, mergedRange := range [][2]string{{"A3", "A5"}, {"B3", "B4"}, {"K3", "K4"}} {
		if err := f.MergeCell(sheet, mergedRange[0], mergedRange[1]); err != nil {
			return err
		}
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createCostWorkbookWithDifferentMergedMetricRanges(path string) error {
	f := excelize.NewFile()
	sheet := "成本-月度结算"
	f.SetSheetName("Sheet1", sheet)
	if err := writeSheetRows(f, sheet, [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "1月", "", "", "", ""},
		{"", "", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "付款金额"},
		{"上海合并供应商科技有限公司", "外包服务合同-A", "2026/1/1", "2026/12/31", "月度", "固定金额", "640101", "/", 300, "是", 200, ""},
		{"上海合并供应商科技有限公司", "外包服务合同-B", "2026/1/1", "2026/12/31", "月度", "固定金额", "640101", "/", "", "是", "", 150},
		{"上海合并供应商科技有限公司", "外包服务合同-C", "2026/1/1", "2026/12/31", "月度", "固定金额", "640101", "/", "", "是", "", ""},
	}); err != nil {
		return err
	}
	for _, mergedRange := range [][2]string{{"I3", "I5"}, {"K3", "K4"}, {"L4", "L5"}} {
		if err := f.MergeCell(sheet, mergedRange[0], mergedRange[1]); err != nil {
			return err
		}
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createFundWorkbookWithDifferentMergedMetricRanges(path string) error {
	f := excelize.NewFile()
	sheet := "26年Q1收入明细"
	f.SetSheetName("Sheet1", sheet)
	if err := writeSheetRows(f, sheet, [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "1月", "", "", "", ""},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额"},
		{"Yipit,LLC", "数据采购合同-A", "2026/1/1", "2026/3/31", "月结", "固定金额", "/", 300, "是", 200, ""},
		{"Yipit,LLC", "数据采购合同-B", "2026/1/1", "2026/3/31", "月结", "固定金额", "/", "", "是", "", 150},
		{"Yipit,LLC", "数据采购合同-C", "2026/1/1", "2026/3/31", "月结", "固定金额", "/", "", "是", "", ""},
	}); err != nil {
		return err
	}
	for _, mergedRange := range [][2]string{{"H3", "H5"}, {"J3", "J4"}, {"K4", "K5"}} {
		if err := f.MergeCell(sheet, mergedRange[0], mergedRange[1]); err != nil {
			return err
		}
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createFundWorkbookWithMergedMonthlyAmounts(path string) error {
	f := excelize.NewFile()
	sheet := "26年Q1收入明细"
	f.SetSheetName("Sheet1", sheet)
	if err := writeSheetRows(f, sheet, [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "1月", "", "", "", "", "2月", "", "", "", "", "3月", "", "", "", ""},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额"},
		{"Yipit,LLC", "数据采购合同-快手", "2026/1/1", "2026/3/31", "月结", "固定金额", "/", 100, "是", 100, 100, "/", 200, "是", 200, 200, "/", 300, "是", 300, 300},
		{"Yipit,LLC", "数据采购合同-抖音", "2026/1/1", "2026/3/31", "月结", "固定金额", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Yipit,LLC", "数据采购合同-shopee", "2026/1/1", "2026/3/31", "月结", "固定金额", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Yipit,LLC", "数据采购合同-Tmal", "2026/1/1", "2026/3/31", "月结", "固定金额", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
	}); err != nil {
		return err
	}
	for _, mergedRange := range [][2]string{{"G1", "K1"}, {"L1", "P1"}, {"Q1", "U1"}} {
		if err := f.MergeCell(sheet, mergedRange[0], mergedRange[1]); err != nil {
			return err
		}
	}
	for _, col := range []string{"G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U"} {
		if err := f.MergeCell(sheet, col+"3", col+"6"); err != nil {
			return err
		}
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func createCostWorkbookWithMergedMonthlyAmounts(path string) error {
	f := excelize.NewFile()
	sheet := "成本-月度结算"
	f.SetSheetName("Sheet1", sheet)
	if err := writeSheetRows(f, sheet, [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价", "入账科目", "1月", "", "", "", "", "2月", "", "", "", "", "3月", "", "", "", ""},
		{"", "", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "付款金额", "数量", "结算金额", "是否开票", "开票金额", "付款金额", "数量", "结算金额", "是否开票", "开票金额", "付款金额"},
		{"上海合并供应商科技有限公司", "外包服务合同-A", "2026/1/1", "2026/3/31", "月结", "固定金额", "640101", "/", 100, "是", 100, 100, "/", 200, "是", 200, 200, "/", 300, "是", 300, 300},
		{"上海合并供应商科技有限公司", "外包服务合同-B", "2026/1/1", "2026/3/31", "月结", "固定金额", "640101", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"上海合并供应商科技有限公司", "外包服务合同-C", "2026/1/1", "2026/3/31", "月结", "固定金额", "640101", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"上海合并供应商科技有限公司", "外包服务合同-D", "2026/1/1", "2026/3/31", "月结", "固定金额", "640101", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
	}); err != nil {
		return err
	}
	for _, mergedRange := range [][2]string{{"H1", "L1"}, {"M1", "Q1"}, {"R1", "V1"}} {
		if err := f.MergeCell(sheet, mergedRange[0], mergedRange[1]); err != nil {
			return err
		}
	}
	for _, col := range []string{"H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V"} {
		if err := f.MergeCell(sheet, col+"3", col+"6"); err != nil {
			return err
		}
	}
	defer func() { _ = f.Close() }()
	return f.SaveAs(path)
}

func writeSheetRows(f *excelize.File, sheet string, rows [][]any) error {
	for rIdx, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, rIdx+1)
		if err != nil {
			return err
		}
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			return err
		}
	}
	return nil
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query count failed: %v", err)
	}
	if got != want {
		t.Fatalf("count for %q = %d, want %d", query, got, want)
	}
}

func assertYearMonthAmount(t *testing.T, db *sql.DB, query string, want float64) {
	t.Helper()
	var got float64
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query amount failed: %v", err)
	}
	if got != want {
		t.Fatalf("amount for %q = %.2f, want %.2f", query, got, want)
	}
}

func assertRowString(t *testing.T, db *sql.DB, query string, want ...string) {
	t.Helper()
	dest := make([]sql.NullString, len(want))
	scanArgs := make([]any, len(want))
	for i := range dest {
		scanArgs[i] = &dest[i]
	}
	if err := db.QueryRow(query).Scan(scanArgs...); err != nil {
		t.Fatalf("query row string failed: %v", err)
	}
	for i, expected := range want {
		got := ""
		if dest[i].Valid {
			got = dest[i].String
		}
		if got != expected {
			t.Fatalf("column %d for %q = %q, want %q", i, query, got, expected)
		}
	}
}

func assertRowStringFloat(t *testing.T, db *sql.DB, query string, s1, s2, s3, s4, s5 string, f1, f2 float64) {
	t.Helper()
	var c1, c2, c3, c4, c5 sql.NullString
	var n1, n2 sql.NullFloat64
	if err := db.QueryRow(query).Scan(&c1, &c2, &c3, &c4, &c5, &n1, &n2); err != nil {
		t.Fatalf("query row string/float failed: %v", err)
	}
	gotStrings := []string{nullStringValue(c1), nullStringValue(c2), nullStringValue(c3), nullStringValue(c4), nullStringValue(c5)}
	wantStrings := []string{s1, s2, s3, s4, s5}
	for i := range wantStrings {
		if gotStrings[i] != wantStrings[i] {
			t.Fatalf("string column %d for %q = %q, want %q", i, query, gotStrings[i], wantStrings[i])
		}
	}
	if !n1.Valid || n1.Float64 != f1 {
		t.Fatalf("float column 1 for %q = %v, want %.2f", query, n1.Float64, f1)
	}
	if !n2.Valid || n2.Float64 != f2 {
		t.Fatalf("float column 2 for %q = %v, want %.2f", query, n2.Float64, f2)
	}
}

func nullStringValue(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
