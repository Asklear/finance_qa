package integration_test

import (
	"database/sql"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestReconciliationAnalysis_Feb2026ExpertFeedback(t *testing.T) {
	dbPath := setupReconciliationAnalysisTestDB(t)

	eng, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Close()

	t.Run("利润差异解释要和财务专家口径一致", func(t *testing.T) {
		res := eng.Query("2026年2月实际利润多少，为什么和银行卡上差这么多？看看具体差异到底怎么回事")
		assertTraceBundle(t, res)

		if !res.Success {
			t.Fatalf("expected success, got message=%q", res.Message)
		}
		mustContainAll(t, res.Message,
			"账上看收入 2485230.69 元",
			"净利润 -2756.97 元",
			"银行卡上看",
			"辽宁金程信息科技有限公司",
			"历史应收回款",
			"飞未云科(深圳)技术有限公司",
			"销项税",
			"南京林悦智能科技有限公司",
			"供应商",
			"南京汇智互娱教育科技有限公司",
			"供应商",
		)
		mustNotContainAny(t, res.Message,
			"预收未确认收入",
			"2025年11月结算款",
			"2025年12月结算款",
		)

		highlights := mustSliceOfMaps(t, res.Data["highlights"], "highlights")
		if len(highlights) < 4 {
			t.Fatalf("expected at least 4 highlights, got %d", len(highlights))
		}

		jincheng := findHighlight(t, highlights, "辽宁金程信息科技有限公司")
		if got := asString(jincheng["comparison_basis"]); got != "historical_receipt_and_current_revenue" {
			t.Fatalf("jincheng comparison_basis = %q, want historical_receipt_and_current_revenue", got)
		}
		if got := asString(jincheng["role"]); got != "customer" {
			t.Fatalf("jincheng role = %q, want customer", got)
		}

		flywei := findHighlight(t, highlights, "飞未云科(深圳)技术有限公司")
		if got := numberFromMap(t, flywei, "output_vat"); !floatAlmostEqual(got, 25477.36) {
			t.Fatalf("flywei output_vat = %.2f, want 25477.36", got)
		}
		if got := asString(flywei["comparison_basis"]); got != "vat_gap_only" {
			t.Fatalf("flywei comparison_basis = %q, want vat_gap_only", got)
		}

		linyue := findHighlight(t, highlights, "南京林悦智能科技有限公司")
		if got := asString(linyue["comparison_basis"]); got != "supplier_payment_or_cost" {
			t.Fatalf("linyue comparison_basis = %q, want supplier_payment_or_cost", got)
		}
		if got := asString(linyue["role"]); got != "supplier" {
			t.Fatalf("linyue role = %q, want supplier", got)
		}

		huizhi := findHighlight(t, highlights, "南京汇智互娱教育科技有限公司")
		if got := asString(huizhi["comparison_basis"]); got != "supplier_payment_or_cost" {
			t.Fatalf("huizhi comparison_basis = %q, want supplier_payment_or_cost", got)
		}
	})

	t.Run("金程销售额不能回退到公司总收入", func(t *testing.T) {
		res := eng.Query("辽宁金程信息科技有限公司2月销售额多少")
		assertTraceBundle(t, res)

		if !res.Success {
			t.Fatalf("expected success, got %q", res.Message)
		}
		mustContainAll(t, res.Message, "账上确认收入 2010161.88 元", "历史应收回款")
		mustNotContainAny(t, res.Message, "2485230.69")

		if got := numberFromMap(t, res.Data, "amount"); !floatAlmostEqual(got, 2010161.88) {
			t.Fatalf("jincheng amount = %.2f, want 2010161.88", got)
		}
		if got, _ := res.Data["role"].(string); got != "customer" {
			t.Fatalf("jincheng role = %q, want customer", got)
		}
	})

	t.Run("飞未销售额差额要明确是销项税", func(t *testing.T) {
		res := eng.Query("飞未云科(深圳)技术有限公司2月销售额多少")
		assertTraceBundle(t, res)

		if !res.Success {
			t.Fatalf("expected success, got %q", res.Message)
		}
		mustContainAll(t, res.Message, "账上确认收入 424622.64 元", "销项税")

		if got := numberFromMap(t, res.Data, "amount"); !floatAlmostEqual(got, 424622.64) {
			t.Fatalf("flywei amount = %.2f, want 424622.64", got)
		}
		if got := numberFromMap(t, res.Data, "output_vat"); !floatAlmostEqual(got, 25477.36) {
			t.Fatalf("flywei output_vat = %.2f, want 25477.36", got)
		}
		if got, _ := res.Data["role"].(string); got != "customer" {
			t.Fatalf("flywei role = %q, want customer", got)
		}
	})

	t.Run("林悦和汇智都要走供应商成本路径", func(t *testing.T) {
		linyue := eng.Query("南京林悦智能科技有限公司2月成本多少")
		assertTraceBundle(t, linyue)
		if !linyue.Success {
			t.Fatalf("linyue cost query failed: %s", linyue.Message)
		}
		mustContainAll(t, linyue.Message, "供应商相关", "成本/费用 1943396.23 元")
		mustNotContainAny(t, linyue.Message, "预收")
		if got := numberFromMap(t, linyue.Data, "amount"); !floatAlmostEqual(got, 1943396.23) {
			t.Fatalf("linyue amount = %.2f, want 1943396.23", got)
		}

		huizhi := eng.Query("南京汇智互娱教育科技有限公司2月成本多少")
		assertTraceBundle(t, huizhi)
		if !huizhi.Success {
			t.Fatalf("huizhi cost query failed: %s", huizhi.Message)
		}
		mustContainAll(t, huizhi.Message, "供应商相关", "成本/费用 101415.10 元")
		mustNotContainAny(t, huizhi.Message, "预收")
		if got := numberFromMap(t, huizhi.Data, "amount"); !floatAlmostEqual(got, 101415.10) {
			t.Fatalf("huizhi amount = %.2f, want 101415.10", got)
		}
	})

	t.Run("2月账上净利润仍要可单独查询", func(t *testing.T) {
		res := eng.Query("2026年2月账上净利润是多少")
		assertTraceBundle(t, res)

		if !res.Success {
			t.Fatalf("expected success, got %q", res.Message)
		}
		if !strings.Contains(res.Message, "-2756.97") {
			t.Fatalf("profit message should surface -2756.97, got %q", res.Message)
		}
		if got := numberFromMap(t, res.Data, "account_value"); !floatAlmostEqual(got, -2756.97) {
			t.Fatalf("account_value = %.2f, want -2756.97", got)
		}
		book := mustMap(t, res.Data["财务做账口径(看利润)"], "财务做账口径(看利润)")
		if got := numberFromMap(t, book, "账面利润"); !floatAlmostEqual(got, -2756.97) {
			t.Fatalf("账面利润 = %.2f, want -2756.97", got)
		}
	})
}

func setupReconciliationAnalysisTestDB(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "reconciliation_analysis.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE balance_sheet (
			company TEXT,
			period TEXT,
			account_code TEXT,
			account_name TEXT,
			opening_balance REAL,
			closing_balance REAL
		)`,
		`CREATE TABLE income_statement (
			company TEXT,
			period TEXT,
			item_name TEXT,
			current_amount REAL,
			cumulative_amount REAL
		)`,
		`CREATE TABLE journal (
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
		)`,
		`CREATE TABLE bank_statement (
			company TEXT,
			transaction_date TEXT,
			debit_amount REAL,
			credit_amount REAL,
			counterparty_name TEXT,
			summary TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}

	company := "模拟财务科技有限公司"
	if _, err := db.Exec(`
INSERT INTO income_statement (company, period, item_name, current_amount, cumulative_amount) VALUES
  (?, '2026-02', '一、营业收入', 2485230.69, 2485230.69),
  (?, '2026-02', '减：营业成本', 2066415.10, 2066415.10),
  (?, '2026-02', '管理费用', 420274.77, 420274.77),
  (?, '2026-02', '营业税金及附加', 1159.29, 1159.29),
  (?, '2026-02', '财务费用', 138.69, 138.69),
  (?, '2026-02', '四、净利润（净亏损以\"－\"号填列）', -2756.97, -2756.97)
`, company, company, company, company, company, company); err != nil {
		t.Fatalf("seed income_statement: %v", err)
	}

	bankRows := []struct {
		date    string
		debit   float64
		credit  float64
		counter string
		summary string
	}{
		{"2026-02-05", 0, 2207560.83, "辽宁金程信息科技有限公司", "结算款"},
		{"2026-02-06", 1218855.82, 0, "南京林悦智能科技有限公司", "转账"},
		{"2026-02-12", 0, 208400.00, "飞未云科(深圳)技术有限公司", "结算款"},
		{"2026-02-12", 0, 241700.00, "飞未云科(深圳)技术有限公司", "结算款"},
		{"2026-02-13", 53750.00, 0, "南京汇智互娱教育科技有限公司", "转账"},
		{"2026-02-25", 430000.00, 0, "南京林悦智能科技有限公司", "合同款"},
		{"2026-02-25", 53750.00, 0, "南京汇智互娱教育科技有限公司", "转账"},
	}
	for _, row := range bankRows {
		if _, err := db.Exec(`
INSERT INTO bank_statement (company, transaction_date, debit_amount, credit_amount, counterparty_name, summary)
VALUES (?, ?, ?, ?, ?, ?)
`, company, row.date, row.debit, row.credit, row.counter, row.summary); err != nil {
			t.Fatalf("seed bank row: %v", err)
		}
	}

	journalRows := []struct {
		date      string
		voucherNo string
		code      string
		name      string
		summary   string
		direction string
		amount    float64
		debit     float64
		credit    float64
		counter   string
	}{
		{"2026-02-05", "V001", "112201", "应收账款", "辽宁金程信息科技有限公司转账", "贷", 2207560.83, 0, 2207560.83, "辽宁金程信息科技有限公司"},
		{"2026-02-06", "V002", "112201", "应收账款", "为辽宁金程信息科技有限公司服务", "借", 2027271.59, 2027271.59, 0, "辽宁金程信息科技有限公司"},
		{"2026-02-06", "V002", "600101", "技术服务费", "为辽宁金程信息科技有限公司服务", "贷", 1912520.37, 0, 1912520.37, "辽宁金程信息科技有限公司"},
		{"2026-02-06", "V002", "22210106", "销项税额", "为辽宁金程信息科技有限公司服务", "贷", 114751.22, 0, 114751.22, "辽宁金程信息科技有限公司"},
		{"2026-02-06", "V003", "112201", "应收账款", "为辽宁金程信息科技有限公司服务", "借", 103500.00, 103500.00, 0, "辽宁金程信息科技有限公司"},
		{"2026-02-06", "V003", "600101", "技术服务费", "为辽宁金程信息科技有限公司服务", "贷", 97641.51, 0, 97641.51, "辽宁金程信息科技有限公司"},
		{"2026-02-06", "V003", "22210106", "销项税额", "为辽宁金程信息科技有限公司服务", "贷", 5858.49, 0, 5858.49, "辽宁金程信息科技有限公司"},
		{"2026-02-12", "V004", "600101", "技术服务费", "飞未云科(深圳)技术有限公司转账", "贷", 424622.64, 0, 424622.64, "飞未云科(深圳)技术有限公司"},
		{"2026-02-12", "V004", "22210106", "销项税额", "飞未云科(深圳)技术有限公司转账", "贷", 25477.36, 0, 25477.36, "飞未云科(深圳)技术有限公司"},
		{"2026-02-18", "V005", "600101", "技术服务费", "其他客户收入", "贷", 50446.17, 0, 50446.17, "其他客户"},
		{"2026-02-06", "V006", "220201", "应付账款", "转账南京林悦智能科技有限公司", "借", 1218855.82, 1218855.82, 0, "南京林悦智能科技有限公司"},
		{"2026-02-25", "V007", "112301", "预付账款", "转账南京林悦智能科技有限公司", "借", 430000.00, 430000.00, 0, "南京林悦智能科技有限公司"},
		{"2026-02-28", "V008", "640102", "技术服务费", "收到南京林悦智能科技有限公司发票", "借", 1537735.85, 1537735.85, 0, "南京林悦智能科技有限公司"},
		{"2026-02-28", "V008", "22210101", "进项税额", "收到南京林悦智能科技有限公司发票", "借", 92264.15, 92264.15, 0, "南京林悦智能科技有限公司"},
		{"2026-02-28", "V009", "640102", "技术服务费", "收到南京林悦智能科技有限公司发票", "借", 405660.38, 405660.38, 0, "南京林悦智能科技有限公司"},
		{"2026-02-28", "V009", "22210101", "进项税额", "收到南京林悦智能科技有限公司发票", "借", 24339.62, 24339.62, 0, "南京林悦智能科技有限公司"},
		{"2026-02-28", "V010", "220201", "应付账款", "收到南京林悦智能科技有限公司发票", "贷", 1630000.00, 0, 1630000.00, "南京林悦智能科技有限公司"},
		{"2026-02-28", "V011", "112301", "预付账款", "收到南京林悦智能科技有限公司发票", "贷", 430000.00, 0, 430000.00, "南京林悦智能科技有限公司"},
		{"2026-02-13", "V012", "112301", "预付账款", "转账南京汇智互娱教育科技有限公司", "借", 53750.00, 53750.00, 0, "南京汇智互娱教育科技有限公司"},
		{"2026-02-25", "V013", "112301", "预付账款", "转账南京汇智互娱教育科技有限公司", "借", 53750.00, 53750.00, 0, "南京汇智互娱教育科技有限公司"},
		{"2026-02-28", "V014", "66022304", "服务费", "收到南京汇智互娱教育科技有限公司发票", "借", 50707.55, 50707.55, 0, "南京汇智互娱教育科技有限公司"},
		{"2026-02-28", "V014", "22210101", "进项税额", "收到南京汇智互娱教育科技有限公司发票", "借", 3042.45, 3042.45, 0, "南京汇智互娱教育科技有限公司"},
		{"2026-02-28", "V015", "66022304", "服务费", "收到南京汇智互娱教育科技有限公司发票", "借", 50707.55, 50707.55, 0, "南京汇智互娱教育科技有限公司"},
		{"2026-02-28", "V015", "22210101", "进项税额", "收到南京汇智互娱教育科技有限公司发票", "借", 3042.45, 3042.45, 0, "南京汇智互娱教育科技有限公司"},
		{"2026-02-28", "V016", "112301", "预付账款", "收到南京汇智互娱教育科技有限公司发票", "贷", 53750.00, 0, 53750.00, "南京汇智互娱教育科技有限公司"},
		{"2026-02-28", "V017", "112301", "预付账款", "收到南京汇智互娱教育科技有限公司发票", "贷", 53750.00, 0, 53750.00, "南京汇智互娱教育科技有限公司"},
		{"2026-02-20", "V018", "640101", "营业成本", "其他项目成本", "借", 123018.87, 123018.87, 0, "其他供应商"},
		{"2026-02-21", "V019", "660201", "管理费用", "其他管理费用", "借", 318859.67, 318859.67, 0, "其他"},
		{"2026-02-22", "V020", "6403", "税金及附加", "税金及附加", "借", 1159.29, 1159.29, 0, "税局"},
		{"2026-02-23", "V021", "660301", "财务费用", "财务费用", "借", 138.69, 138.69, 0, "银行"},
		{"2026-02-24", "V022", "6301", "营业外收入", "营业外收入", "贷", 0.19, 0, 0.19, "其他"},
	}
	for _, row := range journalRows {
		if _, err := db.Exec(`
INSERT INTO journal (
	company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty
)
VALUES (?, '2026-02', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, company, row.date, row.voucherNo, row.code, row.name, row.summary, row.direction, row.amount, row.debit, row.credit, row.counter); err != nil {
			t.Fatalf("seed journal row: %v", err)
		}
	}

	return dbPath
}

func assertTraceBundle(t *testing.T, res query.Result) {
	t.Helper()

	if len(res.ExecutedSQL) == 0 {
		t.Fatalf("expected executed_sql trace, got none")
	}
	if len(res.CalculationLogs) == 0 {
		t.Fatalf("expected calculation_logs trace, got none")
	}

	trace, ok := res.Data["trace"].(map[string]any)
	if !ok {
		t.Fatalf("expected data.trace map, got %T", res.Data["trace"])
	}
	process, ok := res.Data["process"].(map[string]any)
	if !ok {
		t.Fatalf("expected data.process map, got %T", res.Data["process"])
	}

	for label, bundle := range map[string]map[string]any{"trace": trace, "process": process} {
		executed, ok := bundle["executed_sql"].([]string)
		if !ok || len(executed) == 0 {
			t.Fatalf("expected %s.executed_sql slice, got %T %#v", label, bundle["executed_sql"], bundle["executed_sql"])
		}
		logs, ok := bundle["calculation_logs"].([]string)
		if !ok || len(logs) == 0 {
			t.Fatalf("expected %s.calculation_logs slice, got %T %#v", label, bundle["calculation_logs"], bundle["calculation_logs"])
		}
	}

	if _, ok := res.Data["executed_sql"].([]string); !ok {
		t.Fatalf("expected data.executed_sql slice, got %T", res.Data["executed_sql"])
	}
	if _, ok := res.Data["calculation_logs"].([]string); !ok {
		t.Fatalf("expected data.calculation_logs slice, got %T", res.Data["calculation_logs"])
	}
}

func mustContainAll(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("message %q should contain %q", got, want)
		}
	}
}

func mustNotContainAny(t *testing.T, got string, rejects ...string) {
	t.Helper()
	for _, reject := range rejects {
		if strings.Contains(got, reject) {
			t.Fatalf("message %q should not contain %q", got, reject)
		}
	}
}

func mustSliceOfMaps(t *testing.T, v any, label string) []map[string]any {
	t.Helper()
	items, ok := v.([]map[string]any)
	if ok {
		return items
	}
	raw, ok := v.([]any)
	if !ok {
		t.Fatalf("%s should be a slice, got %T", label, v)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s item should be map, got %T", label, item)
		}
		out = append(out, m)
	}
	return out
}

func findHighlight(t *testing.T, items []map[string]any, name string) map[string]any {
	t.Helper()
	for _, item := range items {
		if asString(item["name"]) == name {
			return item
		}
	}
	t.Fatalf("highlight %q not found in %#v", name, items)
	return nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mustMap(t *testing.T, v any, label string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s should be a map, got %T", label, v)
	}
	return m
}

func floatAlmostEqual(got, want float64) bool {
	return math.Abs(got-want) <= 0.01
}
