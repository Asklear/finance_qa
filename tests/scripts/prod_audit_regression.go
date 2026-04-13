package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type AuditQuestion struct {
	ID         int
	Question   string
	Validators []validator
}

type QueryResult struct {
	Success         bool           `json:"success"`
	Message         string         `json:"message"`
	Data            map[string]any `json:"data"`
	ExecutedSQL     []string       `json:"executed_sql"`
	CalculationLogs []string       `json:"calculation_logs"`
}

type validator func(res QueryResult, db *sql.DB) []string

func main() {
	company := "南京优集数据科技有限公司"
	dbPath := mustLocateFinanceDB()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(fmt.Sprintf("open finance.db failed: %v", err))
	}
	defer db.Close()

	questions := []AuditQuestion{
		{1, "2026年1月收入/成本多少", []validator{mustSuccess, noMetricEntityTrap, mustExposeTrace, mustContainRevenueAndCost}},
		{2, "2026年2月收入/成本/利润分别是多少", []validator{mustSuccess, noMetricEntityTrap, mustExposeTrace, mustContainRevenueCostProfit}},
		{3, "2026年2月整体支出多少", []validator{mustSuccess, mustExposeTrace}},
		{4, "1月人力成本（应付职工薪酬）", []validator{mustSuccess, mustExposeTrace}},
		{5, "供应商有多少个？", []validator{mustSuccess, mustExposeTrace, mustListSuppliers}},
		{6, "南京林悦智能科技有限公司数据出来了吗", []validator{mustSuccess, mustExposeTrace}},
		{7, "梁梦瑶报销了多少钱", []validator{mustSuccess, mustExposeTrace}},
		{8, "飞未云科(深圳)技术有限公司支付的成本是多少", []validator{mustSuccess, mustExposeTrace}},
		{9, "2026年2月销项税额是多少", []validator{mustSuccess, mustExposeTrace}},
		{10, "2026年2月进项税额是多少", []validator{mustSuccess, mustExposeTrace}},
		{11, "2026年2月总成本", []validator{mustSuccess, mustExposeTrace}},
		{12, "资产负债表：2026年2月货币资金余额", []validator{mustSuccess, mustExposeTrace}},
		{13, "当前的应收账款汇总", []validator{mustSuccess, mustExposeTrace}},
		{14, "南京市中闻（南京）律师事务所的付款记录", []validator{mustSuccess, mustExposeTrace}},
		{15, "公司经营状况深度评估", []validator{mustExposeTrace}},
		{16, "辽宁金程信息科技有限公司2月销售额多少", []validator{mustSuccess, mustExposeTrace, mustDifferentiateSettlementVsRecognition}},
		{17, "南京林悦智能科技有限公司2月成本多少", []validator{mustSuccess, mustExposeTrace, mustTreatSupplierAsCost}},
	}

	fmt.Println("# 🚀 南京优集生产数据：全量回归审计报告 (Strict)")
	fmt.Printf("> 生成时间: %s | 锚定账期: 2026-02 | 数据库: %s\n\n", time.Now().Format("2006-01-02 15:04:05"), dbPath)
	fmt.Println("| ID | 审计提问 | 状态 | 关键原因 | 耗时 |")
	fmt.Println("|:---|:---|:---:|:---|---:|")

	failCount := 0
	for _, aq := range questions {
		start := time.Now()
		res, parseErr, raw := runQuery(company, aq.Question)
		dur := time.Since(start)

		reasons := make([]string, 0)
		if parseErr != nil {
			reasons = append(reasons, fmt.Sprintf("JSON解析失败: %v", parseErr))
			reasons = append(reasons, truncate(raw, 220))
		} else {
			for _, v := range aq.Validators {
				reasons = append(reasons, v(res, db)...)
			}
		}

		status := "✅ PASS"
		reasonText := "通过"
		if len(reasons) > 0 {
			status = "❌ FAIL"
			reasonText = strings.Join(uniqueNonEmpty(reasons), "；")
			failCount++
		}
		fmt.Printf("| %d | %s | %s | %s | %dms |\n", aq.ID, aq.Question, status, sanitizeCell(reasonText), dur.Milliseconds())
	}

	fmt.Println()
	if failCount > 0 {
		fmt.Printf("## 结论: ❌ %d 个问题未通过（严格语义断言）。\n", failCount)
		os.Exit(1)
	}
	fmt.Println("## 结论: ✅ 全部通过（严格语义断言）。")
}

