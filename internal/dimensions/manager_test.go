package dimensions

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"financeqa/internal/db"

	_ "modernc.org/sqlite"
)

func TestMemoryRepositoryAndManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	mgr := NewManager(repo)

	desc := "Product dimension"
	meta := map[string]any{"source": "seed"}
	dim, err := mgr.CreateDimension(ctx, CreateDimensionInput{
		Code:           " product ",
		Name:           " Product ",
		Type:           DimensionTypeProduct,
		Description:    &desc,
		IsHierarchical: true,
		Metadata:       meta,
	})
	if err != nil {
		t.Fatalf("create dimension: %v", err)
	}
	if dim.Code != "product" || dim.Name != "Product" {
		t.Fatalf("trimmed dimension fields not preserved: %+v", dim)
	}
	meta["source"] = "mutated"
	gotDim, err := mgr.GetDimensionByCode(ctx, "PRODUCT")
	if err != nil {
		t.Fatalf("get dimension by code: %v", err)
	}
	if gotDim.Metadata["source"] != "seed" {
		t.Fatalf("dimension metadata should be cloned, got %+v", gotDim.Metadata)
	}

	if _, err := mgr.CreateDimension(ctx, CreateDimensionInput{Code: "product", Name: "Duplicate", Type: DimensionTypeProduct}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate dimension error = %v, want ErrAlreadyExists", err)
	}
	typed := DimensionTypeProduct
	listed, err := mgr.ListDimensions(ctx, DimensionQueryOptions{Type: &typed})
	if err != nil {
		t.Fatalf("list dimensions: %v", err)
	}
	if listed.Total != 1 || len(listed.Data) != 1 {
		t.Fatalf("unexpected listed dimensions: %+v", listed)
	}

	root, err := mgr.AddMember(ctx, AddMemberInput{
		DimensionID: dim.ID,
		Code:        " P001 ",
		Name:        " SaaS ",
	})
	if err != nil {
		t.Fatalf("add root member: %v", err)
	}
	child, err := mgr.AddMember(ctx, AddMemberInput{
		DimensionID: dim.ID,
		Code:        " P001A ",
		Name:        " SaaS Enterprise ",
		ParentID:    &root.ID,
	})
	if err != nil {
		t.Fatalf("add child member: %v", err)
	}
	if child.Level != 2 || child.Path != "P001/P001A" {
		t.Fatalf("unexpected child member shape: %+v", child)
	}

	tree, err := mgr.GetMemberTree(ctx, dim.ID, nil)
	if err != nil {
		t.Fatalf("get member tree: %v", err)
	}
	if len(tree) != 1 || len(tree[0].Children) != 1 || tree[0].IsLeaf {
		t.Fatalf("unexpected tree: %+v", tree)
	}

	active := true
	rule, err := mgr.CreateMappingRule(ctx, CreateMappingRuleInput{
		Company:             "ACME",
		RuleName:            "Revenue",
		Priority:            10,
		AccountCodePattern:  strPtr("%6001%"),
		AccountNamePattern:  strPtr("%收入%"),
		SummaryPattern:      strPtr("%客户%"),
		CounterpartyPattern: strPtr("%客户%"),
		DimensionCode:       dim.Code,
		MemberCode:          root.Code,
		AllocationRatio:     1,
		IsActive:            &active,
	})
	if err != nil {
		t.Fatalf("create mapping rule: %v", err)
	}

	mapper, err := mgr.GetMapper(ctx, "ACME")
	if err != nil {
		t.Fatalf("get mapper: %v", err)
	}
	if got, ok := mapper.MapAccount("600101", "主营业务收入", "客户回款", "客户A"); !ok || got != root.Code {
		t.Fatalf("map account = (%q, %v), want %q, true", got, ok, root.Code)
	}
	if got := mapper.GetCode("主营业务收入"); got != root.Code {
		t.Fatalf("mapper get code = %q, want %q", got, root.Code)
	}
	if got := mapper.MapCategory("600101", "主营业务收入", "客户回款", "客户A", func(code string) string { return "fallback:" + code }); got != root.Code {
		t.Fatalf("map category = %q, want %q", got, root.Code)
	}

	rules, err := mgr.ListMappingRules(ctx, MappingRuleQueryOptions{Company: "ACME", IsActive: &active})
	if err != nil {
		t.Fatalf("list mapping rules: %v", err)
	}
	if rules.Total != 1 {
		t.Fatalf("unexpected active rule count: %+v", rules)
	}

	inactive := false
	updatedRule, err := mgr.UpdateMappingRule(ctx, rule.ID, MappingRulePatch{IsActive: &inactive})
	if err != nil {
		t.Fatalf("update mapping rule: %v", err)
	}
	if updatedRule.IsActive {
		t.Fatal("expected rule to be deactivated")
	}

	rules, err = mgr.ListMappingRules(ctx, MappingRuleQueryOptions{Company: "ACME", IsActive: &active})
	if err != nil {
		t.Fatalf("list active rules after update: %v", err)
	}
	if rules.Total != 0 {
		t.Fatalf("expected no active rules after deactivation, got %+v", rules)
	}

	if err := mgr.DeleteMember(ctx, root.ID, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete member without cascade = %v, want ErrConflict", err)
	}
	if err := mgr.DeleteDimension(ctx, dim.ID, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete dimension without force = %v, want ErrConflict", err)
	}
	if err := mgr.DeleteMember(ctx, root.ID, true); err != nil {
		t.Fatalf("cascade delete member: %v", err)
	}
	if err := mgr.DeleteDimension(ctx, dim.ID, false); err != nil {
		t.Fatalf("delete empty dimension: %v", err)
	}
}

