package query

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"financeqa/internal/config"
)

type Intent string

const (
	IntentPrecise        Intent = "precise"
	IntentMonthlySummary Intent = "monthly_summary"
	IntentTaxQuery       Intent = "tax_query"
	IntentARAPQuery      Intent = "ar_ap_query"
	IntentAnalysis       Intent = "analysis"
	IntentFallback       Intent = "fallback"
)

var (
	fullMonthPattern = regexp.MustCompile(`(\d{4})[年\.](\d{1,2})月?`)
	monthOnlyPattern = regexp.MustCompile(`(^|[^\d])(\d{1,2})月([^\d]|$)`)
	rangePattern     = regexp.MustCompile(`(\d{4})[年\.](\d{1,2})月?[\s\-~到至](\d{4})[年\.](\d{1,2})月?`)
)

func ExtractPeriodWithNow(question string, now time.Time) (string, string) {
	q := strings.TrimSpace(question)
	currentYear := now.Year()
	currentMonth := int(now.Month())

	if m := rangePattern.FindStringSubmatch(q); len(m) == 5 {
		return normalizeYearMonth(m[1], m[2]), normalizeYearMonth(m[3], m[4])
	}

	if m := fullMonthPattern.FindStringSubmatch(q); len(m) == 3 {
		p := normalizeYearMonth(m[1], m[2])
		return p, p
	}

	if m := monthOnlyPattern.FindStringSubmatch(q); len(m) == 4 {
		month, _ := strconv.Atoi(m[2])
		year := currentYear
		if month > currentMonth {
			year--
		}
		p := fmt.Sprintf("%d-%02d", year, month)
		return p, p
	}

	p := fmt.Sprintf("%d-%02d", currentYear, currentMonth)
	return p, p
}

func ResolveCompany(requested string, available []string) string {
	req := strings.TrimSpace(requested)
	if len(available) == 0 {
		return req
	}

	companies := append([]string(nil), available...)
	sort.Slice(companies, func(i, j int) bool {
		return len([]rune(companies[i])) > len([]rune(companies[j]))
	})

	best := ""
	for _, c := range companies {
		if req == "" {
			if best == "" {
				best = c
			}
			continue
		}
		if strings.Contains(req, c) || strings.Contains(c, req) {
			if len([]rune(c)) > len([]rune(best)) {
				best = c
			}
		}
	}
	if best != "" {
		return best
	}
	return companies[0]
}

func ClassifyIntent(question string) Intent {
	q := strings.ToLower(strings.TrimSpace(question))
	mgr := config.GetKeywordsManager()

	if containsAny(q, []string{"账龄", "周转", "流动比率", "速动比率", "分析"}) ||
		mgr.CheckKeywordsInText(q, "intents.analysis.primary_keywords") {
		return IntentAnalysis
	}
	if containsAny(q, []string{"税", "增值税", "进项", "销项"}) ||
		mgr.CheckKeywordsInText(q, "intents.tax_query.keywords") {
		return IntentTaxQuery
	}
	if containsAny(q, []string{"应收", "应付", "欠款", "未收", "未付"}) ||
		mgr.CheckKeywordsInText(q, "intents.ar_ap_query.keywords") {
		return IntentARAPQuery
	}
	if containsAny(q, []string{"收入", "支出", "利润", "收支汇总"}) ||
		mgr.CheckKeywordsInText(q, "intents.monthly_summary.keywords") ||
		mgr.HasMonthlySummarySpecialKeyword(q) {
		return IntentMonthlySummary
	}
	if matchesAnyPattern(q, toStringSlice(mgr.Get("intents.entity_count.patterns", []any{}))) {
		return IntentFallback
	}
	if mgr.CheckKeywordsInText(q, "intents.customer_query.primary_keywords") ||
		containsAny(q, []string{"供应商", "客户", "项目", "健康度", "有数据", "出来了吗"}) {
		return IntentFallback
	}
	return IntentPrecise
}

func matchesAnyPattern(text string, patterns []string) bool {
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func toStringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func containsAny(s string, parts []string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func normalizeYearMonth(year, month string) string {
	m, _ := strconv.Atoi(month)
	return fmt.Sprintf("%s-%02d", year, m)
}
