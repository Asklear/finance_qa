package query

import "strings"

func normalizeEntityText(s string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "（", "", "）", "", "(", "", ")", "", "-", "", "_", "", ",", "", "，", "", ".", "", "。", "")
	return replacer.Replace(strings.TrimSpace(s))
}

func stripTemporalNoise(entity string) string {
	return strings.TrimSpace(temporalNoisePattern.ReplaceAllString(entity, ""))
}

func trimEntityNoiseSuffixes(entity string) string {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return ""
	}
	suffixes := append([]string{
		"报销了", "报销", "报账", "到账", "回款", "收款", "付款",
		"费用", "支出", "收入", "成本", "利润", "明细", "金额",
		"产生了", "产生", "多少", "报",
	}, getRuleConfig().GenericMetricStopwords...)
	for {
		trimmed := entity
		for _, suffix := range suffixes {
			suffix = strings.TrimSpace(suffix)
			if suffix == "" {
				continue
			}
			if strings.HasSuffix(trimmed, suffix) {
				trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, suffix))
			}
		}
		if trimmed == entity {
			return trimmed
		}
		entity = trimmed
	}
}

func shouldSkipEntityFragment(fragment string, minLen int) bool {
	if len([]rune(fragment)) < minLen {
		return true
	}
	if isGenericMetricEntity(fragment) || looksLikeTemporalMetricEntity(fragment) {
		return true
	}
	return containsAny(fragment, []string{
		"帮我", "一下", "查询", "多少", "哪些", "价格", "一共", "支出", "报销",
		"经营", "分析", "风险", "健康", "评价", "应收", "应付", "账款", "费用",
		"资金", "货币", "流水", "工资", "社保", "公积金", "人力成本", "薪酬",
		"营收", "收入", "成本", "利润", "季度", "半年", "全年", "年度", "累计", "年内",
	})
}
