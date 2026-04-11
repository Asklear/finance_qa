package dimensions

import "time"

// ReportOptions controls generated mapping report shape.
type ReportOptions struct {
	IncludeDetails         bool
	UnmappedSummaryLimit   int
	RuleUsageLimit         int
	IncludeRecommendations bool
}

// MappingResult captures one auto-mapping run summary.
type MappingResult struct {
	Success   bool              `json:"success"`
	Company   string            `json:"company"`
	Period    string            `json:"period"`
	Stats     MappingRunStats   `json:"stats"`
	Mappings  []MappedJournal   `json:"mappings"`
	Unmapped  []UnmappedJournal `json:"unmappedJournals"`
	Conflicts []RuleConflict    `json:"conflicts"`
	Message   string            `json:"message"`
}

// MappingRunStats summarizes mapping counts.
type MappingRunStats struct {
	Total           int `json:"total"`
	Mapped          int `json:"mapped"`
	Unmapped        int `json:"unmapped"`
	Conflicts       int `json:"conflicts"`
	ManualOverrides int `json:"manualOverrides"`
}

// MappedJournal is one mapped journal projection.
type MappedJournal struct {
	JournalID     int64   `json:"journalId"`
	DimensionType string  `json:"dimensionType"`
	MemberCode    string  `json:"memberCode"`
	MemberName    string  `json:"memberName"`
	Confidence    float64 `json:"confidence"`
	RuleID        *int64  `json:"ruleId,omitempty"`
	IsManual      bool    `json:"isManual"`
}

// UnmappedJournal is one journal needing manual mapping.
type UnmappedJournal struct {
	JournalID   int64   `json:"journalId"`
	AccountCode string  `json:"accountCode"`
	AccountName string  `json:"accountName"`
	Summary     string  `json:"summary"`
	Amount      float64 `json:"amount"`
}

// RuleConflict describes same-priority matching collisions.
type RuleConflict struct {
	JournalID int64                `json:"journalId"`
	Rules     []RuleConflictDetail `json:"rules"`
}

// RuleConflictDetail is one competing rule.
type RuleConflictDetail struct {
	RuleID     int64  `json:"ruleId"`
	MemberCode string `json:"memberCode"`
	Priority   int    `json:"priority"`
}

// DetailedMappingStats is report-ready mapping quality data.
type DetailedMappingStats struct {
	TotalJournals           int                         `json:"totalJournals"`
	MappedJournals          int                         `json:"mappedJournals"`
	UnmappedJournals        int                         `json:"unmappedJournals"`
	ManualMappings          int                         `json:"manualMappings"`
	AutoMappings            int                         `json:"autoMappings"`
	MappingRate             float64                     `json:"mappingRate"`
	DimensionDistribution   []DimensionDistributionItem `json:"dimensionDistribution"`
	UnmappedSummaryAnalysis []UnmappedSummaryItem       `json:"unmappedSummaryAnalysis"`
	RuleUsageStats          []RuleUsageStat             `json:"ruleUsageStats"`
	QualityMetrics          MappingQualityMetrics       `json:"qualityMetrics"`
	Company                 string                      `json:"company"`
	Period                  string                      `json:"period"`
	GeneratedAt             time.Time                   `json:"generatedAt"`
}

// DimensionDistributionItem is per-dimension mapped volume share.
type DimensionDistributionItem struct {
	DimensionType string  `json:"dimensionType"`
	DimensionName string  `json:"dimensionName"`
	MappedCount   int     `json:"mappedCount"`
	Percentage    float64 `json:"percentage"`
}

// UnmappedSummaryItem is grouped unmapped summary analysis.
type UnmappedSummaryItem struct {
	Summary     string  `json:"summary"`
	Count       int     `json:"count"`
	TotalAmount float64 `json:"totalAmount"`
	Percentage  float64 `json:"percentage"`
}

// RuleUsageStat tracks mapping rule coverage.
type RuleUsageStat struct {
	RuleID        int64   `json:"ruleId"`
	RuleName      string  `json:"ruleName"`
	DimensionType string  `json:"dimensionType"`
	MemberCode    string  `json:"memberCode"`
	MemberName    string  `json:"memberName"`
	UsageCount    int     `json:"usageCount"`
	Percentage    float64 `json:"percentage"`
}

// MappingQualityMetrics tracks confidence bands.
type MappingQualityMetrics struct {
	HighConfidenceRate   float64 `json:"highConfidenceRate"`
	MediumConfidenceRate float64 `json:"mediumConfidenceRate"`
	LowConfidenceRate    float64 `json:"lowConfidenceRate"`
	AverageConfidence    float64 `json:"averageConfidence"`
}
