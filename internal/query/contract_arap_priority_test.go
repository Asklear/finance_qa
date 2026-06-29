package query

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestGenericReceivableQuestionUsesContractAggregateBeforeBalanceSheet(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月应收账款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(600) {
		t.Fatalf("total = %v, want contract receivable 600", got)
	}
	if strings.Contains(res.Message, "科目余额表") {
		t.Fatalf("generic receivable should not answer from balance sheet only, got message=%q", res.Message)
	}
	if !strings.Contains(res.Message, "合同") || !strings.Contains(res.Message, "应收") {
		t.Fatalf("message should disclose contract receivable口径, got %q", res.Message)
	}
}

func TestExplicitBalanceSheetReceivableQuestionKeepsOfficialARAP(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月科目余额中的应收账款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source"]; got != "balance_sheet" {
		t.Fatalf("source = %v, want balance_sheet; data=%+v", got, res.Data)
	}
	if got := res.Data["total"]; got != float64(9999) {
		t.Fatalf("total = %v, want official balance 9999", got)
	}
	if !strings.Contains(res.Message, "科目余额表") {
		t.Fatalf("message should disclose balance sheet source, got %q", res.Message)
	}
}

func TestHangingReceivablePayableQuestionUsesProjectAggregateBeforeOfficialBalances(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月应收和应付分别还挂着多少?哪头更重?")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; data=%+v message=%s", got, res.Data, res.Message)
	}
	metrics, ok := res.Data["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics missing: %+v", res.Data)
	}
	if got := metrics["应收"]; got != float64(600) {
		t.Fatalf("metrics[应收] = %v, want project receivable 600", got)
	}
	if got := metrics["应付"]; got != float64(500) {
		t.Fatalf("metrics[应付] = %v, want project payable 500", got)
	}
	for _, want := range []string{"项目应收", "项目应付"} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("message = %q, want include %q", res.Message, want)
		}
	}
	if strings.Contains(res.Message, "账上挂账") || strings.Contains(res.Message, "科目余额") {
		t.Fatalf("generic hanging AR/AP question should not prefer official balance when project data exists, got %q", res.Message)
	}
}

