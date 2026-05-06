package query

import (
	"regexp"
	"sort"
	"strings"
)

func (e *Engine) matchCounterpartyByName(question string) string {
	nq := normalizeEntityText(question)
	if nq == "" {
		return ""
	}
	for _, name := range e.counterpartyNameCandidates() {
		nm := normalizeEntityText(name)
		if len([]rune(nm)) < 2 {
			continue
		}
		if strings.Contains(nq, nm) {
			return name
		}
	}
	return ""
}

func (e *Engine) matchCounterpartyByAliasSegment(question string) string {
	for _, seg := range chineseEntitySegmentPattern.FindAllString(strings.TrimSpace(question), -1) {
		runes := []rune(seg)
		for length := len(runes); length >= 2; length-- {
			for i := 0; i <= len(runes)-length; i++ {
				sub := string(runes[i : i+length])
				if shouldSkipEntityFragment(sub, 4) {
					continue
				}
				candidates := e.resolveCounterpartyCandidates(sub)
				if len(candidates) > 0 {
					return candidates[0]
				}
			}
		}
	}
	return ""
}

func counterpartyNameCandidatesQuery() string {
	return `
SELECT name
FROM (
  SELECT counterparty_name AS name
  FROM bank_statement
  WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
    AND COALESCE(TRIM(counterparty_name), '') <> ''
  UNION
  SELECT counterparty AS name
  FROM journal
  WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
    AND COALESCE(TRIM(counterparty), '') <> ''
  UNION
  SELECT customer_name AS name
  FROM fin_contracts
  WHERE COALESCE(TRIM(customer_name), '') <> ''
) candidates
ORDER BY LENGTH(name) DESC, name
`
}

func (e *Engine) counterpartyNameCandidates() []string {
	companyKey := strings.TrimSpace(e.Company)
	e.cacheMu.RLock()
	if cached, ok := e.counterpartyNames[companyKey]; ok {
		out := append([]string{}, cached...)
		e.cacheMu.RUnlock()
		return out
	}
	e.cacheMu.RUnlock()

	rows, err := e.db.Query(counterpartyNameCandidatesQuery(), e.Company, e.Company, e.Company, e.Company)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]string, 0, 128)
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, name)
		}
	}
	out = append(out, e.summaryDerivedCounterpartyCandidates()...)
	out = dedupeStrings(out)

	e.cacheMu.Lock()
	e.counterpartyNames[companyKey] = append([]string{}, out...)
	e.cacheMu.Unlock()
	return out
}

func (e *Engine) resolveCounterpartyCandidates(alias string) []string {
	return rankCounterpartyAliasMatches(alias, e.counterpartyNameCandidates())
}

func (e *Engine) summaryDerivedCounterpartyCandidates() []string {
	rows, err := e.db.Query(`
SELECT summary
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND COALESCE(TRIM(summary), '') <> ''
`, e.Company, e.Company)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]string, 0, 32)
	for rows.Next() {
		var summary string
		if err := rows.Scan(&summary); err != nil {
			continue
		}
		if name := extractCounterpartyNameFromSummary(summary); name != "" {
			out = append(out, name)
		}
	}
	return out
}

var queryCompanyNamePattern = regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]+?(?:有限责任公司|股份有限公司|有限公司|事务所|中心|分公司|公司)`)

func extractCounterpartyNameFromSummary(summary string) string {
	text := strings.TrimSpace(summary)
	if text == "" {
		return ""
	}
	candidates := make([]string, 0, 4)
	for _, matched := range queryCompanyNamePattern.FindAllString(trimSummaryEntityNoise(text), -1) {
		candidates = append(candidates, matched)
	}
	separators := []string{"_", "-", "－", "—", ":", "：", "/", "／", "(", ")", "（", "）"}
	for _, sep := range separators {
		text = strings.ReplaceAll(text, sep, " ")
	}
	for _, part := range strings.Fields(text) {
		part = trimSummaryEntityNoise(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "公司") || strings.Contains(part, "中心") || strings.Contains(part, "事务所") {
			candidates = append(candidates, part)
		}
	}
	best := ""
	bestLen := 0
	for _, candidate := range candidates {
		candidate = trimSummaryEntityNoise(strings.TrimSpace(candidate))
		if candidate == "" || !isRealishQueryEntity(candidate) {
			continue
		}
		candidateLen := len([]rune(normalizeEntityText(candidate)))
		if candidateLen < 6 {
			continue
		}
		if best == "" || candidateLen < bestLen {
			best = candidate
			bestLen = candidateLen
		}
	}
	return best
}

func trimSummaryEntityNoise(name string) string {
	name = strings.TrimSpace(name)
	prefixes := []string{"收到", "转账", "支付", "付款", "付", "为", "向", "给", "预提成本", "冲销", "冲回", "结转", "购买", "采购", "购入", "购买了", "采购了", "代付", "代收"}
	suffixes := []string{"发票", "服务", "服务费", "转账", "结算款", "款"}
	changed := true
	for changed {
		changed = false
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				name = strings.TrimSpace(strings.TrimPrefix(name, prefix))
				changed = true
			}
		}
		for _, suffix := range suffixes {
			if strings.HasSuffix(name, suffix) {
				name = strings.TrimSpace(strings.TrimSuffix(name, suffix))
				changed = true
			}
		}
	}
	return strings.TrimSpace(name)
}

func rankCounterpartyAliasMatches(alias string, names []string) []string {
	nAlias := normalizeEntityText(alias)
	if len([]rune(nAlias)) < 2 {
		return nil
	}

	type candidateScore struct {
		name  string
		score int
	}
	matches := make([]candidateScore, 0, 8)
	seen := map[string]struct{}{}
	for _, name := range names {
		nName := normalizeEntityText(name)
		if len([]rune(nName)) < 2 {
			continue
		}
		if !strings.Contains(nName, nAlias) && !strings.Contains(nAlias, nName) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		score := 0
		switch {
		case nName == nAlias:
			score = 10000
		case strings.Contains(nName, nAlias):
			score = len([]rune(nAlias))*100 - len([]rune(nName))
		default:
			score = len([]rune(nName))*60 - len([]rune(nAlias))
		}
		matches = append(matches, candidateScore{name: name, score: score})
		seen[name] = struct{}{}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return len([]rune(matches[i].name)) < len([]rune(matches[j].name))
		}
		return matches[i].score > matches[j].score
	})

	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.name)
		if len(out) == 6 {
			break
		}
	}
	return out
}