func runQuery(company, question string) (QueryResult, error, string) {
	cmd := exec.Command("go", "run", "cmd/financeqa/main.go", "query", "--company", company, question)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()

	raw := strings.TrimSpace(out.String())
	var res QueryResult
	err := json.Unmarshal([]byte(raw), &res)
	return res, err, raw
}

func mustLocateFinanceDB() string {
	candidates := []string{"finance.db", filepath.Join("..", "..", "finance.db")}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	panic("finance.db not found. please run from repo root or tests/scripts")
}

func mustSuccess(res QueryResult, _ *sql.DB) []string {
	if res.Success {
		return nil
	}
	return []string{"success=false: " + truncate(res.Message, 160)}
}

func mustExposeTrace(res QueryResult, _ *sql.DB) []string {
	var reasons []string
	if len(res.ExecutedSQL) == 0 {
		reasons = append(reasons, "缺少 executed_sql")
	}
	if len(res.CalculationLogs) == 0 {
		reasons = append(reasons, "缺少 calculation_logs")
	}
	if dataSQL := sliceLen(res.Data, "executed_sql"); dataSQL == 0 {
		reasons = append(reasons, "data.executed_sql 为空")
	}
	if dataLogs := sliceLen(res.Data, "calculation_logs"); dataLogs == 0 {
		reasons = append(reasons, "data.calculation_logs 为空")
	}
	return reasons
}

func noMetricEntityTrap(res QueryResult, _ *sql.DB) []string {
	badEntities := map[string]struct{}{"收入": {}, "成本": {}, "利润": {}, "支出": {}, "销售额": {}}
	entity, _ := res.Data["entity"].(string)
	if _, bad := badEntities[entity]; bad {
		return []string{fmt.Sprintf("误走实体路径: entity=%s", entity)}
	}
	if strings.Contains(res.Message, "[收入]") || strings.Contains(res.Message, "[成本]") {
		return []string{"误走实体路径: message 含 [收入]/[成本]"}
	}
	for _, log := range res.CalculationLogs {
		if strings.Contains(log, "entity=收入") || strings.Contains(log, "entity=成本") || strings.Contains(log, "entity=利润") {
			return []string{"误走实体路径: calculation_logs 含 entity=收入/成本/利润"}
		}
	}
	return nil
}

func mustContainRevenueAndCost(res QueryResult, db *sql.DB) []string {
	expectedRevenue, expectedCost := queryRevenueAndCost(db, "2026-01")
	book, ok := asMap(res.Data["财务做账口径(看利润)"])
	if !ok {
		return []string{"缺少 财务做账口径(看利润)"}
	}

	reasons := make([]string, 0)
	rev, revOK := findFloat(book, []string{"营业收入", "收入"})
	cost, costOK := findFloat(book, []string{"营业成本及费用", "总成本", "成本"})
	if !revOK {
		reasons = append(reasons, "未返回营业收入")
	}
	if !costOK {
		reasons = append(reasons, "未返回成本")
	}
	if revOK && expectedRevenue > 0 && !approxEqual(rev, expectedRevenue) {
		reasons = append(reasons, fmt.Sprintf("营业收入不匹配: got=%.2f want=%.2f", rev, expectedRevenue))
	}
	if costOK && expectedCost > 0 && !approxEqual(cost, expectedCost) {
		reasons = append(reasons, fmt.Sprintf("成本不匹配: got=%.2f want=%.2f", cost, expectedCost))
	}
	return reasons
}

