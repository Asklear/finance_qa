package query

import (
	"fmt"
	"strings"
)

// buildFallbackHint 构建一个供宿主 LLM 阅读的结构化提示响应。
// 当规则引擎无法识别用户意图时，返回足够的上下文信息，
// 让 OpenClaw / Claude Code 等宿主 LLM 自行修正查询后重试。
func (e *Engine) buildFallbackHint(question, from, to, priorErr string) Result {
	// 收集可用的科目名称，帮助宿主 LLM 精准匹配
	accounts := e.listAvailableAccounts(to)

	// 收集可用的交易对手方名称
	counterparties := e.listCounterparties(from, to)

	supportedIntents := []string{
		"monthly_summary — 月度收支/收入/支出/利润汇总（例：'2026年2月收入多少'）",
		"tax — 增值税进项/销项查询（例：'今年的增值税是多少'）",
		"ar_ap — 应收/应付账款余额（例：'应收账款余额多少'）",
		"analysis — 财务健康度/账龄分析（例：'公司财务健康度怎么样'）",
		"entity_count — 供应商/客户/项目数量统计（例：'有多少供应商'）",
		"counterparty_amount — 指定交易对手的收付款金额（例：'某某客户今年的销售额'）",
		"account_balance — 指定科目余额查询（例：'应付职工薪酬余额'）",
	}

	hint := fmt.Sprintf(
		"原始提问未能匹配任何已知查询路径。请根据以下信息改写查询后重新调用 financeqa：\n"+
			"1. 请在查询中包含明确的时间（如'2026年2月'而非'这个月'）\n"+
			"2. 使用下方的科目名或交易对手名精确指代实体\n"+
			"3. 用更具体的财务术语（收入/支出/利润/应收/增值税等）",
	)

	data := map[string]any{
		"fallback_attempted":  true,
		"original_question":   question,
		"period_from":         from,
		"period_to":           to,
		"hint":                hint,
		"supported_intents":   supportedIntents,
		"available_accounts":  accounts,
		"counterparty_sample": counterparties,
	}

	msg := "查询未匹配，请参考 hint 改写后重试"
	if strings.TrimSpace(priorErr) != "" {
		msg = fmt.Sprintf("%s（先前错误：%s）", msg, priorErr)
	}

	return Result{
		Success: false,
		Message: msg,
		Data:    data,
	}
}

// listAvailableAccounts 返回当前期间可查询的科目名，最多 30 条
func (e *Engine) listAvailableAccounts(period string) []string {
	rows, err := e.db.Query(`
SELECT DISTINCT account_name
FROM balance_sheet
WHERE company = ?
  AND period = ?
ORDER BY account_name
LIMIT 30
`, e.Company, period)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil && strings.TrimSpace(name) != "" {
			out = append(out, name)
		}
	}
	return out
}

// listCounterparties 返回当前期间有交易记录的交易对手方名称，最多 20 条
func (e *Engine) listCounterparties(from, to string) []string {
	rows, err := e.db.Query(`
SELECT DISTINCT counterparty_name
FROM bank_statement
WHERE company = ?
  AND DATE(transaction_date) >= DATE(?)
  AND DATE(transaction_date) <= DATE(?)
  AND counterparty_name IS NOT NULL
  AND counterparty_name <> ''
ORDER BY counterparty_name
LIMIT 20
`, e.Company, from+"-01", monthEndDay(to))
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil && strings.TrimSpace(name) != "" {
			out = append(out, name)
		}
	}
	return out
}
