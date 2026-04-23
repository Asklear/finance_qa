package query

import (
	"regexp"
	"strings"
)

func inferInternalPartyFromVoucher(company string, rows []voucherLedgerRow, cfg RuleConfig) (string, string) {
	contextScore := 0
	if voucherHasInternalSettlementDebit(rows, cfg) {
		contextScore = 1
	}
	texts := make([]string, 0, len(rows)*3)
	for _, row := range rows {
		texts = append(texts, row.Counterparty, row.Summary, row.AccountName)
	}
	return resolveInternalPartyFromTexts(company, texts, contextScore, cfg)
}

func resolveInternalPartyFromTexts(company string, texts []string, contextScore int, cfg RuleConfig) (string, string) {
	bestName := ""
	bestScore := 0
	bestBasis := ""
	for _, text := range texts {
		for _, candidate := range extractInternalPartyCandidates(text, cfg) {
			candidate = normalizeInternalPartyCandidate(company, candidate, cfg)
			score, basis := internalPartyScore(company, candidate, contextScore, cfg)
			if score > bestScore || (score == bestScore && len([]rune(candidate)) > len([]rune(bestName))) {
				bestScore = score
				bestName = candidate
				bestBasis = basis
			}
		}
	}
	if bestScore < 2 {
		return "", ""
	}
	return bestName, bestBasis
}

func normalizeInternalPartyCandidate(company, candidate string, cfg RuleConfig) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	for _, alias := range companyAliases(company) {
		alias = strings.TrimSpace(alias)
		if len([]rune(alias)) < 2 {
			continue
		}
		if idx := strings.Index(candidate, alias); idx > 0 {
			trimmed := strings.TrimSpace(candidate[idx:])
			if looksLikeInternalOrgUnit(trimmed, cfg) {
				return trimmed
			}
		}
	}
	return candidate
}

func extractInternalPartyCandidates(text string, cfg RuleConfig) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	candidates := []string{text}
	pattern := internalOrgPattern(cfg)
	candidates = append(candidates, pattern.FindAllString(text, -1)...)

	fields := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '_', '-', '－', '—', ':', '：', '/', '／', ',', '，', ';', '；':
			return true
		default:
			return false
		}
	})
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if pattern.MatchString(field) {
			candidates = append(candidates, pattern.FindAllString(field, -1)...)
		}
		if looksLikeInternalOrgUnit(field, cfg) {
			candidates = append(candidates, field)
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func internalPartyScore(company, candidate string, contextScore int, cfg RuleConfig) (int, string) {
	candidate = strings.TrimSpace(candidate)
	if !looksLikeInternalOrgUnit(candidate, cfg) {
		return 0, ""
	}
	score := 1 + contextScore
	basis := []string{"org_unit"}
	if internalPartyMatchesCompany(company, candidate) {
		score += 2
		basis = append(basis, "shared_brand")
	}
	if isGenericBranchLabel(candidate, cfg) {
		score++
		basis = append(basis, "generic_branch_label")
	}
	if contextScore > 0 {
		basis = append(basis, "internal_account_context")
	}
	return score, strings.Join(basis, "+")
}

func internalPartyMatchesCompany(company, candidate string) bool {
	nCandidate := normalizeEntityText(candidate)
	if nCandidate == "" {
		return false
	}
	for _, alias := range companyAliases(company) {
		nAlias := normalizeEntityText(alias)
		if len([]rune(nAlias)) < 2 {
			continue
		}
		if strings.Contains(nCandidate, nAlias) || strings.Contains(nAlias, nCandidate) {
			return true
		}
	}
	return false
}

func looksLikeInternalOrgUnit(candidate string, cfg RuleConfig) bool {
	n := normalizeEntityText(candidate)
	for _, suffix := range cfg.InternalPartyOrgSuffixes() {
		if strings.Contains(n, normalizeEntityText(suffix)) {
			return true
		}
	}
	return false
}

func isGenericBranchLabel(candidate string, cfg RuleConfig) bool {
	n := normalizeEntityText(candidate)
	for _, suffix := range cfg.InternalPartyOrgSuffixes() {
		if strings.HasSuffix(n, normalizeEntityText(suffix)) &&
			!strings.Contains(n, normalizeEntityText("有限公司")) &&
			!strings.Contains(n, normalizeEntityText("有限责任公司")) {
			return true
		}
	}
	return false
}

func internalOrgPattern(cfg RuleConfig) *regexp.Regexp {
	suffixes := cfg.InternalPartyOrgSuffixes()
	if len(suffixes) == 0 {
		return regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]+?(?:分公司|子公司|事业部|办事处|分部|总部|总公司)`)
	}
	quoted := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		suffix = strings.TrimSpace(suffix)
		if suffix == "" {
			continue
		}
		quoted = append(quoted, regexp.QuoteMeta(suffix))
	}
	if len(quoted) == 0 {
		return regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]+?(?:分公司|子公司|事业部|办事处|分部|总部|总公司)`)
	}
	return regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]+?(?:` + strings.Join(quoted, "|") + `)`)
}
