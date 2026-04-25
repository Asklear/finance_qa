package query

import (
	"regexp"
	"strings"
)

func extractNamedEntityFromQuestion(question string) string {
	q := strings.TrimSpace(question)
	if q == "" {
		return ""
	}
	candidate := q
	replacements := []string{
		"今年", "本年", "累计", "年内", "其中", "到账", "回款", "收款", "付款", "支付", "收入", "营收", "销售额", "GMV", "gmv",
		"成本", "利润", "应收账款", "应付账款", "应收", "应付", "数据出来了吗", "数据出来了没", "数据出来", "多少", "是什么", "分别",
		"合同", "项目", "情况", "余额", "明细", "数据", "账上", "财务账", "会计账", "科目余额", "余额表", "资产负债表", "报表口径", "期初", "期末",
		"3月", "2月", "1月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月",
		"第一季度", "第二季度", "第三季度", "第四季度", "季度", "上半年", "下半年", "全年", "全年度", "整年", "年度", "Q1", "Q2", "Q3", "Q4", "q1", "q2", "q3", "q4",
	}
	for _, token := range replacements {
		candidate = strings.ReplaceAll(candidate, token, " ")
	}
	candidate = regexp.MustCompile(`20\d{2}年`).ReplaceAllString(candidate, " ")
	candidate = regexp.MustCompile(`[\?\?,，。；;！!（）()\s]+`).ReplaceAllString(candidate, " ")
	parts := regexp.MustCompile(`[\x{4e00}-\x{9fa5}A-Za-z0-9()（）]+`).FindAllString(candidate, -1)
	for _, part := range parts {
		part = trimEntityNoiseSuffixes(stripTemporalNoise(strings.TrimSpace(part)))
		if !isRealishQueryEntity(part) || looksLikeAccountFragment(part) {
			continue
		}
		return part
	}
	if m := namedEntityPattern.FindStringSubmatch(q); len(m) == 2 {
		entity := trimEntityNoiseSuffixes(stripTemporalNoise(strings.TrimSpace(m[1])))
		if len([]rune(entity)) >= 2 && !isGenericMetricEntity(entity) && !looksLikeAccountFragment(entity) && !looksLikeSyntheticQuestionFragment(entity) {
			return entity
		}
	}
	return ""
}

func isRealishQueryEntity(entity string) bool {
	trimmed := strings.TrimSpace(entity)
	return len([]rune(trimmed)) >= 2 &&
		!isGenericMetricEntity(trimmed) &&
		!looksLikeTemporalMetricEntity(trimmed) &&
		!looksLikeSyntheticQuestionFragment(trimmed)
}

func looksLikeAccountFragment(entity string) bool {
	return containsAny(entity, []string{"应收", "应付", "账款", "余额", "情况", "明细", "数据"})
}

func looksLikeSyntheticQuestionFragment(entity string) bool {
	normalized := normalizeEntityText(entity)
	if normalized == "" {
		return false
	}
	questionFragments := []string{
		"单笔", "最大", "最小", "来自谁", "是谁", "多少", "金额", "是多少", "什么",
		"流入", "流出", "到账", "回款", "收款", "付款",
		"税额", "销项", "进项", "净税额",
	}
	matchCount := 0
	for _, fragment := range questionFragments {
		if strings.Contains(normalized, normalizeEntityText(fragment)) {
			matchCount++
		}
	}
	return matchCount >= 1
}