func TestInitializeStandardRulesSeedsStandardChart(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager(NewMemoryRepository())

	if err := mgr.InitializeStandardRules(ctx, "ACME"); err != nil {
		t.Fatalf("initialize standard rules: %v", err)
	}

	for _, code := range []string{"CAS", "PROJECT", "CUSTOMER", "SUPPLIER"} {
		if _, err := mgr.GetDimensionByCode(ctx, code); err != nil {
			t.Fatalf("expected seeded dimension %s: %v", code, err)
		}
	}

	mapper, err := mgr.GetMapper(ctx, "ACME")
	if err != nil {
		t.Fatalf("get mapper: %v", err)
	}
	if got, ok := mapper.MapAccount("600101", "", "", ""); !ok || got != "6001" {
		t.Fatalf("expected standard CAS rule for 600101 to map to 6001, got (%q, %v)", got, ok)
	}
	if got, ok := mapper.MapAccount("160101", "", "", ""); !ok || got != "160101" {
		t.Fatalf("expected standard CAS rule for 160101, got (%q, %v)", got, ok)
	}

	pkg, err := mgr.BuildExportPackage(ctx)
	if err != nil {
		t.Fatalf("build export package: %v", err)
	}
	if len(pkg.Dimensions) < 4 {
		t.Fatalf("expected seeded dimensions in export package, got %d", len(pkg.Dimensions))
	}
	if len(pkg.MappingRules) == 0 {
		t.Fatal("expected seeded mapping rules in export package")
	}
}

