package query

import (
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

	e.cacheMu.Lock()
	e.counterpartyNames[companyKey] = append([]string{}, out...)
	e.cacheMu.Unlock()
	return out
}

func (e *Engine) resolveCounterpartyCandidates(alias string) []string {
	return rankCounterpartyAliasMatches(alias, e.counterpartyNameCandidates())
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
