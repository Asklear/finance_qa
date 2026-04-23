package query

import "strings"

func (e *Engine) collectCounterpartyEvidence(name, from, to string) []LedgerEvidence {
	cacheKey := buildCounterpartyEvidenceCacheKey(e.Company, name, from, to)
	e.cacheMu.RLock()
	if cached, ok := e.counterpartyEvCache[cacheKey]; ok {
		e.cacheMu.RUnlock()
		return cloneLedgerEvidence(cached)
	}
	e.cacheMu.RUnlock()

	startDate := from + "-01"
	endDate := monthEndDay(to)
	evidence := make([]LedgerEvidence, 0, 32)
	seen := map[string]struct{}{}

	direct := e.collectDirectCounterpartyEvidence(e.resolveCounterpartyCandidates(name), startDate, endDate)
	evidence = appendUniqueLedgerEvidence(evidence, seen, direct)
	if hasCompleteStructuredCounterpartyEvidence(evidence) {
		e.storeCounterpartyEvidenceCache(cacheKey, evidence)
		return cloneLedgerEvidence(evidence)
	}

	evidence = appendUniqueLedgerEvidence(evidence, seen, e.collectLikeMatchedCounterpartyEvidence(name, startDate, endDate))
	e.storeCounterpartyEvidenceCache(cacheKey, evidence)
	return cloneLedgerEvidence(evidence)
}

func buildCounterpartyEvidenceCacheKey(company, name, from, to string) string {
	return strings.Join([]string{
		strings.TrimSpace(company),
		strings.TrimSpace(name),
		strings.TrimSpace(from),
		strings.TrimSpace(to),
	}, "\x1f")
}