func TestRepositoryHelperFunctionsCloneAndWindow(t *testing.T) {
	desc := "original"
	dimMeta := map[string]any{"k": "v"}
	parent := int64(7)
	updatedAt := fixedTime(t)
	clonedDim := cloneDimension(Dimension{
		Code:        "A",
		Description: &desc,
		Metadata:    dimMeta,
	})
	desc = "changed"
	clonedDim.Metadata["k"] = "x"
	if got := *clonedDim.Description; got != "original" {
		t.Fatalf("cloneDimension description = %q, want original", got)
	}
	if got := dimMeta["k"]; got != "v" {
		t.Fatalf("cloneDimension should isolate metadata map, got %v", got)
	}

	clonedMember := cloneMember(DimensionMember{Code: "M", ParentID: &parent, Metadata: map[string]any{"k": "v"}})
	parent = 9
	if clonedMember.ParentID == nil || *clonedMember.ParentID != 7 {
		t.Fatalf("cloneMember parent id = %+v, want 7", clonedMember.ParentID)
	}

	codePattern := "6001%"
	clonedRule := cloneMappingRule(MappingRule{
		AccountCodePattern: &codePattern,
		UpdatedAt:          &updatedAt,
	})
	codePattern = "changed"
	if clonedRule.AccountCodePattern == nil || *clonedRule.AccountCodePattern != "6001%" {
		t.Fatalf("cloneMappingRule account pattern = %+v", clonedRule.AccountCodePattern)
	}
	if clonedRule.UpdatedAt == nil || !clonedRule.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("cloneMappingRule updatedAt = %+v", clonedRule.UpdatedAt)
	}

	if got := removeID([]int64{1, 2, 3}, 2); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("removeID = %+v", got)
	}
	if start, end := normalizeWindow(10, 3, -2); start != 0 || end != 3 {
		t.Fatalf("normalizeWindow negative offset = (%d,%d)", start, end)
	}
	if start, end := normalizeWindow(10, 0, 4); start != 4 || end != 10 {
		t.Fatalf("normalizeWindow zero limit = (%d,%d)", start, end)
	}
	if !containsFold("HelloWorld", "world") {
		t.Fatal("containsFold should be case-insensitive")
	}

	original := map[string]any{"a": "b"}
	clonedMap := cloneMap(original)
	clonedMap["a"] = "c"
	if original["a"] != "b" {
		t.Fatalf("cloneMap should isolate top-level map, got %v", original["a"])
	}
}

func TestSQLiteRepositoryWorksWithManager(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "dimensions.sqlite")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	repo := NewSQLiteRepository(sqlDB)
	mgr := NewManager(repo)
	ctx := context.Background()

	dim, err := mgr.CreateDimension(ctx, CreateDimensionInput{
		Code:           "project",
		Name:           "Project",
		Type:           DimensionTypeProject,
		IsHierarchical: true,
		Metadata:       map[string]any{"source": "sqlite"},
	})
	if err != nil {
		t.Fatalf("create dimension: %v", err)
	}
	member, err := mgr.AddMember(ctx, AddMemberInput{
		DimensionID: dim.ID,
		Code:        "P001",
		Name:        "SaaS",
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	active := true
	if _, err := mgr.CreateMappingRule(ctx, CreateMappingRuleInput{
		Company:            "ACME",
		RuleName:           "Revenue",
		Priority:           10,
		AccountCodePattern: strPtr("6001%"),
		DimensionCode:      dim.Code,
		MemberCode:         member.Code,
		AllocationRatio:    1,
		IsActive:           &active,
	}); err != nil {
		t.Fatalf("create mapping rule: %v", err)
	}

	updatedName := "Project Updated"
	updated, err := mgr.UpdateDimension(ctx, dim.ID, DimensionPatch{Name: &updatedName})
	if err != nil {
		t.Fatalf("update dimension: %v", err)
	}
	if updated.Name != updatedName {
		t.Fatalf("dimension name = %q, want %q", updated.Name, updatedName)
	}

	sortOrder := 5
	updatedMember, err := mgr.UpdateMember(ctx, member.ID, MemberPatch{SortOrder: &sortOrder})
	if err != nil {
		t.Fatalf("update member: %v", err)
	}
	if updatedMember.SortOrder != sortOrder {
		t.Fatalf("member sort order = %d, want %d", updatedMember.SortOrder, sortOrder)
	}

	pkg, err := mgr.BuildExportPackage(ctx)
	if err != nil {
		t.Fatalf("build export package: %v", err)
	}
	if len(pkg.Dimensions) != 1 || len(pkg.Members[dim.Code]) != 1 || len(pkg.MappingRules) != 1 {
		t.Fatalf("unexpected export package: %+v", pkg)
	}
}

func fixedTime(t *testing.T) time.Time {
	t.Helper()
	return time.Unix(1700000000, 0).UTC()
}

func strPtr(v string) *string { return &v }
