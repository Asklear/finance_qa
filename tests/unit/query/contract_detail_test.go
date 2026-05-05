package query_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestContractDetailClauseQuestionRoutesToContractDetail(t *testing.T) {
	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("百度边缘计算资源服务协议付款条款是什么？", now)
	if spec.QueryFamily != query.QueryFamilyContractDetail {
		t.Fatalf("QueryFamily = %s, want %s", spec.QueryFamily, query.QueryFamilyContractDetail)
	}
}

func TestContractDetailInvoiceQuestionRoutesToContractDetail(t *testing.T) {
	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("百度边缘计算资源服务协议发票金额和开票日期是多少？", now)
	if spec.QueryFamily != query.QueryFamilyContractDetail {
		t.Fatalf("QueryFamily = %s, want %s", spec.QueryFamily, query.QueryFamilyContractDetail)
	}
}

func TestContractOperatingQuestionStillUsesExistingContractFlow(t *testing.T) {
	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("2026年3月已开票未回款的项目有哪些？", now)
	if spec.QueryFamily == query.QueryFamilyContractDetail {
		t.Fatalf("operating aggregate question must not route to contract_detail")
	}
	if !spec.PreferContractAggregate {
		t.Fatalf("operating aggregate question should keep existing contract aggregate route")
	}
}

func TestContractDetailProbeClauseChoosesMainAndPageTables(t *testing.T) {
	engine := newContractDetailTestEngine(t)
	defer engine.Close()

	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("百度边缘计算资源服务协议付款条款是什么？", now)
	probe := engine.ProbeContractDetailSources(spec)

	if probe.Intent != query.ContractDetailIntentClause {
		t.Fatalf("intent = %s, want %s", probe.Intent, query.ContractDetailIntentClause)
	}
	if !hasString(probe.CandidateTables, "contract_main") || !hasString(probe.CandidateTables, "contract_pages") {
		t.Fatalf("candidate tables = %#v", probe.CandidateTables)
	}
	if probe.MatchedContractRows == 0 {
		t.Fatalf("expected matched contract rows, got 0")
	}
	if probe.NeedsPageText {
		t.Fatalf("structured clause question should not read page text until fallback is needed")
	}
}

func TestContractDetailProbeInvoiceChoosesInvoiceTables(t *testing.T) {
	engine := newContractDetailTestEngine(t)
	defer engine.Close()

	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("百度边缘计算资源服务协议发票金额和开票日期是多少？", now)
	probe := engine.ProbeContractDetailSources(spec)

	if probe.Intent != query.ContractDetailIntentInvoice {
		t.Fatalf("intent = %s, want %s", probe.Intent, query.ContractDetailIntentInvoice)
	}
	if hasString(probe.CandidateTables, "contract_invoice_summaries") || !hasString(probe.CandidateTables, "contract_invoices") {
		t.Fatalf("candidate tables = %#v", probe.CandidateTables)
	}
	if !probe.HasStructuredAnswer {
		t.Fatalf("invoice questions should have structured answer when invoice rows exist")
	}
}

func TestContractDetailProbePageTextChoosesPages(t *testing.T) {
	engine := newContractDetailTestEngine(t)
	defer engine.Close()

	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("百度边缘计算资源服务协议第几页写了验收？", now)
	probe := engine.ProbeContractDetailSources(spec)

	if probe.Intent != query.ContractDetailIntentPage {
		t.Fatalf("intent = %s, want %s", probe.Intent, query.ContractDetailIntentPage)
	}
	if !hasString(probe.CandidateTables, "contract_pages") {
		t.Fatalf("candidate tables = %#v", probe.CandidateTables)
	}
}

func TestContractDetailProbeNotUsedForOperatingAmountQuestion(t *testing.T) {
	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("2026年3月收入、成本、利润分别是多少？", now)
	if spec.QueryFamily == query.QueryFamilyContractDetail {
		t.Fatalf("operating amount question must not route to contract_detail")
	}
}

