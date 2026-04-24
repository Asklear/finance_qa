package query

import (
	"math"
	"sort"
)

func reconciliationSnapshotMagnitude(snap counterpartySnapshot) float64 {
	return math.Max(snap.BankIn+snap.BankOut, snap.RevenueNet+snap.BookCost+snap.BookExpense)
}

func selectReconciliationHighlights(snapshots []counterpartySnapshot, limit int) []counterpartySnapshot {
	filtered := make([]counterpartySnapshot, 0, len(snapshots))
	for _, snap := range snapshots {
		if snap.ComparisonBasis == "" {
			continue
		}
		filtered = append(filtered, snap)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return reconciliationSnapshotMagnitude(filtered[i]) > reconciliationSnapshotMagnitude(filtered[j])
	})
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func (e *Engine) collectReconciliationHighlights(from, to string, candidateLimit, highlightLimit int) []counterpartySnapshot {
	interesting := e.topCounterpartiesByCashMovement(from, to, candidateLimit)
	snapshots := make([]counterpartySnapshot, 0, len(interesting))
	for _, name := range interesting {
		snap := e.buildCounterpartySnapshot(name, from, to)
		if snap.Role == "unknown" && snap.BankIn == 0 && snap.BankOut == 0 && snap.RevenueNet == 0 && snap.BookCost == 0 && snap.BookExpense == 0 {
			continue
		}
		snapshots = append(snapshots, snap)
	}
	return selectReconciliationHighlights(snapshots, highlightLimit)
}
