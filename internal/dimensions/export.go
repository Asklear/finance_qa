package dimensions

import "time"

type ExportDataPackage struct {
	Version      string                    `json:"version"`
	ExportedAt   time.Time                 `json:"exportedAt"`
	Dimensions   []DimensionExport         `json:"dimensions"`
	Members      map[string][]MemberExport `json:"members"`
	MappingRules []MappingRuleExport       `json:"mappingRules"`
}

type DimensionExport struct {
	Code           string        `json:"code"`
	Name           string        `json:"name"`
	Type           DimensionType `json:"type"`
	Description    *string       `json:"description,omitempty"`
	IsHierarchical bool          `json:"isHierarchical"`
	IsActive       bool          `json:"isActive"`
}

type MemberExport struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	ParentCode *string `json:"parentCode,omitempty"`
	Level      int     `json:"level"`
	Path       string  `json:"path"`
	IsActive   bool    `json:"isActive"`
	SortOrder  int     `json:"sortOrder"`
}

type MappingRuleExport struct {
	ID                  *int64  `json:"id,omitempty"`
	Company             string  `json:"company"`
	RuleName            string  `json:"ruleName"`
	Priority            int     `json:"priority"`
	AccountCodePattern  *string `json:"accountCodePattern,omitempty"`
	AccountNamePattern  *string `json:"accountNamePattern,omitempty"`
	SummaryPattern      *string `json:"summaryPattern,omitempty"`
	CounterpartyPattern *string `json:"counterpartyPattern,omitempty"`
	DimensionCode       string  `json:"dimensionCode"`
	MemberCode          string  `json:"memberCode"`
	AllocationRatio     float64 `json:"allocationRatio"`
	ValidFrom           *string `json:"validFrom,omitempty"`
	ValidTo             *string `json:"validTo,omitempty"`
	IsActive            bool    `json:"isActive"`
}
