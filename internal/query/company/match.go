package company

import (
	"regexp"
	"sort"
	"strings"
)

// Resolve returns the best company match for a requested company name.
func Resolve(req string, companies []string) string {
	if len(companies) == 0 {
		return req
	}
	q := strings.TrimSpace(req)
	if q == "" {
		return defaultResolvedCompany(companies)
	}
	if isPlaceholderCompanyName(q) {
		return defaultResolvedCompany(companies)
	}

	// 若用户输入的是简称，优先提升到包含该简称的最长正式公司名。
	for _, c := range companies {
		if normalizeCompanyText(c) == normalizeCompanyText(q) {
			best := c
			for _, other := range companies {
				if other != c && strings.Contains(normalizeCompanyText(other), normalizeCompanyText(q)) && len([]rune(other)) > len([]rune(best)) {
					best = other
				}
			}
			return best
		}
	}

	best, _ := BestMatch(q, companies)
	if best == "" {
		return companies[0]
	}
	return best
}

func defaultResolvedCompany(companies []string) string {
	for _, company := range companies {
		if strings.TrimSpace(company) != "" && !isPlaceholderCompanyName(company) {
			return company
		}
	}
	if len(companies) == 0 {
		return ""
	}
	return companies[0]
}

func isPlaceholderCompanyName(company string) bool {
	switch strings.ToLower(strings.TrimSpace(company)) {
	case "defaultcompany", "default_company":
		return true
	default:
		return false
	}
}

func ResolveMention(question string, companies []string) string {
	q := strings.TrimSpace(question)
	if q == "" || len(companies) == 0 {
		return ""
	}

	best, score := BestMatch(q, companies)
	if score <= 0 {
		return ""
	}
	return best
}

func BestMatch(query string, companies []string) (string, int) {
	if len(companies) == 0 {
		return "", -1
	}

	best := companies[0]
	bestScore := matchScore(query, best)
	for _, c := range companies[1:] {
		score := matchScore(query, c)
		if score > bestScore || (score == bestScore && len(c) > len(best)) {
			best = c
			bestScore = score
		}
	}
	return best, bestScore
}

func matchScore(query, company string) int {
	if company == "" {
		return -1
	}
	q := normalizeCompanyText(query)
	c := normalizeCompanyText(company)
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

	aliases := Aliases(company)
	for _, a := range aliases {
		na := normalizeCompanyText(a)
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

func Aliases(company string) []string {
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

	// 衍生更短别名（如“某某智能”->“某某”）。
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

func queryNGramContainScore(q, company string) int {
	runes := []rune(q)
	best := 0
	for length := minInt(8, len(runes)); length >= 2; length-- {
		for i := 0; i <= len(runes)-length; i++ {
			sub := string(runes[i : i+length])
			if strings.Contains(company, sub) {
				// 更偏好更长的命中片段。
				score := 200 + length*length
				if score > best {
					best = score
				}
			}
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizeCompanyText(s string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "（", "", "）", "", "(", "", ")", "", "-", "", "_", "", ",", "", "，", "", ".", "", "。", "")
	return replacer.Replace(strings.TrimSpace(s))
}
