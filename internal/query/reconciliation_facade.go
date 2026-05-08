package query

import queryreconciliation "financeqa/internal/query/reconciliation"

type evidenceLevel = queryreconciliation.EvidenceLevel

const (
	evidenceDirect  = queryreconciliation.EvidenceDirect
	evidenceDerived = queryreconciliation.EvidenceDerived
	evidenceUnknown = queryreconciliation.EvidenceUnknown
)

type counterpartySnapshot = queryreconciliation.CounterpartySnapshot
type voucherContext = queryreconciliation.VoucherContext
type monthlyBookView = queryreconciliation.MonthlyBookView