func TestProjectReceivableRangeToLastCompleteNaturalMonthKeepsRequestedPeriod(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 25, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("项目口径看，从2025年10月起到上一个完整自然月月底，还有多少应收未收？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(600) {
		t.Fatalf("total = %v, want project receivable 600", got)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["period_from"]; got != "2025-10" {
		t.Fatalf("query_spec.period_from = %v, want 2025-10", got)
	}
	if got := spec["period_to"]; got != "2026-05" {
		t.Fatalf("query_spec.period_to = %v, want 2026-05", got)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	if got := summary["period_from"]; got != "2025-10" {
		t.Fatalf("contract_summary.period_from = %v, want 2025-10", got)
	}
	if got := summary["period_to"]; got != "2026-05" {
		t.Fatalf("contract_summary.period_to = %v, want 2026-05", got)
	}
	if !strings.Contains(res.Message, "2025-10~2026-05") {
		t.Fatalf("message should include requested range, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "应收未收") {
		t.Fatalf("message should preserve requested receivable-unpaid label, got %q", res.Message)
	}
	finalAnswer, _ := res.Data["final_answer"].(string)
	if !strings.Contains(finalAnswer, "应收未收") {
		t.Fatalf("final_answer should preserve requested receivable-unpaid label, got %q", finalAnswer)
	}
}

func TestHangingReceivablePayableWithoutProjectDataFallsBackToOfficialBalances(t *testing.T) {
	dbPath := buildOfficialOnlyARAPDB(t)
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.April, 14, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("应收和应付分别还挂着多少?哪头更重?")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["period"]; got != "2026-03" {
		t.Fatalf("period = %v, want latest available balance period 2026-03; data=%+v", got, res.Data)
	}
	if got := res.Data["source"]; got != "balance_sheet" {
		t.Fatalf("source = %v, want balance_sheet; data=%+v message=%s", got, res.Data, res.Message)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["period_to"]; got != "2026-03" {
		t.Fatalf("query_spec.period_to = %v, want 2026-03", got)
	}
	if got := res.Data["payable_side_total"]; got != float64(10665) {
		t.Fatalf("payable_side_total = %v, want 10665", got)
	}
}

func TestGenericPayableQuestionUsesContractAggregateBeforeBalanceSheet(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月应付账款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(500) {
		t.Fatalf("total = %v, want contract payable 500", got)
	}
	if !strings.Contains(res.Message, "合同") || !strings.Contains(res.Message, "应付") {
		t.Fatalf("message should disclose contract payable口径, got %q", res.Message)
	}
}

func TestInvoicedUnpaidQuestionUsesInvoiceGapWithoutSyntheticEntity(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年已开票未付款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(400) {
		t.Fatalf("total = %v, want invoice gap 400", got)
	}
	if entity := res.Data["entity"]; entity != nil && entity != "" {
		t.Fatalf("synthetic entity should be empty, got %v", entity)
	}
	if !strings.Contains(res.Message, "已开票未回款") {
		t.Fatalf("message should explain customer-side invoice gap, got %q", res.Message)
	}
}

func TestProjectInvoiceOpenRosterQuestionDoesNotExtractSyntheticEntity(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("有哪些项目已开票未回款")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(400) {
		t.Fatalf("total = %v, want invoice open amount 400", got)
	}
	if entity := res.Data["entity"]; entity != nil && entity != "" {
		t.Fatalf("synthetic entity should be empty, got %v", entity)
	}
	if strings.Contains(res.Message, "没有识别到合同/项目主体") || strings.Contains(res.Message, "合同口径当前不能直接回答") {
		t.Fatalf("project roster question should answer company-scope invoice open amount, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "已开票未回款") {
		t.Fatalf("message should explain invoice open amount, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "测试客户") || !strings.Contains(res.Message, "测试客户项目") {
		t.Fatalf("message should list customer and project content, got %q", res.Message)
	}
	if strings.Contains(res.Message, "C-001") {
		t.Fatalf("message should not expose internal contract id, got %q", res.Message)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	items, ok := summary["invoice_open_items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("invoice_open_items = %#v, want one item", summary["invoice_open_items"])
	}
	if got := items[0]["customer_name"]; got != "测试客户" {
		t.Fatalf("invoice_open_items[0].customer_name = %v", got)
	}
	if got := items[0]["contract_content"]; got != "测试客户项目" {
		t.Fatalf("invoice_open_items[0].contract_content = %v", got)
	}
	if got := items[0]["open_amount"]; got != float64(400) {
		t.Fatalf("invoice_open_items[0].open_amount = %v, want 400", got)
	}
}

func TestProjectInvoiceOpenNaturalPhraseUsesInvoiceGapAsPrimaryMetric(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月已开票但还没回款的项目金额是多少？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["metric"]; got != "已开票未回款" {
		t.Fatalf("metric = %v, want 已开票未回款", got)
	}
	if got := res.Data["total"]; got != float64(400) {
		t.Fatalf("total = %v, want invoice open amount 400", got)
	}
	requested, ok := res.Data["requested_metrics"].([]string)
	if !ok || len(requested) != 1 || requested[0] != "已开票未回款" {
		t.Fatalf("requested_metrics = %#v, want [已开票未回款]", res.Data["requested_metrics"])
	}
	for _, want := range []string{"项目口径", "已开票未回款 400.00", "测试客户"} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("message should include %q for project invoice-open detail, got %q", want, res.Message)
		}
	}
}

func TestReceivedInvoiceUnpaidQuestionUsesSupplierInvoiceGap(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月已收票未付款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(300) {
		t.Fatalf("total = %v, want supplier invoice unpaid 300", got)
	}
	if !strings.Contains(res.Message, "已收票未付款") {
		t.Fatalf("message should explain supplier-side invoice gap, got %q", res.Message)
	}
}

func TestProjectCostUnpaidQuestionUsesProjectPayableBeforeInvoiceGap(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("按项目成本口径，2026年3月未付款合计多少？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(500) {
		t.Fatalf("total = %v, want project payable 500", got)
	}
	for _, want := range []string{"项目应付（应付未付/未付款）", "项目成本", "已付款"} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("message should include %q for project payable context, got %q", want, res.Message)
		}
	}
	if got := res.Data["metric_label"]; got != "项目应付（应付未付/未付款）" {
		t.Fatalf("metric_label = %v, want 项目应付（应付未付/未付款）; data=%+v", got, res.Data)
	}
	if got, _ := res.Data["business_basis"].(string); !strings.Contains(got, "项目成本") || !strings.Contains(got, "应付未付") {
		t.Fatalf("business_basis = %q, want project cost payable basis; data=%+v", got, res.Data)
	}
	if strings.Contains(res.Message, "已收票未付款 300.00") {
		t.Fatalf("generic project-cost unpaid should not select invoice gap as primary answer, got %q", res.Message)
	}
}

func TestProjectPayableRangeToLastCompleteNaturalMonthKeepsRequestedPeriod(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 28, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("老板，帮我看下从项目口径，2025年10月到上个完整自然月月底，我们还有多少应付未付？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(500) {
		t.Fatalf("total = %v, want project payable 500", got)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["period_from"]; got != "2025-10" {
		t.Fatalf("query_spec.period_from = %v, want 2025-10", got)
	}
	if got := spec["period_to"]; got != "2026-05" {
		t.Fatalf("query_spec.period_to = %v, want 2026-05", got)
	}
	if !strings.Contains(res.Message, "2025-10~2026-05") || !strings.Contains(res.Message, "项目应付（应付未付/未付款） 500.00") {
		t.Fatalf("message should keep requested range and project payable metric, got %q", res.Message)
	}
	if strings.Contains(res.Message, "已收票未付款 300.00") {
		t.Fatalf("range project payable aggregate should not select invoice gap as primary answer, got %q", res.Message)
	}
}

func TestUnpaidProjectRosterQuestionReturnsInvoiceOpenItems(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月未付款的项目及对应金额有哪些？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(300) {
		t.Fatalf("total = %v, want supplier invoice unpaid 300", got)
	}
	if !strings.Contains(res.Message, "测试供应商") || !strings.Contains(res.Message, "测试供应商项目") || !strings.Contains(res.Message, "未付款 300.00") {
		t.Fatalf("message should list unpaid supplier-side project items, got %q", res.Message)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	items, ok := summary["invoice_unpaid_items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("invoice_unpaid_items = %#v, want one invoice unpaid item", summary["invoice_unpaid_items"])
	}
	if got := items[0]["supplier_name"]; got != "测试供应商" {
		t.Fatalf("invoice_unpaid_items[0].supplier_name = %v", got)
	}
	if got := items[0]["contract_content"]; got != "测试供应商项目" {
		t.Fatalf("invoice_unpaid_items[0].contract_content = %v", got)
	}
	if got := items[0]["open_amount"]; got != float64(300) {
		t.Fatalf("invoice_unpaid_items[0].open_amount = %v, want 300", got)
	}
}

func TestUnpaidProjectRosterQuestionUsesInvoiceOpenItems(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-006','测试供应商二','测试供应商项目二')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-007','测试供应商三','测试供应商项目三')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-008','测试供应商四','测试供应商项目四')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-009','测试供应商五','测试供应商项目五')`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-006','2026-03','contract_revenue_cost','成本-月度结算',100,0,'是',100)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-007','2026-03','contract_revenue_cost','成本-月度结算',80,0,'是',80)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-008','2026-03','contract_revenue_cost','成本-月度结算',70,0,'是',70)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-009','2026-03','contract_revenue_cost','成本-月度结算',60,10,'是',60)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v\n%s", err, stmt)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 28, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("25年至26年未付款的项目及对应金额有哪些？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(600) {
		t.Fatalf("total = %v, want supplier invoice unpaid 600", got)
	}
	if !strings.Contains(res.Message, "已收票未付款 600.00") {
		t.Fatalf("roster should use supplier invoice unpaid metric, got %q", res.Message)
	}
	for _, want := range []string{
		"测试供应商-测试供应商项目",
		"测试供应商二-测试供应商项目二",
		"测试供应商三-测试供应商项目三",
		"测试供应商四-测试供应商项目四",
		"测试供应商五-测试供应商项目五",
	} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("message should list all supplier invoice unpaid items; missing %q in %q", want, res.Message)
		}
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	items, ok := summary["invoice_unpaid_items"].([]map[string]any)
	if !ok || len(items) != 5 {
		t.Fatalf("invoice_unpaid_items = %#v, want five invoice unpaid items", summary["invoice_unpaid_items"])
	}
	if got := items[0]["open_amount"]; got != float64(300) {
		t.Fatalf("invoice_unpaid_items[0].open_amount = %v, want 300", got)
	}
}

func TestUnpaidProjectRosterRollsSingleContractMergedRowsIntoProjectLabel(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE fin_cost_settlement_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, customer_name TEXT, year_month TEXT, settlement_amount REAL, paid_amount REAL, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlement_group_members (group_id INTEGER, contract_id TEXT, source_row_number INTEGER)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-010','单合同供应商','单合同项目')`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-010','2026-02','contract_revenue_cost','成本-月度结算',600,0,'是',600)`,
		`INSERT INTO fin_cost_settlement_groups(id, customer_name, year_month, settlement_amount, paid_amount, invoice_amount) VALUES (10,'单合同供应商','2026-03',900,0,900)`,
		`INSERT INTO fin_cost_settlement_group_members(group_id, contract_id, source_row_number) VALUES (10,'C-010',1)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v\n%s", err, stmt)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月到2026年3月未付款的项目及对应金额有哪些？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if !strings.Contains(res.Message, "单合同供应商-单合同项目 已收票 1500.00 元、已付款 0.00 元、未付款 1500.00 元") {
		t.Fatalf("message should roll direct and single-contract merged rows into the project label, got %q", res.Message)
	}
	if strings.Contains(res.Message, "合并行合计（覆盖合同/项目：单合同项目）") {
		t.Fatalf("single-contract merged row should not expose merged label, got %q", res.Message)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	items, ok := summary["invoice_unpaid_items"].([]map[string]any)
	if !ok {
		t.Fatalf("invoice_unpaid_items = %#v, want item payload", summary["invoice_unpaid_items"])
	}
	var found bool
	for _, item := range items {
		if item["supplier_name"] == "单合同供应商" && item["contract_content"] == "单合同项目" {
			found = true
			if got := item["open_amount"]; got != float64(1500) {
				t.Fatalf("single-contract item open_amount = %v, want 1500", got)
			}
		}
	}
	if !found {
		t.Fatalf("invoice_unpaid_items should include rolled-up single-contract item, got %#v", items)
	}
}

func TestUnpaidProjectRosterLooseYearRangeDoesNotResolveSyntheticEntity(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-004','南京众信数通智能科技有限公司','对应-众信成本')`); err != nil {
		t.Fatalf("insert live-like contract content: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 28, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("25年至26年未付款的项目及对应金额有哪些？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(300) {
		t.Fatalf("total = %v, want supplier invoice unpaid 300", got)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["period_from"]; got != "2026-03" {
		t.Fatalf("query_spec.period_from = %v, want actual project data start 2026-03", got)
	}
	if got := spec["period_to"]; got != "2026-03" {
		t.Fatalf("query_spec.period_to = %v, want actual project data end 2026-03", got)
	}
	if got := spec["query_family"]; got != QueryFamilyCoreMetric {
		t.Fatalf("query_family = %v, want %v", got, QueryFamilyCoreMetric)
	}
	if got := spec["needs_contract_dimension"]; got != false {
		t.Fatalf("needs_contract_dimension = %v, want false", got)
	}
	if got := spec["entity"]; got != "" {
		t.Fatalf("query_spec.entity = %v, want empty", got)
	}
	if entity := res.Data["entity"]; entity != nil && entity != "" {
		t.Fatalf("result entity = %v, want empty", entity)
	}
	if strings.Contains(res.Message, "没有识别到合同/项目主体") || strings.Contains(res.Message, "合同口径当前不能直接回答") {
		t.Fatalf("project roster question should answer company-scope payable items, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "测试供应商") || !strings.Contains(res.Message, "测试供应商项目") || !strings.Contains(res.Message, "未付款 300.00") {
		t.Fatalf("message should list unpaid supplier-side project items, got %q", res.Message)
	}
}

func TestUnpaidProjectRosterLooseYearRangeUsesActualProjectDataStart(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`UPDATE fin_fund_income SET year_month='2025-10' WHERE contract_id='C-001'`); err != nil {
		t.Fatalf("move project workbook coverage start: %v", err)
	}
	if _, err := db.Exec(`UPDATE fin_cost_settlements SET year_month='2026-01' WHERE contract_id='C-002'`); err != nil {
		t.Fatalf("move cost fixture after coverage start: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-005','测试供应商','后续已结清项目')`); err != nil {
		t.Fatalf("insert latest-period contract: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-005','2026-05','contract_revenue_cost','成本-月度结算',100,100,'是',100)`); err != nil {
		t.Fatalf("insert latest-period cost row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 28, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("25年至26年未付款的项目及对应金额有哪些？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["period"]; got != "2025-10~2026-05" {
		t.Fatalf("period = %v, want actual project data range 2025-10~2026-05", got)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["period_from"]; got != "2025-10" {
		t.Fatalf("query_spec.period_from = %v, want 2025-10", got)
	}
	if got := spec["period_to"]; got != "2026-05" {
		t.Fatalf("query_spec.period_to = %v, want 2026-05", got)
	}
	if got := res.Data["requested_period"]; got != "2025-01~2026-05" {
		t.Fatalf("requested_period = %v, want original broad request 2025-01~2026-05", got)
	}
	if !strings.Contains(res.Message, "2025-10~2026-05") || !strings.Contains(res.Message, "已收票未付款 300.00") {
		t.Fatalf("message should use actual data range and invoice-unpaid amount, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "项目成本口径") {
		t.Fatalf("message should disclose supplier-side project cost basis, got %q", res.Message)
	}
}

func TestUnpaidProjectRosterFullYearRangeUsesActualProjectDataStart(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`UPDATE fin_fund_income SET year_month='2025-10' WHERE contract_id='C-001'`); err != nil {
		t.Fatalf("move project workbook coverage start: %v", err)
	}
	if _, err := db.Exec(`UPDATE fin_cost_settlements SET year_month='2026-01' WHERE contract_id='C-002'`); err != nil {
		t.Fatalf("move cost fixture after coverage start: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-005','测试供应商','后续已结清项目')`); err != nil {
		t.Fatalf("insert latest-period contract: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-005','2026-05','contract_revenue_cost','成本-月度结算',100,100,'是',100)`); err != nil {
		t.Fatalf("insert latest-period cost row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 28, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("列一下2025年至2026年还有未付款的项目和金额。")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["period"]; got != "2025-10~2026-05" {
		t.Fatalf("period = %v, want actual project data range 2025-10~2026-05", got)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["period_from"]; got != "2025-10" {
		t.Fatalf("query_spec.period_from = %v, want 2025-10", got)
	}
	if got := spec["period_to"]; got != "2026-05" {
		t.Fatalf("query_spec.period_to = %v, want 2026-05", got)
	}
	if !strings.Contains(res.Message, "已收票未付款 300.00") {
		t.Fatalf("message should answer supplier-side unpaid project items, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "项目成本口径") {
		t.Fatalf("message should disclose supplier-side project cost basis, got %q", res.Message)
	}
}

func TestNoPaymentProjectRosterUsesSupplierInvoiceOpenItems(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 28, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("从项目口径看，2025年到2026年还有哪些项目没有付款，金额各是多少？")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["metric"]; got != "已收票未付款" {
		t.Fatalf("metric = %v, want 已收票未付款; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(300) {
		t.Fatalf("total = %v, want supplier invoice-unpaid 300", got)
	}
	if strings.Contains(res.Message, "项目结算") || strings.Contains(res.Message, "未回款") {
		t.Fatalf("no-payment project roster should not answer from revenue receivable side, got %q", res.Message)
	}
}

func TestLatestRevenueSituationUsesContractAggregate(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司", WithAsOfAnchor(time.Date(2026, time.June, 28, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("老板，帮我看看最新月份的营收情况。")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(1000) {
		t.Fatalf("total = %v, want latest project settlement revenue 1000", got)
	}
	if !strings.Contains(res.Message, "项目结算收入（营收） 1000.00") {
		t.Fatalf("message should answer latest project revenue settlement, got %q", res.Message)
	}
	if got := res.Data["metric_label"]; got != "项目结算收入（营收）" {
		t.Fatalf("metric_label = %v, want 项目结算收入（营收）; data=%+v", got, res.Data)
	}
	if got, _ := res.Data["business_basis"].(string); !strings.Contains(got, "项目口径") || !strings.Contains(got, "收入") {
		t.Fatalf("business_basis = %q, want project revenue basis; data=%+v", got, res.Data)
	}
}

func TestContractAggregateDataFinalAnswerIncludesSourceLineage(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月应收账款有多少")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	finalAnswer, _ := res.Data["final_answer"].(string)
	if !strings.Contains(finalAnswer, "项目应收（应收未收） 600.00 元") {
		t.Fatalf("data.final_answer should keep business result, got %q", finalAnswer)
	}
	if !strings.Contains(finalAnswer, "来源：") {
		t.Fatalf("data.final_answer should include source lineage, got %q", finalAnswer)
	}
}

func TestCompanyScopeContractInvoiceUnpaidQuestionDoesNotRequireSpecificContractSubject(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月已开票未付款的合同有哪些")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(300) {
		t.Fatalf("total = %v, want supplier invoice unpaid 300", got)
	}
	if strings.Contains(res.Message, "没有识别到合同/项目主体") || strings.Contains(res.Message, "合同口径当前不能直接回答") {
		t.Fatalf("company-scope contract invoice unpaid question should not require a specific subject, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "已收票未付款") || !strings.Contains(res.Message, "测试供应商") || !strings.Contains(res.Message, "测试供应商项目") {
		t.Fatalf("message should explain supplier-side invoice unpaid roster, got %q", res.Message)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != QueryFamilyCoreMetric {
		t.Fatalf("query_family = %v, want %v", got, QueryFamilyCoreMetric)
	}
	if got := spec["needs_contract_dimension"]; got != false {
		t.Fatalf("needs_contract_dimension = %v, want false", got)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	items, ok := summary["invoice_unpaid_items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("invoice_unpaid_items = %#v, want one supplier-side item", summary["invoice_unpaid_items"])
	}
	if got := items[0]["supplier_name"]; got != "测试供应商" {
		t.Fatalf("invoice_unpaid_items[0].supplier_name = %v", got)
	}
	if got := items[0]["contract_content"]; got != "测试供应商项目" {
		t.Fatalf("invoice_unpaid_items[0].contract_content = %v", got)
	}
	if got := items[0]["open_amount"]; got != float64(300) {
		t.Fatalf("invoice_unpaid_items[0].open_amount = %v, want 300", got)
	}
}

func TestCompanyScopeContractMetricQuestionUsesAggregateWithoutSyntheticSubject(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月合同收入情况")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(1000) {
		t.Fatalf("total = %v, want contract revenue 1000", got)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != QueryFamilyCoreMetric {
		t.Fatalf("query_family = %v, want %v", got, QueryFamilyCoreMetric)
	}
	if got := spec["needs_contract_dimension"]; got != false {
		t.Fatalf("needs_contract_dimension = %v, want false", got)
	}
	if strings.Contains(res.Message, "没有识别到合同/项目主体") {
		t.Fatalf("company-scope contract metric question should not require a specific subject, got %q", res.Message)
	}
}

func TestCompanyScopeCustomerRevenueDetailUsesContractAggregateWithoutSyntheticSubject(t *testing.T) {
	dbPath := buildCompanyScopeRevenueDetailDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月客户收入明细")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(3000) {
		t.Fatalf("total = %v, want contract revenue 3000", got)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["entity"]; got != "" {
		t.Fatalf("query_spec.entity = %v, want empty for dimension word", got)
	}
	if !strings.Contains(res.Message, "北京甲方有限公司") || !strings.Contains(res.Message, "上海乙方科技有限公司") {
		t.Fatalf("message should include contract revenue detail customers, got: %s", res.Message)
	}
	summary, ok := res.Data["contract_summary"].(map[string]any)
	if !ok {
		t.Fatalf("contract_summary missing: %+v", res.Data)
	}
	items, ok := summary["revenue_items"].([]map[string]any)
	if !ok || len(items) != 2 {
		t.Fatalf("contract_summary.revenue_items = %#v, want 2 items", summary["revenue_items"])
	}
}

func TestCompanyScopeContractRevenueDetailDoesNotUseRandomContractSubject(t *testing.T) {
	dbPath := buildContractARAPPriorityDB(t)
	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月合同收入明细")
	if !res.Success {
		t.Fatalf("query failed: %s data=%+v", res.Message, res.Data)
	}
	if got := res.Data["source_priority"]; got != "contract_first" {
		t.Fatalf("source_priority = %v, want contract_first; message=%s data=%+v", got, res.Message, res.Data)
	}
	if got := res.Data["total"]; got != float64(1000) {
		t.Fatalf("total = %v, want company-scope contract revenue 1000", got)
	}
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["entity"]; got != "" {
		t.Fatalf("query_spec.entity = %v, want empty; should not infer 租赁合同 from generic 合同", got)
	}
}

func buildContractARAPPriorityDB(t *testing.T) string {
	t.Helper()
	dbPath := t.TempDir() + "/contract-arap-priority.sqlite"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, opening_period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, transaction_time TEXT, transaction_type TEXT, debit_amount REAL, credit_amount REAL, balance REAL, summary TEXT, counterparty_name TEXT, counterparty_account TEXT)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, received_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, paid_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('测试公司','2026-03','1122','应收账款',0,9999)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('测试公司','2026-03','2202','应付账款',0,8888)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('测试公司','2026-03','2241','其他应付款',0,1777)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES ('测试公司','2026-03','营业收入',1000,1000)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-001','测试客户','测试客户项目')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-002','测试供应商','测试供应商项目')`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES ('C-003','测试租赁供应商','租赁合同')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES ('C-001','2026-03','contract_fund_income','26年Q1收入明细',1000,400,'是',800)`,
		`INSERT INTO fin_cost_settlements(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, paid_amount, is_invoiced, invoice_amount) VALUES ('C-002','2026-03','contract_revenue_cost','成本-月度结算',700,200,'是',500)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v\n%s", err, stmt)
		}
	}
	return dbPath
}

func buildOfficialOnlyARAPDB(t *testing.T) string {
	t.Helper()
	dbPath := t.TempDir() + "/official-only-arap.sqlite"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, opening_period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, transaction_time TEXT, transaction_type TEXT, debit_amount REAL, credit_amount REAL, balance REAL, summary TEXT, counterparty_name TEXT, counterparty_account TEXT)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, received_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, paid_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('测试公司','2026-03','1122','应收账款',0,9999)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('测试公司','2026-03','2202','应付账款',0,8888)`,
		`INSERT INTO balance_sheet(company, period, account_code, account_name, opening_balance, closing_balance) VALUES ('测试公司','2026-03','2241','其他应付款',0,1777)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v\n%s", err, stmt)
		}
	}
	return dbPath
}

func buildCompanyScopeRevenueDetailDB(t *testing.T) string {
	t.Helper()
	dbPath := t.TempDir() + "/company-scope-revenue-detail.sqlite"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE balance_sheet (company TEXT, period TEXT, account_code TEXT, account_name TEXT, opening_balance REAL, closing_balance REAL)`,
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, credit_amount REAL, debit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE fin_contracts (contract_id TEXT PRIMARY KEY, customer_name TEXT, contract_content TEXT)`,
		`CREATE TABLE fin_fund_income (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, received_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`CREATE TABLE fin_cost_settlements (id INTEGER PRIMARY KEY AUTOINCREMENT, contract_id TEXT, year_month TEXT, source_report_type TEXT, source_sheet_name TEXT, settlement_amount REAL, paid_amount REAL, is_invoiced TEXT, invoice_amount REAL)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content) VALUES
		 ('R-001','北京甲方有限公司','年度数据服务'),
		 ('R-002','上海乙方科技有限公司','市场调研服务')`,
		`INSERT INTO fin_fund_income(contract_id, year_month, source_report_type, source_sheet_name, settlement_amount, received_amount, is_invoiced, invoice_amount) VALUES
		 ('R-001','2026-03','contract_fund_income','26年Q1收入明细',1000,800,'是',1000),
		 ('R-002','2026-03','contract_fund_income','26年Q1收入明细',2000,1500,'是',2000)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount) VALUES
		 ('测试公司','2026-03','营业收入',9999,9999),
		 ('测试公司','2026-03','营业成本',100,100),
		 ('测试公司','2026-03','净利润',9899,9899)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}
	return dbPath
}
