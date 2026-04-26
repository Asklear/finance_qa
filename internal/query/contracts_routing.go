package query

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

func shouldUseContractDimension(question string) bool {
	q := strings.TrimSpace(question)
	if !strings.Contains(q, "合同") {
		return false
	}
	if shouldUseCompanyScopeContractAggregate(q) {
		return false
	}
	return containsAny(q, contractPriorityKeywords())
}

func shouldUseContractDetailQuestion(question string) bool {
	q := strings.TrimSpace(question)
	if !containsAny(q, []string{"合同", "协议", "发票"}) {
		return false
	}
	if shouldUseCompanyScopeContractAggregate(q) {
		return false
	}
	detailKeywords := []string{
		"条款", "细节", "正文", "原文", "具体内容", "合同内容", "内容是什么",
		"付款条款", "付款方式", "结算周期", "结算方式", "服务范围", "服务内容",
		"交付", "验收", "保密", "违约", "税率", "签署", "签约", "起止", "到期",
		"续约", "第几页", "哪一页", "发票金额", "发票明细", "发票号码", "发票号",
		"开票日期", "票面金额", "不含税", "含税", "税额", "购买方", "销售方",
	}
	if !containsAny(q, detailKeywords) {
		return false
	}
	operatingKeywords := []string{"收入", "营收", "成本", "利润", "回款", "到账", "付款", "应收", "应付", "未回款", "未付款", "已开票", "未支付"}
	if containsAny(q, operatingKeywords) && !containsAny(q, detailKeywords) {
		return false
	}
	return true
}

func shouldUseCompanyScopeContractAggregate(question string) bool {
	q := strings.TrimSpace(question)
	if !containsAny(q, []string{"合同", "项目"}) {
		return false
	}
	entity := extractNamedEntityFromQuestion(q)
	if looksLikeBossRewriteNonEntity(entity) {
		entity = ""
	}
	if isRealishQueryEntity(entity) {
		return false
	}
	if shouldUseExplicitFinancialAccountQuestion(q) {
		return false
	}
	metric := detectBossMetric(q)
	if isBossContractFirstMetric(metric) {
		return true
	}
	return containsAny(q, []string{
		"结算", "执行", "情况", "多少", "有哪些", "哪些", "明细", "列表", "分别", "汇总", "统计",
	})
}

func contractPriorityKeywords() []string {
	return getRuleConfig().ContractPriorityKeywords()
}

func isContractPriorityQuestion(question string) bool {
	q := strings.TrimSpace(question)
	return containsAny(q, contractPriorityKeywords())
}

func extractContractBaseQuestion(question string) string {
	q := strings.TrimSpace(question)
	if idx := strings.Index(q, "其中"); idx >= 0 {
		q = strings.TrimSpace(q[:idx])
	}
	return strings.TrimSpace(strings.TrimRight(q, "，,。；;？?"))
}

func extractContractQuestionPeriods(question string, anchor time.Time) (string, string) {
	baseQuestion := extractContractBaseQuestion(question)
	if year, ok := extractExplicitStandaloneYear(baseQuestion); ok {
		return fmt.Sprintf("%04d-01", year), fmt.Sprintf("%04d-12", year)
	}
	return ExtractPeriodWithNow(baseQuestion, anchor)
}

func extractExplicitStandaloneYear(question string) (int, bool) {
	q := strings.TrimSpace(question)
	if q == "" {
		return 0, false
	}
	if strings.Contains(q, "今年") || strings.Contains(q, "本年") {
		return 0, false
	}
	specificPeriodPatterns := []*regexp.Regexp{
		regexp.MustCompile(`20\d{2}年\s*(?:上半年|下半年|全年|整年|全年度|年度|累计|年内)`),
		regexp.MustCompile(`20\d{2}年\s*(?:第?\s*[一二三四1234]\s*季度|Q\s*[1-4])`),
		regexp.MustCompile(`20\d{2}年\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月`),
		regexp.MustCompile(`20\d{2}年\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月?\s*(?:到|至|-|~)`),
	}
	for _, pattern := range specificPeriodPatterns {
		if pattern.MatchString(q) {
			return 0, false
		}
	}
	m := regexp.MustCompile(`(20\d{2})年`).FindStringSubmatch(q)
	if len(m) != 2 {
		return 0, false
	}
	return mustAtoi(m[1]), true
}
