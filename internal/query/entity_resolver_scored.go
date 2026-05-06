package query

import (
	"sort"
	"strings"
)

type scoredEntityCandidate struct {
	Name    string
	Score   int
	Sources map[string]struct{}
}

func (e *Engine) resolveEntityByScoredCandidates(question string) string {
	ranked := e.rankEntityCandidates(question)
	if len(ranked) == 0 {
		return ""
	}
	best := ranked[0]
	if best.Score < entityAcceptScoreThreshold() {
		return ""
	}
	if len(ranked) > 1 && best.Score-ranked[1].Score < entityAcceptMarginThreshold() {
		return ""
	}
	return best.Name
}

func (e *Engine) rankEntityCandidates(question string) []scoredEntityCandidate {
	q := strings.TrimSpace(question)
	nq := normalizeEntityText(q)
	if nq == "" {
		return nil
	}
	candidates := map[string]*scoredEntityCandidate{}
	addCandidate := func(name, source string) {
		name = strings.TrimSpace(name)
		if name == "" || !isRealishQueryEntity(name) {
			return
		}
		c, ok := candidates[name]
		if !ok {
			c = &scoredEntityCandidate{Name: name, Sources: map[string]struct{}{}}
			candidates[name] = c
		}
		if source != "" {
			c.Sources[source] = struct{}{}
		}
	}

	for _, name := range e.counterpartyNameCandidates() {
		addCandidate(name, "counterparty")
	}
	for _, name := range e.contractCustomerCandidates() {
		addCandidate(name, "contract_customer")
	}
	for _, name := range e.contractContentCandidates() {
		addCandidate(name, "contract_content")
	}

	scored := make([]scoredEntityCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		score := scoreEntityCandidate(q, nq, candidate.Name, candidate.Sources)
		if score <= 0 {
			continue
		}
		candidate.Score = score
		scored = append(scored, *candidate)
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return len([]rune(scored[i].Name)) > len([]rune(scored[j].Name))
		}
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > 8 {
		return scored[:8]
	}
	return scored
}

func scoreEntityCandidate(question, normalizedQuestion, name string, sources map[string]struct{}) int {
	normalizedName := normalizeEntityText(name)
	if len([]rune(normalizedName)) < 2 {
		return 0
	}
	if strings.Contains(normalizedQuestion, normalizedName) {
		return 10000 + len([]rune(normalizedName))*10 + entitySourceScore(sources)
	}

	best := 0
	for _, seg := range chineseEntitySegmentPattern.FindAllString(strings.TrimSpace(question), -1) {
		runes := []rune(seg)
		for length := len(runes); length >= 2; length-- {
			for i := 0; i <= len(runes)-length; i++ {
				fragment := trimEntityNoiseSuffixes(stripTemporalNoise(string(runes[i : i+length])))
				if shouldSkipEntityFragment(fragment, 2) {
					continue
				}
				normalizedFragment := normalizeEntityText(fragment)
				if len([]rune(normalizedFragment)) < 2 {
					continue
				}
				score := entityFragmentCandidateScore(normalizedFragment, normalizedName)
				if score > best {
					best = score
				}
			}
		}
	}
	if best == 0 {
		return 0
	}
	return best + entitySourceScore(sources)
}

func entityFragmentCandidateScore(fragment, candidate string) int {
	switch {
	case fragment == candidate:
		return 9000 + len([]rune(fragment))*10
	case strings.Contains(candidate, fragment):
		if len([]rune(fragment)) < 2 {
			return 0
		}
		return len([]rune(fragment))*100 - len([]rune(candidate))
	case strings.Contains(fragment, candidate):
		return len([]rune(candidate))*80 - len([]rune(fragment))
	default:
		return 0
	}
}

func entitySourceScore(sources map[string]struct{}) int {
	score := 0
	if _, ok := sources["contract_customer"]; ok {
		score += 80
	}
	if _, ok := sources["counterparty"]; ok {
		score += 60
	}
	if _, ok := sources["contract_content"]; ok {
		score += 30
	}
	return score
}

func entityAcceptScoreThreshold() int {
	return 220
}

func entityAcceptMarginThreshold() int {
	return 25
}