func TestContractOperatingQuestionsDoNotRouteToContractDetail(t *testing.T) {
	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	for _, question := range []string{
		"2026年3月收入、成本、利润分别是多少？",
		"金程今年回款情况",
		"有哪些项目已开票未回款",
		"2026年3月已开票未付款的合同有哪些",
		"2026年3月供应商有哪些",
	} {
		spec := query.BuildQuerySpec(question, now)
		if spec.QueryFamily == query.QueryFamilyContractDetail {
			t.Fatalf("%q routed to contract_detail", question)
		}
	}
}

func TestContractDetailPlanAndRegistryUseDetailSource(t *testing.T) {
	engine := newContractDetailTestEngine(t)
	defer engine.Close()

	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	spec := query.BuildQuerySpec("百度边缘计算资源服务协议发票金额是多少？", now)
	plan := query.PlanQuerySpec(spec)
	if !plan.Requires(query.SourceCapabilityContractDetail) {
		t.Fatalf("plan capabilities = %#v, want contract detail", plan.Capabilities)
	}
	registry := query.NewDefaultSourceRegistry(engine)
	adapter, ok := registry.Resolve(query.SourceCapabilityContractDetail)
	if !ok {
		t.Fatalf("default registry missing contract detail adapter")
	}
	if adapter.Name() != "contract_detail" {
		t.Fatalf("adapter name = %s, want contract_detail", adapter.Name())
	}
	factSet, err := adapter.Fetch(context.Background(), spec)
	if err != nil {
		t.Fatalf("fetch contract detail: %v", err)
	}
	if factSet.Source != "contract_detail" || len(factSet.Facts) == 0 {
		t.Fatalf("factSet = %#v", factSet)
	}
}

func TestContractDetailClauseQueryReturnsPaymentTermsSafely(t *testing.T) {
	engine := newContractDetailTestEngine(t)
	defer engine.Close()

	res := engine.Query("百度边缘计算资源服务协议付款条款是什么？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "百度边缘计算资源服务协议") || !strings.Contains(res.Message, "30日内付款") {
		t.Fatalf("message should contain contract title and payment terms, got: %s", res.Message)
	}
	assertContractDetailResultSafe(t, res)
	spec, ok := res.Data["query_spec"].(map[string]any)
	if !ok {
		t.Fatalf("query_spec missing: %+v", res.Data)
	}
	if got := spec["query_family"]; got != query.QueryFamilyContractDetail {
		t.Fatalf("query_family = %v, want %v", got, query.QueryFamilyContractDetail)
	}
	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	if !hasString(sourceTables, "contract_main") {
		t.Fatalf("source_tables = %#v", sourceTables)
	}
}

func TestContractDetailInvoiceQueryReturnsAmountsAndDatesSafely(t *testing.T) {
	engine := newContractDetailTestEngine(t)
	defer engine.Close()

	res := engine.Query("百度边缘计算资源服务协议发票金额和开票日期是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	for _, want := range []string{"百度边缘计算资源服务协议", "83000.00", "41500.00", "2026-03-28"} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("message should contain %q, got: %s", want, res.Message)
		}
	}
	assertContractDetailResultSafe(t, res)
	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	if hasString(sourceTables, "contract_invoice_summaries") || !hasString(sourceTables, "contract_invoices") {
		t.Fatalf("source_tables = %#v", sourceTables)
	}
}

