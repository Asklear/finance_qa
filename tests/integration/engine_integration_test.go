package integration_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/query"
)

func TestEngineCoreQueriesAgainstSQLite(t *testing.T) {
	dbPath := setupQueryTestDB(t)
	eng, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	if eng.Company != "模拟财务科技有限公司" {
		t.Fatalf("resolved company = %q, want 模拟财务科技有限公司", eng.Company)
	}

	res := eng.Query("2026年2月货币资金余额是多少")
	if !res.Success {
		t.Fatalf("precise query failed: %s", res.Message)
	}
	if v := numberFromMap(t, res.Data, "closing_balance"); v != 150 {
		t.Fatalf("closing balance = %.2f, want 150", v)
	}

	income := eng.Query("2026年2月收入是多少")
	if !income.Success {
		t.Fatalf("income query failed: %s", income.Message)
	}
	if m, ok := income.Data["metric"].(string); !ok || m != "收入" {
		t.Fatalf("income metric should be 收入, got %v", income.Data["metric"])
	}
	if _, ok := income.Data["dual_perspective"].(map[string]any); !ok {
		t.Fatalf("income should include dual_perspective, got %T", income.Data["dual_perspective"])
	}
	if v, ok := income.Data["现金流入"]; !ok || v.(float64) != 1500 {
		t.Errorf("expected 现金流入=1500, got %v", v)
	}

	expense := eng.Query("2026年2月支出是多少")
	if !expense.Success {
		t.Fatalf("expense query failed: %s", expense.Message)
	}
	if v := numberFromMap(t, expense.Data, "现金流出"); v != 350 {
		t.Fatalf("expense total = %.2f, want 350", v)
	}

	profit := eng.Query("2026年2月利润是多少")
	if !profit.Success {
		t.Fatalf("profit query failed: %s", profit.Message)
	}
	if v := numberFromMap(t, profit.Data, "account_value"); v != 700 {
		t.Fatalf("account_value = %.2f, want 700", v)
	}
	if v := numberFromMap(t, profit.Data, "净现金流"); v != 1150 {
		t.Fatalf("net cash = %.2f, want 1150", v)
	}
	if strings.Contains(profit.Message, "银行卡上看") || strings.Contains(profit.Message, "账上看") {
		t.Fatalf("profit answer should stay single-accrual in message, got %s", profit.Message)
	}
	if !containsText(profit.ExecutedSQL, "monthlyBookSummary(income_statement)") {
		t.Fatalf("profit should expose monthly book trace, got %v", profit.ExecutedSQL)
	}

	multiMetric := eng.Query("2026年2月收入/成本/利润分别是多少")
	if !multiMetric.Success {
		t.Fatalf("multi metric query failed: %s", multiMetric.Message)
	}
	if !strings.Contains(multiMetric.Message, "收入") || !strings.Contains(multiMetric.Message, "成本") || !strings.Contains(multiMetric.Message, "利润") {
		t.Fatalf("multi metric message should contain 收入/成本/利润, got: %s", multiMetric.Message)
	}
	switch rm := multiMetric.Data["requested_metrics"].(type) {
	case []any:
		if len(rm) != 3 {
			t.Fatalf("requested_metrics should expose 3 metrics, got %v", multiMetric.Data["requested_metrics"])
		}
	case []string:
		if len(rm) != 3 {
			t.Fatalf("requested_metrics should expose 3 metrics, got %v", multiMetric.Data["requested_metrics"])
		}
	default:
		t.Fatalf("requested_metrics should expose 3 metrics, got %v", multiMetric.Data["requested_metrics"])
	}

	tax := eng.Query("2026年2月增值税是多少")
	if !tax.Success {
		t.Fatalf("tax query failed: %s", tax.Message)
	}
	if v := numberFromMap(t, tax.Data, "total_output"); v != 130 {
		t.Fatalf("output vat = %.2f, want 130", v)
	}
	if v := numberFromMap(t, tax.Data, "total_input"); v != 20 {
		t.Fatalf("input vat = %.2f, want 20", v)
	}
	if v := numberFromMap(t, tax.Data, "net_vat"); v != 110 {
		t.Fatalf("net vat = %.2f, want 110", v)
	}

	ar := eng.Query("2026年2月应收账款情况")
	if !ar.Success {
		t.Fatalf("ar query failed: %s", ar.Message)
	}
	if v := numberFromMap(t, ar.Data, "total"); v != 1200 {
		t.Fatalf("ar total = %.2f, want 1200", v)
	}
	if details, ok := ar.Data["details"].([]map[string]any); !ok || len(details) == 0 {
		t.Fatalf("ar details missing or empty, got %v", ar.Data["details"])
	}

	ap := eng.Query("2026年2月应付账款情况")
	if !ap.Success {
		t.Fatalf("ap query failed: %s", ap.Message)
	}
	if v := numberFromMap(t, ap.Data, "total"); v != 700 {
		t.Fatalf("ap total = %.2f, want 700", v)
	}
	if details, ok := ap.Data["details"].([]map[string]any); !ok || len(details) == 0 {
		t.Fatalf("ap details missing or empty, got %v", ap.Data["details"])
	}

	analysis := eng.Query("2026年2月账龄分析")
	if !analysis.Success {
		t.Fatalf("analysis query failed: %s", analysis.Message)
	}
	if v := numberFromMap(t, analysis.Data, "receivable_total"); v <= 0 {
		t.Fatalf("analysis receivable_total = %.2f, want > 0", v)
	}

	supplierCount := eng.Query("2026年2月供应商多少")
	if !supplierCount.Success {
		t.Fatalf("supplier count fallback failed: %s", supplierCount.Message)
	}
	if v := numberFromMap(t, supplierCount.Data, "count"); v <= 0 {
		t.Fatalf("supplier count = %.2f, want > 0", v)
	}

	hrCost := eng.Query("2026年2月人力成本多少")
	if !hrCost.Success {
		t.Fatalf("hr cost fallback failed: %s", hrCost.Message)
	}
	if v := numberFromMap(t, hrCost.Data, "total"); v != 300 {
		t.Fatalf("hr cost = %.2f, want 300", v)
	}
	hrCostByPayroll := eng.Query("2026年2月应付职工薪酬是多少")
	if !hrCostByPayroll.Success {
		t.Fatalf("payroll phrase should still route to hr cost fallback, got: %s", hrCostByPayroll.Message)
	}
	if v := numberFromMap(t, hrCostByPayroll.Data, "total"); v != 300 {
		t.Fatalf("payroll phrase hr cost = %.2f, want 300", v)
	}

	customerSales := eng.Query("客户A客户2月销售额多少")
	if !customerSales.Success {
		t.Fatalf("customer sales fallback failed: %s", customerSales.Message)
	}
	if v := numberFromMap(t, customerSales.Data, "total"); v != 1000 {
		t.Fatalf("customer sales = %.2f, want 1000", v)
	}

	hostPayload := eng.Query("给宿主LLM输出2026年2月全量财报原始数据")
	if !hostPayload.Success {
		t.Fatalf("host payload query failed: %s", hostPayload.Message)
	}
	if method, ok := hostPayload.Data["answer_method"].(string); !ok || method != "llm_payload" {
		t.Fatalf("host payload answer_method should be llm_payload, got %v", hostPayload.Data["answer_method"])
	}
	if _, ok := hostPayload.Data["llm_payload"].(map[string]any); !ok {
		t.Fatalf("host payload should include llm_payload map, got %T", hostPayload.Data["llm_payload"])
	}
}

func setupQueryTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "query_test.db")

	sql := `
CREATE TABLE balance_sheet (
  company TEXT,
  period TEXT,
  account_code TEXT,
  account_name TEXT,
  opening_balance REAL,
  closing_balance REAL
);
CREATE TABLE income_statement (
  company TEXT,
  period TEXT,
  item_name TEXT,
  current_amount REAL,
  cumulative_amount REAL
);
CREATE TABLE budget (
  company TEXT,
  period TEXT
);
CREATE TABLE bank_statement (
  company TEXT,
  transaction_date TEXT,
  credit_amount REAL,
  debit_amount REAL,
  counterparty_name TEXT,
  summary TEXT
);
CREATE TABLE journal (
  company TEXT,
  period TEXT,
  voucher_date TEXT,
  voucher_no TEXT,
  account_code TEXT,
  account_name TEXT,
  summary TEXT,
  direction TEXT,
  amount REAL,
  debit_amount REAL,
  credit_amount REAL,
  counterparty TEXT
);
CREATE TABLE cas_mapping (
  standard_code TEXT PRIMARY KEY,
  standard_name TEXT NOT NULL,
  category TEXT
);

INSERT INTO cas_mapping VALUES ('1002','银行存款','资产');
INSERT INTO cas_mapping VALUES ('1122','应收账款','资产');
INSERT INTO cas_mapping VALUES ('2202','应付账款','负债');
INSERT INTO cas_mapping VALUES ('2211','应付职工薪酬','负债');

INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','2026-02','1002','货币资金',100,150);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','2026-02','112201','应收账款-客户A',500,600);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','2026-02','112202','应收账款-客户B',500,600);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','2026-02','2202','应付账款',500,700);
INSERT INTO balance_sheet VALUES ('模拟财务科技有限公司','2026-02','2211','应付职工薪酬',200,300);
INSERT INTO balance_sheet VALUES ('苏州模拟财务','2026-02','1002','货币资金',10,20);

INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业收入',2000,3000);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业成本',1000,1500);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','管理费用',300,450);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','净利润',700,1050);

INSERT INTO bank_statement VALUES ('模拟财务科技有限公司','2026-02-10',1000,0,'客户A','回款');
INSERT INTO bank_statement VALUES ('模拟财务科技有限公司','2026-02-11',0,300,'供应商B','付款');
INSERT INTO bank_statement VALUES ('模拟财务科技有限公司','2026-02-12',500,50,'客户C','手续费');

INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-15','V001','222101','应交税费-销项税','销项税','贷',130,0,130,'税局');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-16','V002','222102','应交税费-进项税','进项税','借',20,20,0,'税局');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-18','V003','112201','应收账款-客户A','销售','借',900,900,0,'客户A');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-01-01','V004','112201','应收账款-客户B','销售','借',300,300,0,'客户B');
`
	runSQLite(t, dbPath, sql)
	return dbPath
}

func runSQLite(t *testing.T, dbPath string, sql string) {
	t.Helper()
	cmd := exec.Command("sqlite3", dbPath)
	cmd.Stdin = stringsReader(sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 failed: %v\n%s", err, string(out))
	}
}

func stringsReader(s string) *os.File {
	f, err := os.CreateTemp("", "sqlite-input-*.sql")
	if err != nil {
		panic(err)
	}
	if _, err := f.WriteString(s); err != nil {
		panic(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		panic(err)
	}
	return f
}

func containsText(lines []string, keyword string) bool {
	for _, line := range lines {
		if strings.Contains(line, keyword) {
			return true
		}
	}
	return false
}

func numberFromMap(t *testing.T, data map[string]any, key string) float64 {
	t.Helper()
	v, ok := data[key]
	if !ok {
		t.Fatalf("missing key %q in data: %+v", key, data)
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		t.Fatalf("key %q has unsupported type %T", key, v)
	}
	return 0
}
