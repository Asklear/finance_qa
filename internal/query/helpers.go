package query

import (
	"fmt"
	"regexp"
	"sort"
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
	IntentHostPayload           Intent = "host_payload"
	IntentPrecise               Intent = "precise"
	IntentTaxQuery              Intent = "tax"
	IntentARAPQuery             Intent = "arap"
	IntentLargeTransactionQuery Intent = "large_transaction"
	IntentAnalysis              Intent = "analysis"
	IntentFallback              Intent = "fallback"
)

// ResolveCompany 智能匹配公司名
func ResolveCompany(req string, companies []string) string {
	if len(companies) == 0 {
		return req
	}
	q := strings.TrimSpace(req)
	if q == "" {
		return companies[0]
	}
	// 若用户输入的是简称，优先提升到包含该简称的最长正式公司名
	for _, c := range companies {
		if normalizeEntityText(c) == normalizeEntityText(q) {
			best := c
			for _, other := range companies {
				if other != c && strings.Contains(normalizeEntityText(other), normalizeEntityText(q)) && len([]rune(other)) > len([]rune(best)) {
					best = other
				}
			}
			return best
		}
	}

	best := companies[0]
	bestScore := -1
	for _, c := range companies {
		score := companyMatchScore(q, c)
		if score > bestScore || (score == bestScore && len(c) > len(best)) {
			best = c
			bestScore = score
		}
	}
	return best
}

// ExtractPeriodWithNow 从自然语言提取账期
func ExtractPeriodWithNow(question string, anchor time.Time) (string, string) {
	year := anchor.Year()
	anchorMonth := int(anchor.Month())
	q := strings.TrimSpace(question)

	type ym struct {
		year  int
		month int
	}

	// 1) 显式范围: 2026年1月到2026年2月
	rangeRe := regexp.MustCompile(`(20\d{2})年\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月?\s*(?:到|至|-|~)\s*(20\d{2})年\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月`)
	if m := rangeRe.FindStringSubmatch(q); len(m) == 5 {
		y1, _ := strconv.Atoi(m[1])
		y2, _ := strconv.Atoi(m[3])
		m1 := parseChineseOrDigitMonth(m[2])
		m2 := parseChineseOrDigitMonth(m[4])
		if validMonth(m1) && validMonth(m2) {
			return fmt.Sprintf("%04d-%02d", y1, m1), fmt.Sprintf("%04d-%02d", y2, m2)
		}
	}

	// 2) 明确的年月出现（可包含中文月份）
	ymRe := regexp.MustCompile(`(20\d{2})年\s*([0-1]?\d|[一二三四五六七八九十两]{1,3})月`)
	yms := ymRe.FindAllStringSubmatch(q, -1)
	if len(yms) >= 2 {
		first := ym{year: mustAtoi(yms[0][1]), month: parseChineseOrDigitMonth(yms[0][2])}
		last := ym{year: mustAtoi(yms[len(yms)-1][1]), month: parseChineseOrDigitMonth(yms[len(yms)-1][2])}
		if validMonth(first.month) && validMonth(last.month) {
			return fmt.Sprintf("%04d-%02d", first.year, first.month), fmt.Sprintf("%04d-%02d", last.year, last.month)
		}
	}
	if len(yms) == 1 {
		y := mustAtoi(yms[0][1])
		m := parseChineseOrDigitMonth(yms[0][2])
		if validMonth(m) {
			p := fmt.Sprintf("%04d-%02d", y, m)
			return p, p
		}
	}

	// 3) 相对月份
	switch {
	case strings.Contains(q, "今年") || strings.Contains(q, "本年"):
		return fmt.Sprintf("%04d-01", year), anchor.Format("2006-01")
	case strings.Contains(q, "上个月"):
		t := anchor.AddDate(0, -1, 0)
		p := t.Format("2006-01")
		return p, p
	case strings.Contains(q, "下个月"):
		t := anchor.AddDate(0, 1, 0)
		p := t.Format("2006-01")
		return p, p
	case strings.Contains(q, "本月") || strings.Contains(q, "这个月") || strings.Contains(q, "当月"):
		p := anchor.Format("2006-01")
		return p, p
	}

	// 4) 仅有月份（数字或中文月份）：自动锚定年份（若超过锚点月则归到上一年）
	monthRe := regexp.MustCompile(`([0-1]?\d|[一二三四五六七八九十两]{1,3})月`)
	if m := monthRe.FindStringSubmatch(q); len(m) == 2 {
		month := parseChineseOrDigitMonth(m[1])
		if validMonth(month) {
			y := year
			// 仅在跨度较大时回推上一年（例如 12 月 vs 当前 4 月）
			if month > anchorMonth && (month-anchorMonth) >= 6 {
				y = year - 1
			}
			p := fmt.Sprintf("%04d-%02d", y, month)
			return p, p
		}
	}

	period := anchor.Format("2006-01")
	return period, period
}