func TestContractDetailInvoiceQueryAggregatesWithoutSummaryTable(t *testing.T) {
	dbPath := buildContractDetailTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE contract_invoice_summaries`); err != nil {
		t.Fatalf("drop summary table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	engine, err := query.NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("百度边缘计算资源服务协议发票金额和开票日期是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	for _, want := range []string{"累计开票83000.00元", "最近开票日期2026-03-28", "发票号INV-EDGE-002"} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("message should contain %q, got: %s", want, res.Message)
		}
	}
	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	if hasString(sourceTables, "contract_invoice_summaries") || !hasString(sourceTables, "contract_invoices") {
		t.Fatalf("source_tables = %#v", sourceTables)
	}
}

func TestContractDetailPageFallbackUsesMarkdownText(t *testing.T) {
	dbPath := buildContractDetailTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`UPDATE contract_main SET payment_terms = '' WHERE contract_title = '百度边缘计算资源服务协议'`); err != nil {
		t.Fatalf("clear payment_terms: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	engine, err := query.NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("百度边缘计算资源服务协议付款条款是什么？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if !strings.Contains(res.Message, "第2页") || !strings.Contains(res.Message, "验收通过并收到合格发票后30日内付款") {
		t.Fatalf("message should include page fallback excerpt, got: %s", res.Message)
	}
	assertContractDetailResultSafe(t, res)
}

func TestContractDetailIgnoresDeletedFeishuContracts(t *testing.T) {
	engine := newContractDetailTestEngine(t)
	defer engine.Close()

	res := engine.Query("已删合同付款条款是什么？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}
	if strings.Contains(res.Message, "已删合同") || strings.Contains(res.Message, "不应返回") {
		t.Fatalf("deleted contract should not be returned: %s", res.Message)
	}
	if !strings.Contains(res.Message, "当前没有在合同明细库里匹配到这份合同") {
		t.Fatalf("message should explain no active match, got: %s", res.Message)
	}
}

func newContractDetailTestEngine(t *testing.T) *query.Engine {
	t.Helper()
	dbPath := buildContractDetailTestDB(t)
	engine, err := query.NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return engine
}

func buildContractDetailTestDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "contract-detail.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE contract_main (
			id INTEGER PRIMARY KEY,
			contract_number TEXT,
			contract_title TEXT,
			party_a TEXT,
			party_b TEXT,
			sign_date TEXT,
			start_date TEXT,
			end_date TEXT,
			contract_amount REAL,
			amount_currency TEXT,
			settlement_cycle TEXT,
			settlement_unit_price TEXT,
			payment_terms TEXT,
			payment_method TEXT,
			tax_rate REAL,
			service_scope TEXT,
			file_name TEXT,
			sync_status TEXT
		)`,
		`CREATE TABLE contract_pages (
			id INTEGER PRIMARY KEY,
			contract_id INTEGER,
			page_num INTEGER,
			page_number INTEGER,
			markdown_text TEXT,
			plain_text TEXT
		)`,
		`CREATE TABLE contract_invoice_summaries (
			id INTEGER PRIMARY KEY,
			contract_id INTEGER,
			invoice_count INTEGER,
			total_invoiced_amount REAL,
			total_tax_amount REAL,
			contract_amount REAL,
			invoiced_ratio REAL,
			latest_invoice_date TEXT,
			latest_invoice_number TEXT
		)`,
		`CREATE TABLE contract_invoices (
			id INTEGER PRIMARY KEY,
			contract_id INTEGER,
			invoice_number TEXT,
			issue_date TEXT,
			buyer_name TEXT,
			seller_name TEXT,
			total_amount_without_tax REAL,
			total_tax_amount REAL,
			total_amount REAL,
			remarks TEXT,
			items_json TEXT
		)`,
		`CREATE TABLE fin_contracts (
			contract_id TEXT PRIMARY KEY,
			customer_name TEXT,
			contract_content TEXT
		)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
			VALUES ('C013', '百度在线网络技术(北京)有限公司', '边缘计算资源服务协议')`,
		`INSERT INTO contract_main(
			id, contract_number, contract_title, party_a, party_b, sign_date,
			start_date, end_date, contract_amount, amount_currency, settlement_cycle,
			settlement_unit_price, payment_terms, payment_method, tax_rate, service_scope, file_name
		) VALUES (
			1, 'BAIDU-EDGE-2026', '百度边缘计算资源服务协议',
			'百度在线网络技术(北京)有限公司', '南京优集数据科技有限公司',
			'2026-01-05', '2026-01-01', '2026-12-31', 124500, 'CNY',
			'按月结算', '资源用量单价', '甲方收到合格发票后30日内付款',
			'银行转账', 0.06, '边缘计算资源服务', '百度边缘计算资源服务协议.pdf'
		)`,
		`INSERT INTO contract_main(
			id, contract_number, contract_title, party_a, party_b, sign_date,
			start_date, end_date, contract_amount, amount_currency, settlement_cycle,
			settlement_unit_price, payment_terms, payment_method, tax_rate, service_scope, file_name, sync_status
		) VALUES (
			2, 'DELETED-2026', '已删合同',
			'已删除客户', '南京优集数据科技有限公司',
			'2026-02-01', '2026-02-01', '2026-12-31', 10000, 'CNY',
			'按月结算', '固定单价', '不应返回',
			'银行转账', 0.06, '已删除服务', '已删合同.pdf', 'deleted'
		)`,
		`INSERT INTO contract_pages(id, contract_id, page_num, page_number, markdown_text, plain_text) VALUES
			(11, 1, 1, 1, '# 服务范围\n乙方提供边缘计算资源服务。', '乙方提供边缘计算资源服务。'),
			(12, 1, 2, 2, '## 验收与付款\n甲方验收通过并收到合格发票后30日内付款。', '甲方验收通过并收到合格发票后30日内付款。')`,
		`INSERT INTO contract_invoice_summaries(
			id, contract_id, invoice_count, total_invoiced_amount, total_tax_amount,
			contract_amount, invoiced_ratio, latest_invoice_date, latest_invoice_number
		) VALUES (21, 1, 2, 83000, 4698.11, 124500, 0.6667, '2026-03-28', 'INV-EDGE-002')`,
		`INSERT INTO contract_invoices(
			id, contract_id, invoice_number, issue_date, buyer_name, seller_name,
			total_amount_without_tax, total_tax_amount, total_amount, remarks, items_json
		) VALUES
			(31, 1, 'INV-EDGE-001', '2026-02-28', '百度在线网络技术(北京)有限公司', '南京优集数据科技有限公司', 39150.94, 2349.06, 41500, '2月服务费', '[]'),
			(32, 1, 'INV-EDGE-002', '2026-03-28', '百度在线网络技术(北京)有限公司', '南京优集数据科技有限公司', 39150.94, 2349.06, 41500, '3月服务费', '[]')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec fixture: %v\nsql: %s", err, stmt)
		}
	}
	return dbPath
}

func hasString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func assertContractDetailResultSafe(t *testing.T, res query.Result) {
	t.Helper()
	text := res.Message
	for _, forbidden := range []string{
		"contract_id", "page_id", "storage_key", "file_hash", "job_id", "raw_ocr_json",
		"C013", "executed_sql", "SELECT ",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("message leaked %q: %s", forbidden, text)
		}
	}
	dataText := stringifyForContractDetailTest(res.Data)
	for _, forbidden := range []string{"contract_id", "page_id", "storage_key", "file_hash", "job_id", "raw_ocr_json", "C013"} {
		if strings.Contains(dataText, forbidden) {
			t.Fatalf("data leaked %q: %s", forbidden, dataText)
		}
	}
}

func stringifyForContractDetailTest(v any) string {
	switch typed := v.(type) {
	case map[string]any:
		parts := make([]string, 0, len(typed))
		for key, value := range typed {
			parts = append(parts, key+"="+stringifyForContractDetailTest(value))
		}
		return strings.Join(parts, ";")
	case []map[string]any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, stringifyForContractDetailTest(item))
		}
		return strings.Join(parts, ";")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, stringifyForContractDetailTest(item))
		}
		return strings.Join(parts, ";")
	case []string:
		return strings.Join(typed, ";")
	default:
		return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(
			strings.TrimSpace(anyToStringForContractDetailTest(typed)), "\n", " "), "\t", " "), "\r", " "), "  ", " "))
	}
}

func anyToStringForContractDetailTest(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(fmt.Sprint(v)), "\n", " "), "\t", " "))
}
