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
	return containsAny(q, contractPriorityKeywords())
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