// ClassifyIntent 精准意图识别引擎 V6 (加权版)
func ClassifyIntent(question string) Intent {
	q := strings.ReplaceAll(question, " ", "")
	cfg := getRuleConfig()

	if containsAny(q, []string{"最大", "单笔", "流入对手方", "流出对手方"}) {
		return IntentLargeTransactionQuery
	}

	if containsAny(q, []string{"是谁", "身份", "干嘛的", "哪里的", "谁是"}) {
		return IntentIdentityQuery
	}

	// 这些问题虽然可能包含“应付”，但业务语义是人力成本，不应被 AR/AP 分流截走。
	if containsAny(q, cfg.IntentHRCostKeywords) {
		return IntentFallback
	}

	if containsAny(q, cfg.IntentARAPKeywords) {
		return IntentARAPQuery
	}

	if containsAny(q, cfg.IntentTaxKeywords) {
		return IntentTaxQuery
	}

	// 这类问法需要 fallback 结构化提示，而不是分析模块直接接管
	if containsAny(q, cfg.IntentHealthKeywords) {
		return IntentFallback
	}

	if containsAny(q, cfg.IntentAnalysisKeywords) {
		return IntentAnalysis
	}

	if containsAny(q, cfg.IntentFallbackKeywords) {
		return IntentFallback
	}

	if containsAny(q, cfg.IntentHostPayloadKeywords) {
		return IntentHostPayload
	}

	if strings.Contains(q, "项目") && containsAny(q, []string{"收入", "成本", "支出", "应收", "应付", "数据出来"}) {
		return IntentFallback
	}

	if containsAny(q, cfg.IntentMonthlySummaryKeywords) {
		return IntentMonthlySummary
	}

	if containsAny(q, []string{"期末", "余额", "是多少", "查询余额", "还有多少"}) {
		return IntentPrecise
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
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func companyMatchScore(query, company string) int {
	if company == "" {
		return -1
	}
	q := normalizeEntityText(query)
	c := normalizeEntityText(company)
	if q == "" || c == "" {
		return 0
	}
	if q == c {
		return 10000 + len(company)
	}

	score := 0
	if strings.Contains(c, q) {
		score = maxInt(score, 600+len(q))
	}
	if strings.Contains(q, c) {
		score = maxInt(score, 550+len(c))
	}

	aliases := companyAliases(company)
	for _, a := range aliases {
		na := normalizeEntityText(a)
		if len([]rune(na)) < 2 {
			continue
		}
		if strings.Contains(q, na) {
			score = maxInt(score, 300+len([]rune(na))*10)
		}
	}

	lcs := longestCommonSubstringRunes(q, c)
	score = maxInt(score, lcs*12)
	score = maxInt(score, queryNGramContainScore(q, c))
	return score
}

func companyAliases(company string) []string {
	aliases := []string{company}
	trimSuffixes := []string{"股份有限公司", "有限责任公司", "有限公司", "科技", "数据", "智能", "信息", "技术", "网络"}
	base := company
	for _, s := range []string{"股份有限公司", "有限责任公司", "有限公司"} {
		base = strings.ReplaceAll(base, s, "")
	}
	if base != "" {
		aliases = append(aliases, base)
	}

	segments := regexp.MustCompile(`[\x{4e00}-\x{9fa5}]{2,}`).FindAllString(base, -1)
	for _, seg := range segments {
		aliases = append(aliases, seg)
	}

	// 衍生更短别名（如“林悦智能”->“林悦”）
	for _, a := range append([]string{}, aliases...) {
		tmp := a
		for _, s := range trimSuffixes {
			tmp = strings.ReplaceAll(tmp, s, "")
		}
		tmp = strings.TrimSpace(tmp)
		if len([]rune(tmp)) >= 2 {
			aliases = append(aliases, tmp)
		}
	}

	seen := map[string]bool{}
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return len([]rune(out[i])) > len([]rune(out[j])) })
	return out
}

func parseChineseOrDigitMonth(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	switch raw {
	case "一":
		return 1
	case "二", "两":
		return 2
	case "三":
		return 3
	case "四":
		return 4
	case "五":
		return 5
	case "六":
		return 6
	case "七":
		return 7
	case "八":
		return 8
	case "九":
		return 9
	case "十":
		return 10
	case "十一":
		return 11
	case "十二":
		return 12
	default:
		return 0
	}
}

func validMonth(m int) bool {
	return m >= 1 && m <= 12
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func longestCommonSubstringRunes(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	dp := make([]int, len(br)+1)
	best := 0
	for i := 1; i <= len(ar); i++ {
		prev := 0
		for j := 1; j <= len(br); j++ {
			cur := dp[j]
			if ar[i-1] == br[j-1] {
				dp[j] = prev + 1
				if dp[j] > best {
					best = dp[j]
				}
			} else {
				dp[j] = 0
			}
			prev = cur
		}
	}
	return best
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func queryNGramContainScore(q, company string) int {
	runes := []rune(q)
	best := 0
	for length := minInt(8, len(runes)); length >= 2; length-- {
		for i := 0; i <= len(runes)-length; i++ {
			sub := string(runes[i : i+length])
			if strings.Contains(company, sub) {
				// 更偏好更长的命中片段
				score := 200 + length*length
				if score > best {
					best = score
				}
			}
		}
	}
	return best
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
