package query

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Intent 枚举定义所有审计行为意图
type Intent string

const (
	IntentGeneral               Intent = "general"
	IntentAmount                Intent = "amount"
	IntentIdentityQuery         Intent = "identity"
	IntentMonthlySummary        Intent = "monthly_summary"
	IntentPrecise               Intent = "precise"
	IntentTaxQuery              Intent = "tax"
	IntentARAPQuery             Intent = "arap"
	IntentLargeTransactionQuery Intent = "large_transaction"
	IntentAnalysis              Intent = "analysis"
	IntentFallback              Intent = "fallback"
)

// ResolveCompany 智能匹配公司名
func ResolveCompany(req string, companies []string) string {
	if len(companies) == 0 { return req }
	q := strings.TrimSpace(req)
	var best string
	for _, c := range companies {
		if c == q { return c }
		if (len(q) >= 6 && strings.Contains(c, q)) || (len(c) >= 6 && strings.Contains(q, c)) {
			if len(c) > len(best) { best = c }
		}
	}
	if best != "" { return best }
	return companies[0]
}

// ExtractPeriodWithNow 从自然语言提取账期
func ExtractPeriodWithNow(question string, anchor time.Time) (string, string) {
	yearMatch := regexp.MustCompile(`(20\d{2})`).FindStringSubmatch(question)
	monthMatch := regexp.MustCompile(`(\d{1,2})月`).FindStringSubmatch(question)
	year := strconv.Itoa(anchor.Year())
	if len(yearMatch) > 0 { year = yearMatch[1] }
	month := fmt.Sprintf("%02d", int(anchor.Month()))
	if len(monthMatch) > 0 {
		m, _ := strconv.Atoi(monthMatch[1])
		month = fmt.Sprintf("%02d", m)
	}
	period := fmt.Sprintf("%s-%s", year, month)
	return period, period
}

// ClassifyIntent 精准意图识别引擎 V6 (加权版)
func ClassifyIntent(question string) Intent {
	q := strings.ReplaceAll(question, " ", "")
	
	if containsAny(q, []string{"分析", "评分", "健康", "评价", "风险", "怎么样", "分析下"}) {
		return IntentAnalysis
	}

	if strings.Contains(q, "税") { return IntentTaxQuery }

	if containsAny(q, []string{"期末", "余额", "是多少", "查询余额", "还有多少"}) {
		return IntentGeneral
	}

	if containsAny(q, []string{"是谁", "身份", "干嘛的", "哪里的", "谁是"}) {
		return IntentIdentityQuery
	}

	if containsAny(q, []string{"概括", "总结", "利润", "指标", "经营状况", "收入", "支出汇总", "报销汇总", "成本", "总成本", "费用总额"}) {
		return IntentMonthlySummary
	}

	return IntentGeneral
}

func NormalizeQuestion(q string) string {
	q = strings.ReplaceAll(q, "？", "?")
	q = strings.ReplaceAll(q, "，", ",")
	// 激进清理：移除所有空格，确保实体提取器不会因为空格干扰而失效
	q = strings.ReplaceAll(q, " ", "")
	return strings.TrimSpace(q)
}

func containsAny(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) { return true }
	}
	return false
}
