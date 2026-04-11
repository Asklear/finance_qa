package dimensions

import "time"

// DimensionType represents supported dimension categories.
type DimensionType string

const (
	DimensionTypeDepartment DimensionType = "department"
	DimensionTypeProject    DimensionType = "project"
	DimensionTypeProduct    DimensionType = "product"
	DimensionTypeRegion     DimensionType = "region"
	DimensionTypeChannel    DimensionType = "channel"
	DimensionTypeCustomer   DimensionType = "customer"
	DimensionTypeCustom     DimensionType = "custom"
)

func (t DimensionType) Valid() bool {
	switch t {
	case DimensionTypeDepartment,
		DimensionTypeProject,
		DimensionTypeProduct,
		DimensionTypeRegion,
		DimensionTypeChannel,
		DimensionTypeCustomer,
		DimensionTypeCustom:
		return true
	default:
		return false
	}
}

// Dimension is a business dimension definition (e.g. department, product).
type Dimension struct {
	ID             int64          `json:"id"`
	Code           string         `json:"code"`
	Name           string         `json:"name"`
	Description    *string        `json:"description,omitempty"`
	Type           DimensionType  `json:"type"`
	IsActive       bool           `json:"isActive"`
	IsHierarchical bool           `json:"isHierarchical"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// DimensionMember is one member under a dimension.
type DimensionMember struct {
	ID          int64          `json:"id"`
	DimensionID int64          `json:"dimensionId"`
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	ParentID    *int64         `json:"parentId,omitempty"`
	Level       int            `json:"level"`
	Path        string         `json:"path"`
	IsActive    bool           `json:"isActive"`
	SortOrder   int            `json:"sortOrder"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// TreeNode is a member tree projection.
type TreeNode struct {
	ID       int64      `json:"id"`
	Code     string     `json:"code"`
	Name     string     `json:"name"`
	Level    int        `json:"level"`
	Path     string     `json:"path"`
	Children []TreeNode `json:"children"`
	IsLeaf   bool       `json:"isLeaf"`
}

// MappingRule models automatic journal-to-dimension mapping rules.
type MappingRule struct {
	ID                  int64      `json:"id"`
	Company             string     `json:"company"`
	RuleName            string     `json:"ruleName"`
	Priority            int        `json:"priority"`
	AccountCodePattern  *string    `json:"accountCodePattern,omitempty"`
	AccountNamePattern  *string    `json:"accountNamePattern,omitempty"`
	SummaryPattern      *string    `json:"summaryPattern,omitempty"`
	CounterpartyPattern *string    `json:"counterpartyPattern,omitempty"`
	DimensionCode       string     `json:"dimensionCode"`
	MemberCode          string     `json:"memberCode"`
	AllocationRatio     float64    `json:"allocationRatio"`
	ValidFrom           *string    `json:"validFrom,omitempty"`
	ValidTo             *string    `json:"validTo,omitempty"`
	IsActive            bool       `json:"isActive"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           *time.Time `json:"updatedAt,omitempty"`
}

// ImportResult reports a batch member import outcome.
type ImportResult struct {
	Success      bool          `json:"success"`
	TotalCount   int           `json:"totalCount"`
	SuccessCount int           `json:"successCount"`
	FailedCount  int           `json:"failedCount"`
	Errors       []ImportError `json:"errors"`
	Message      string        `json:"message"`
}

// ImportError describes one row-level import failure.
type ImportError struct {
	Row    int    `json:"row"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// DimensionQueryOptions filters dimensions with pagination.
type DimensionQueryOptions struct {
	Type     *DimensionType
	IsActive *bool
	Keyword  string
	Limit    int
	Offset   int
}

// MemberQueryOptions filters members with pagination.
type MemberQueryOptions struct {
	DimensionID  *int64
	ParentID     *int64
	ParentIsNull bool
	IsActive     *bool
	Keyword      string
	Level        *int
	Limit        int
	Offset       int
}

// MappingRuleQueryOptions filters mapping rules with pagination.
type MappingRuleQueryOptions struct {
	Company       string
	IsActive      *bool
	DimensionCode string
	Keyword       string
	Limit         int
	Offset        int
}

// PaginatedResult returns windowed query results.
type PaginatedResult[T any] struct {
	Data       []T `json:"data"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalPages int `json:"totalPages"`
}

// CreateDimensionInput contains attributes for a new dimension.
type CreateDimensionInput struct {
	Code           string
	Name           string
	Type           DimensionType
	Description    *string
	IsHierarchical bool
	Metadata       map[string]any
}

// DimensionPatch updates mutable fields on a dimension.
type DimensionPatch struct {
	Name           *string
	Description    *string
	Type           *DimensionType
	IsActive       *bool
	IsHierarchical *bool
	Metadata       map[string]any
	MetadataSet    bool
}

// AddMemberInput contains attributes for a new member.
type AddMemberInput struct {
	DimensionID int64
	Code        string
	Name        string
	ParentID    *int64
	SortOrder   int
	Metadata    map[string]any
}

// MemberPatch updates mutable fields on a member.
type MemberPatch struct {
	Name        *string
	ParentID    *int64
	ParentIDSet bool
	IsActive    *bool
	SortOrder   *int
	Metadata    map[string]any
	MetadataSet bool
}

// CreateMappingRuleInput contains attributes for a new mapping rule.
type CreateMappingRuleInput struct {
	Company             string
	RuleName            string
	Priority            int
	AccountCodePattern  *string
	AccountNamePattern  *string
	SummaryPattern      *string
	CounterpartyPattern *string
	DimensionCode       string
	MemberCode          string
	AllocationRatio     float64
	ValidFrom           *string
	ValidTo             *string
	IsActive            *bool
}

// MappingRulePatch updates mutable fields on a mapping rule.
type MappingRulePatch struct {
	RuleName            *string
	Priority            *int
	AccountCodePattern  *string
	AccountNamePattern  *string
	SummaryPattern      *string
	CounterpartyPattern *string
	DimensionCode       *string
	MemberCode          *string
	AllocationRatio     *float64
	ValidFrom           *string
	ValidTo             *string
	IsActive            *bool
}