func mustContainRevenueCostProfit(res QueryResult, db *sql.DB) []string {
	expectedRevenue, expectedCost := queryRevenueAndCost(db, "2026-02")
	expectedProfit := queryNetProfit(db, "2026-02")
	book, ok := asMap(res.Data["财务做账口径(看利润)"])
	if !ok {
		return []string{"缺少 财务做账口径(看利润)"}
	}

	reasons := make([]string, 0)
	rev, revOK := findFloat(book, []string{"营业收入", "收入"})
	cost, costOK := findFloat(book, []string{"营业成本及费用", "总成本", "成本"})
	profit, profitOK := findFloat(book, []string{"账面利润", "利润", "净利润"})
	if !revOK || !costOK || !profitOK {
		return append(reasons, "未完整返回收入/成本/利润")
	}
	if expectedRevenue > 0 && !approxEqual(rev, expectedRevenue) {
		reasons = append(reasons, fmt.Sprintf("收入不匹配: got=%.2f want=%.2f", rev, expectedRevenue))
	}
	if expectedCost > 0 && !approxEqual(cost, expectedCost) {
		reasons = append(reasons, fmt.Sprintf("成本不匹配: got=%.2f want=%.2f", cost, expectedCost))
	}
	if expectedProfit != 0 && !approxEqual(profit, expectedProfit) {
		reasons = append(reasons, fmt.Sprintf("利润不匹配: got=%.2f want=%.2f", profit, expectedProfit))
	}
	return reasons
}

func mustListSuppliers(res QueryResult, _ *sql.DB) []string {
	if strings.Contains(res.Message, "供应商") {
		return nil
	}
	if _, ok := res.Data["names"]; ok {
		return nil
	}
	return []string{"供应商查询未返回具体供应商名称或说明"}
}

func mustDifferentiateSettlementVsRecognition(res QueryResult, _ *sql.DB) []string {
	reasons := make([]string, 0)
	if !strings.Contains(res.Message, "账上确认收入") {
		reasons = append(reasons, "缺少“账上确认收入”描述")
	}
	if !strings.Contains(res.Message, "历史应收回款") {
		reasons = append(reasons, "缺少“历史应收回款”描述")
	}
	if strings.Contains(res.Message, "总收入") {
		reasons = append(reasons, "疑似回退到公司总收入口径")
	}
	return reasons
}

func mustTreatSupplierAsCost(res QueryResult, _ *sql.DB) []string {
	reasons := make([]string, 0)
	if role, _ := res.Data["role"].(string); role != "supplier" {
		reasons = append(reasons, fmt.Sprintf("角色识别错误: role=%q (want supplier)", role))
	}
	if !strings.Contains(res.Message, "供应商") {
		reasons = append(reasons, "缺少供应商口径说明")
	}
	if !strings.Contains(res.Message, "成本") && !strings.Contains(res.Message, "费用") {
		reasons = append(reasons, "未明确归入成本/费用")
	}
	if strings.Contains(res.Message, "未确认收入") || strings.Contains(res.Message, "预收") {
		reasons = append(reasons, "错误归因为未确认收入/预收")
	}
	return reasons
}

func queryRevenueAndCost(db *sql.DB, period string) (float64, float64) {
	var revenue float64
	_ = db.QueryRow(`SELECT COALESCE(SUM(current_amount),0) FROM income_statement
WHERE company LIKE '%南京优集%' AND period = ? AND item_name LIKE '%营业收入%'`, period).Scan(&revenue)

	var cost float64
	_ = db.QueryRow(`SELECT COALESCE(SUM(current_amount),0) FROM income_statement
WHERE company LIKE '%南京优集%' AND period = ? AND (
	item_name LIKE '%营业成本%' OR
	item_name LIKE '%税金及附加%' OR
	item_name LIKE '%销售费用%' OR
	item_name LIKE '%管理费用%' OR
	item_name LIKE '%财务费用%'
)`, period).Scan(&cost)
	return round2(revenue), round2(cost)
}

func queryNetProfit(db *sql.DB, period string) float64 {
	var profit float64
	_ = db.QueryRow(`SELECT COALESCE(SUM(current_amount),0) FROM income_statement
WHERE company LIKE '%南京优集%' AND period = ? AND item_name LIKE '%净利润%'`, period).Scan(&profit)
	return round2(profit)
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func findFloat(m map[string]any, keys []string) (float64, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if f, ok := toFloat(v); ok {
				return f, true
			}
		}
	}
	return 0, false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func sliceLen(data map[string]any, key string) int {
	if data == nil {
		return 0
	}
	v, ok := data[key]
	if !ok || v == nil {
		return 0
	}
	s := anySlice(v)
	return len(s)
}

func anySlice(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []string:
		out := make([]any, 0, len(x))
		for _, s := range x {
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func approxEqual(a, b float64) bool {
	if a == b {
		return true
	}
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 0.01
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func uniqueNonEmpty(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func sanitizeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return truncate(s, 220)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
