package openitems

type CounterpartyEvidenceSource string

const (
	CounterpartyEvidenceDirect  CounterpartyEvidenceSource = "direct"
	CounterpartyEvidenceSummary CounterpartyEvidenceSource = "summary"
	CounterpartyEvidenceAccount CounterpartyEvidenceSource = "account_name"
	CounterpartyEvidenceUnknown CounterpartyEvidenceSource = "unknown"
)

type MatchConfidence string

const (
	MatchConfidenceConfirmed MatchConfidence = "confirmed"
	MatchConfidenceProbable  MatchConfidence = "probable"
	MatchConfidenceUnmatched MatchConfidence = "unmatched"
)

type SettlementConfidence struct {
	ConfirmedHistoricalSettlement float64 `json:"confirmed_historical_settlement"`
	ProbableHistoricalSettlement  float64 `json:"probable_historical_settlement"`
	ConfirmedCurrentSettlement    float64 `json:"confirmed_current_settlement"`
	ProbableCurrentSettlement     float64 `json:"probable_current_settlement"`
	UnmatchedDecrease             float64 `json:"unmatched_decrease"`
}

func (s SettlementConfidence) HasInference() bool {
	return round2(s.ProbableHistoricalSettlement+s.ProbableCurrentSettlement+s.UnmatchedDecrease) > 0
}

func (s SettlementConfidence) merge(other SettlementConfidence) SettlementConfidence {
	return SettlementConfidence{
		ConfirmedHistoricalSettlement: round2(s.ConfirmedHistoricalSettlement + other.ConfirmedHistoricalSettlement),
		ProbableHistoricalSettlement:  round2(s.ProbableHistoricalSettlement + other.ProbableHistoricalSettlement),
		ConfirmedCurrentSettlement:    round2(s.ConfirmedCurrentSettlement + other.ConfirmedCurrentSettlement),
		ProbableCurrentSettlement:     round2(s.ProbableCurrentSettlement + other.ProbableCurrentSettlement),
		UnmatchedDecrease:             round2(s.UnmatchedDecrease + other.UnmatchedDecrease),
	}
}
