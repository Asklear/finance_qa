package query

import (
	"fmt"
	"strings"
)

func appendUniqueLedgerEvidence(base []LedgerEvidence, seen map[string]struct{}, incoming ...[]LedgerEvidence) []LedgerEvidence {
	for _, batch := range incoming {
		for _, ev := range batch {
			key := evidenceKey(ev)
			if _, ok := seen[key]; ok {
				continue
			}
			base = append(base, ev)
			seen[key] = struct{}{}
		}
	}
	return base
}

func cloneLedgerEvidence(src []LedgerEvidence) []LedgerEvidence {
	if len(src) == 0 {
		return nil
	}
	out := make([]LedgerEvidence, len(src))
	copy(out, src)
	return out
}

func (e *Engine) storeCounterpartyEvidenceCache(cacheKey string, evidence []LedgerEvidence) {
	cloned := cloneLedgerEvidence(evidence)
	e.cacheMu.Lock()
	e.counterpartyEvCache[cacheKey] = cloned
	e.cacheMu.Unlock()
}

func hasCompleteStructuredCounterpartyEvidence(evidence []LedgerEvidence) bool {
	hasBank := false
	hasProfitLoss := false
	hasSettlement := false
	for _, ev := range evidence {
		if ev.Source == "bank_statement" {
			hasBank = true
			continue
		}
		switch {
		case strings.HasPrefix(ev.AccountCode, "6001"), strings.HasPrefix(ev.AccountCode, "6051"),
			strings.HasPrefix(ev.AccountCode, "6401"), strings.HasPrefix(ev.AccountCode, "660"),
			strings.HasPrefix(ev.AccountCode, "222101"):
			hasProfitLoss = true
		case strings.HasPrefix(ev.AccountCode, "1122"), strings.HasPrefix(ev.AccountCode, "2202"), strings.HasPrefix(ev.AccountCode, "1123"):
			hasSettlement = true
		}
	}
	return hasProfitLoss || (hasSettlement && hasBank)
}

func evidenceKey(ev LedgerEvidence) string {
	return strings.Join([]string{
		ev.Source,
		strings.TrimSpace(ev.VoucherDate),
		strings.TrimSpace(ev.VoucherNo),
		strings.TrimSpace(ev.Counterparty),
		strings.TrimSpace(ev.AccountCode),
		strings.TrimSpace(ev.AccountName),
		strings.TrimSpace(ev.Summary),
		strings.TrimSpace(ev.Direction),
		fmt.Sprintf("%.6f", ev.DebitAmount),
		fmt.Sprintf("%.6f", ev.CreditAmount),
	}, "\x1f")
}
