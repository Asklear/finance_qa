package dimensions_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"financeqa/internal/db"
	"financeqa/internal/dimensions"
)

func TestManagerDimensionAndMemberLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := dimensions.NewMemoryRepository()
	mgr := dimensions.NewManager(repo)

	dim, err := mgr.CreateDimension(ctx, dimensions.CreateDimensionInput{
		Code:           "product",
		Name:           "Product",
		Type:           dimensions.DimensionTypeProduct,
		IsHierarchical: true,
	})
	if err != nil {
		t.Fatalf("create dimension: %v", err)
	}
	if dim.ID == 0 {
		t.Fatal("expected dimension ID to be set")
	}
	if !dim.IsActive || !dim.IsHierarchical {
		t.Fatalf("unexpected flags: active=%v hierarchical=%v", dim.IsActive, dim.IsHierarchical)
	}

	_, err = mgr.CreateDimension(ctx, dimensions.CreateDimensionInput{
		Code: "product",
		Name: "Duplicate",
		Type: dimensions.DimensionTypeProduct,
	})
	if !errors.Is(err, dimensions.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	root, err := mgr.AddMember(ctx, dimensions.AddMemberInput{
		DimensionID: dim.ID,
		Code:        "P001",
		Name:        "SaaS",
	})
	if err != nil {
		t.Fatalf("add root member: %v", err)
	}

	child, err := mgr.AddMember(ctx, dimensions.AddMemberInput{
		DimensionID: dim.ID,
		Code:        "P001A",
		Name:        "SaaS Enterprise",
		ParentID:    &root.ID,
	})
	if err != nil {
		t.Fatalf("add child member: %v", err)
	}
	if child.Level != 2 {
		t.Fatalf("expected child level=2, got %d", child.Level)
	}
	if child.Path != "P001/P001A" {
		t.Fatalf("unexpected child path: %q", child.Path)
	}

	tree, err := mgr.GetMemberTree(ctx, dim.ID, nil)
	if err != nil {
		t.Fatalf("get member tree: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("expected one root node, got %d", len(tree))
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("expected one child node, got %d", len(tree[0].Children))
	}
	if tree[0].IsLeaf {
		t.Fatal("expected root to be non-leaf")
	}

	typ := dimensions.DimensionTypeProduct
	listed, err := mgr.ListDimensions(ctx, dimensions.DimensionQueryOptions{Type: &typ})
	if err != nil {
		t.Fatalf("list dimensions: %v", err)
	}
	if listed.Total != 1 {
		t.Fatalf("expected total dimensions=1, got %d", listed.Total)
	}
}

func TestManagerMappingRulesAndExportPackage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := dimensions.NewMemoryRepository()
	mgr := dimensions.NewManager(repo)

	dim, err := mgr.CreateDimension(ctx, dimensions.CreateDimensionInput{
		Code: "project",
		Name: "Project",
		Type: dimensions.DimensionTypeProject,
	})
	if err != nil {
		t.Fatalf("create dimension: %v", err)
	}

	member, err := mgr.AddMember(ctx, dimensions.AddMemberInput{
		DimensionID: dim.ID,
		Code:        "PRJ001",
		Name:        "Core Platform",
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}

	active := true
	rule, err := mgr.CreateMappingRule(ctx, dimensions.CreateMappingRuleInput{
		Company:            "ACME",
		RuleName:           "SaaS Revenue",
		Priority:           20,
		AccountCodePattern: strPtr("6001%"),
		SummaryPattern:     strPtr("%SaaS%"),
		DimensionCode:      dim.Code,
		MemberCode:         member.Code,
		AllocationRatio:    1,
		IsActive:           &active,
	})
	if err != nil {
		t.Fatalf("create mapping rule: %v", err)
	}
	if rule.ID == 0 {
		t.Fatal("expected mapping rule ID to be set")
	}

	rules, err := mgr.ListMappingRules(ctx, dimensions.MappingRuleQueryOptions{Company: "ACME", IsActive: &active})
	if err != nil {
		t.Fatalf("list mapping rules: %v", err)
	}
	if rules.Total != 1 {
		t.Fatalf("expected one active rule, got %d", rules.Total)
	}

	inactive := false
	_, err = mgr.UpdateMappingRule(ctx, rule.ID, dimensions.MappingRulePatch{IsActive: &inactive})
	if err != nil {
		t.Fatalf("update mapping rule: %v", err)
	}

	rules, err = mgr.ListMappingRules(ctx, dimensions.MappingRuleQueryOptions{Company: "ACME", IsActive: &active})
	if err != nil {
		t.Fatalf("list active mapping rules: %v", err)
	}
	if rules.Total != 0 {
		t.Fatalf("expected no active rules after deactivation, got %d", rules.Total)
	}

	pkg, err := mgr.BuildExportPackage(ctx)
	if err != nil {
		t.Fatalf("build export package: %v", err)
	}
	if pkg.Version == "" {
		t.Fatal("expected export package version")
	}
	if len(pkg.Dimensions) != 1 {
		t.Fatalf("expected one exported dimension, got %d", len(pkg.Dimensions))
	}
	if got := pkg.Members[dim.Code]; len(got) != 1 {
		t.Fatalf("expected one exported member, got %d", len(got))
	}
	if len(pkg.MappingRules) != 1 {
		t.Fatalf("expected one exported mapping rule, got %d", len(pkg.MappingRules))
	}
	if pkg.MappingRules[0].IsActive {
		t.Fatal("expected exported mapping rule to reflect deactivation")
	}
}

func strPtr(v string) *string { return &v }

func TestManagerDimensionAndMemberLifecycleWithSQLiteRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newSQLiteRepositoryForTest(t)
	mgr := dimensions.NewManager(repo)

	dim, err := mgr.CreateDimension(ctx, dimensions.CreateDimensionInput{
		Code:           "product",
		Name:           "Product",
		Type:           dimensions.DimensionTypeProduct,
		IsHierarchical: true,
		Metadata:       map[string]any{"source": "sqlite-test"},
	})
	if err != nil {
		t.Fatalf("create dimension: %v", err)
	}
	if dim.ID == 0 {
		t.Fatal("expected dimension ID to be set")
	}

	_, err = mgr.CreateDimension(ctx, dimensions.CreateDimensionInput{
		Code: "product",
		Name: "Duplicate",
		Type: dimensions.DimensionTypeProduct,
	})
	if !errors.Is(err, dimensions.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	root, err := mgr.AddMember(ctx, dimensions.AddMemberInput{
		DimensionID: dim.ID,
		Code:        "P001",
		Name:        "SaaS",
	})
	if err != nil {
		t.Fatalf("add root member: %v", err)
	}

	child, err := mgr.AddMember(ctx, dimensions.AddMemberInput{
		DimensionID: dim.ID,
		Code:        "P001A",
		Name:        "SaaS Enterprise",
		ParentID:    &root.ID,
	})
	if err != nil {
		t.Fatalf("add child member: %v", err)
	}
	if child.Level != 2 {
		t.Fatalf("expected child level=2, got %d", child.Level)
	}
	if child.Path != "P001/P001A" {
		t.Fatalf("unexpected child path: %q", child.Path)
	}

	tree, err := mgr.GetMemberTree(ctx, dim.ID, nil)
	if err != nil {
		t.Fatalf("get member tree: %v", err)
	}
	if len(tree) != 1 || len(tree[0].Children) != 1 {
		t.Fatalf("unexpected tree shape: %+v", tree)
	}
}

func TestManagerMappingRulesAndExportPackageWithSQLiteRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newSQLiteRepositoryForTest(t)
	mgr := dimensions.NewManager(repo)

	dim, err := mgr.CreateDimension(ctx, dimensions.CreateDimensionInput{
		Code: "project",
		Name: "Project",
		Type: dimensions.DimensionTypeProject,
	})
	if err != nil {
		t.Fatalf("create dimension: %v", err)
	}

	member, err := mgr.AddMember(ctx, dimensions.AddMemberInput{
		DimensionID: dim.ID,
		Code:        "PRJ001",
		Name:        "Core Platform",
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}

	active := true
	rule, err := mgr.CreateMappingRule(ctx, dimensions.CreateMappingRuleInput{
		Company:            "ACME",
		RuleName:           "SaaS Revenue",
		Priority:           20,
		AccountCodePattern: strPtr("6001%"),
		SummaryPattern:     strPtr("%SaaS%"),
		DimensionCode:      dim.Code,
		MemberCode:         member.Code,
		AllocationRatio:    1,
		IsActive:           &active,
	})
	if err != nil {
		t.Fatalf("create mapping rule: %v", err)
	}
	if rule.ID == 0 {
		t.Fatal("expected mapping rule ID to be set")
	}

	rules, err := mgr.ListMappingRules(ctx, dimensions.MappingRuleQueryOptions{Company: "ACME", IsActive: &active})
	if err != nil {
		t.Fatalf("list mapping rules: %v", err)
	}
	if rules.Total != 1 {
		t.Fatalf("expected one active rule, got %d", rules.Total)
	}

	inactive := false
	_, err = mgr.UpdateMappingRule(ctx, rule.ID, dimensions.MappingRulePatch{IsActive: &inactive})
	if err != nil {
		t.Fatalf("update mapping rule: %v", err)
	}

	rules, err = mgr.ListMappingRules(ctx, dimensions.MappingRuleQueryOptions{Company: "ACME", IsActive: &active})
	if err != nil {
		t.Fatalf("list active mapping rules: %v", err)
	}
	if rules.Total != 0 {
		t.Fatalf("expected no active rules after deactivation, got %d", rules.Total)
	}

	pkg, err := mgr.BuildExportPackage(ctx)
	if err != nil {
		t.Fatalf("build export package: %v", err)
	}
	if len(pkg.Dimensions) != 1 {
		t.Fatalf("expected one exported dimension, got %d", len(pkg.Dimensions))
	}
	if got := pkg.Members[dim.Code]; len(got) != 1 {
		t.Fatalf("expected one exported member, got %d", len(got))
	}
	if len(pkg.MappingRules) != 1 {
		t.Fatalf("expected one exported mapping rule, got %d", len(pkg.MappingRules))
	}
	if pkg.MappingRules[0].IsActive {
		t.Fatal("expected exported mapping rule to reflect deactivation")
	}
}

func newSQLiteRepositoryForTest(t *testing.T) dimensions.Repository {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "dimensions.sqlite")
	if err := db.Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("bootstrap sqlite schema: %v", err)
	}
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return dimensions.NewSQLiteRepository(sqlDB)
}
