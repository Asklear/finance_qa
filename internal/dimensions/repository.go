package dimensions

import "context"

// Repository is a package-local persistence boundary for dimensions.
// Other packages can provide adapters (e.g. SQLite) without coupling manager logic to DB code.
type Repository interface {
	CreateDimension(ctx context.Context, dim Dimension) (Dimension, error)
	GetDimensionByID(ctx context.Context, id int64) (Dimension, error)
	GetDimensionByCode(ctx context.Context, code string) (Dimension, error)
	UpdateDimension(ctx context.Context, id int64, patch DimensionPatch) (Dimension, error)
	DeleteDimension(ctx context.Context, id int64) error
	ListDimensions(ctx context.Context, opts DimensionQueryOptions) ([]Dimension, int, error)

	CreateMember(ctx context.Context, member DimensionMember) (DimensionMember, error)
	GetMemberByID(ctx context.Context, id int64) (DimensionMember, error)
	GetMemberByCode(ctx context.Context, dimensionID int64, code string) (DimensionMember, error)
	UpdateMember(ctx context.Context, id int64, patch MemberPatch) (DimensionMember, error)
	DeleteMember(ctx context.Context, id int64) error
	ListMembers(ctx context.Context, opts MemberQueryOptions) ([]DimensionMember, int, error)

	CreateMappingRule(ctx context.Context, rule MappingRule) (MappingRule, error)
	GetMappingRuleByID(ctx context.Context, id int64) (MappingRule, error)
	GetMappingRuleByName(ctx context.Context, company, ruleName string) (MappingRule, error)
	UpdateMappingRule(ctx context.Context, id int64, patch MappingRulePatch) (MappingRule, error)
	DeleteMappingRule(ctx context.Context, id int64) error
	ListMappingRules(ctx context.Context, opts MappingRuleQueryOptions) ([]MappingRule, int, error)
}
