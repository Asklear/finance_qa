package dimensions_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"financeqa/internal/db"
	"financeqa/internal/dimensions"

	_ "modernc.org/sqlite"
)

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

	repo := dimensions.NewSQLiteRepository(sqlDB)
	mgr := dimensions.NewManager(repo)
	ctx := context.Background()

	dim, err := mgr.CreateDimension(ctx, dimensions.CreateDimensionInput{
		Code: "product",
		Name: "Product",
		Type: dimensions.DimensionTypeProduct,
	})
	if err != nil {
		t.Fatalf("create dimension: %v", err)
	}
	member, err := mgr.AddMember(ctx, dimensions.AddMemberInput{
		DimensionID: dim.ID,
		Code:        "P001",
		Name:        "SaaS",
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	active := true
	_, err = mgr.CreateMappingRule(ctx, dimensions.CreateMappingRuleInput{
		Company:            "ACME",
		RuleName:           "Revenue",
		Priority:           10,
		AccountCodePattern: strPtr2("6001%"),
		DimensionCode:      dim.Code,
		MemberCode:         member.Code,
		AllocationRatio:    1,
		IsActive:           &active,
	})
	if err != nil {
		t.Fatalf("create mapping rule: %v", err)
	}

	gotDim, err := mgr.GetDimensionByCode(ctx, "product")
	if err != nil {
		t.Fatalf("get dimension by code: %v", err)
	}
	if gotDim.Name != "Product" {
		t.Fatalf("dimension name = %q", gotDim.Name)
	}

	pkg, err := mgr.BuildExportPackage(ctx)
	if err != nil {
		t.Fatalf("build export package: %v", err)
	}
	if len(pkg.Dimensions) != 1 || len(pkg.Members[dim.Code]) != 1 || len(pkg.MappingRules) != 1 {
		t.Fatalf("unexpected export package: %+v", pkg)
	}
}

func strPtr2(v string) *string { return &v }
