package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/ingest"
	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestFullPipelineNanjingYouji(t *testing.T) {
	// 1. Setup temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finance_test.db")

	ctx := context.Background()
	if err := db.Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("Failed to bootstrap DB: %v", err)
	}

	// 2. Initial Seeding (CAS Mapping)
	// Open dimensions manager
	manager, cleanup, err := openTestDimensionsManager(dbPath)
	if err != nil {
		t.Fatalf("Failed to open dimensions manager: %v", err)
	}
	defer cleanup()

	company := "南京优集数据科技有限公司"
	if err := manager.InitializeStandardRules(ctx, company); err != nil {
		t.Fatalf("Failed to initialize standard rules: %v", err)
	}

	// 3. Import Data
	importer := ingest.NewImporter(manager)
	// Import Profit Statement (南京优集2026.2利润表.xls)
	profitFile := findProjectFile("南京优集2026.2利润表.xls")
	if profitFile == "" {
		t.Skip("Profit statement sample not found in workspace")
	}

	_, err = importer.ImportFile(ctx, dbPath, profitFile, false)
	if err != nil {
		t.Fatalf("Failed to import profit statement: %v", err)
	}

	// 4. Query and Verify Observability
	engine, err := query.NewEngine(dbPath, company)
	if err != nil {
		t.Fatalf("Failed to create query engine: %v", err)
	}
	defer engine.Close()

	// Query for monthly summary
	res := engine.Query("2026年2月的支出是多少")
	if !res.Success {
		t.Fatalf("Query failed: %s", res.Message)
	}

	// Verify observability fields
	if len(res.ExecutedSQL) == 0 {
		t.Errorf("ExecutedSQL should not be empty")
	}
	if len(res.CalculationLogs) == 0 {
		t.Errorf("CalculationLogs should not be empty")
	}

	// Verify data presence
	if res.Data["财务做账口径(看利润)"] == nil {
		t.Errorf("Accrual perspective data missing")
	}

	fmt.Printf("Query Result: %s\n", res.Message)
	for _, sql := range res.ExecutedSQL {
		fmt.Printf("SQL Trace: %s\n", sql)
	}
	for _, log := range res.CalculationLogs {
		fmt.Printf("Calc Log: %s\n", log)
	}
}

func TestNonStandardMapping(t *testing.T) {
	// 1. Setup temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finance_nonstandard.db")

	ctx := context.Background()
	if err := db.Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("Failed to bootstrap DB: %v", err)
	}

	company := "非标科技有限公司"
	manager, cleanup, err := openTestDimensionsManager(dbPath)
	if err != nil {
		t.Fatalf("Failed to open dimensions manager: %v", err)
	}
	defer cleanup()

	// 2. Setup Non-standard Mapping
	// In CAS, Revenue is 6001. Here we map 5101 (usually Cost) to Revenue.
	if err := manager.InitializeStandardRules(ctx, company); err != nil {
		t.Fatalf("Failed to initialize standard rules: %v", err)
	}
	_, err = manager.CreateMappingRule(ctx, dimensions.CreateMappingRuleInput{
		Company:            company,
		RuleName:           "非标收入映射",
		Priority:           200, // Higher than default
		AccountCodePattern: pointer("5101%"),
		DimensionCode:      "CAS",
		MemberCode:         "6001", // Standard Revenue
	})
	if err != nil {
		t.Fatalf("Failed to create mapping rule: %v", err)
	}

	// 3. Insert Non-standard Journal Data
	sqlDB, _ := sql.Open("sqlite", dbPath)
	_, err = sqlDB.Exec(`
INSERT INTO journal (company, voucher_date, account_code, account_name, direction, amount, summary)
VALUES (?, '2026-02-15', '5101', '非标确认收入', '贷', 10000.0, '销售商品')`, company)
	if err != nil {
		t.Fatalf("Failed to insert test journal: %v", err)
	}
	sqlDB.Close()

	// 4. Query and Verify
	engine, err := query.NewEngine(dbPath, company)
	if err != nil {
		t.Fatalf("Failed to create query engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年2月的收入是多少")
	if !res.Success {
		t.Fatalf("Query failed: %s", res.Message)
	}

	// Logic: 5101 is normally Expense, but our rule maps it to 6001 (Revenue).
	// So Revenue should be 10000.
	data := res.Data["财务做账口径(看利润)"].(map[string]any)
	revenue := data["营业收入"].(float64)
	if revenue != 10000.0 {
		t.Errorf("Expected revenue 10000.0, got %.2f", revenue)
	}

	// Check logs
	foundLog := false
	for _, log := range res.CalculationLogs {
		if strings.Contains(log, "5101 匹配到映射规则 -> 6001") {
			foundLog = true
			break
		}
	}
	if !foundLog {
		t.Errorf("Mapping log not found in calculation_logs")
	}
}

func TestGranularAccountHierarchy(t *testing.T) {
	// 1. Setup temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finance_granular.db")

	ctx := context.Background()
	if err := db.Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("Failed to bootstrap DB: %v", err)
	}

	company := "南京优集数据科技有限公司"
	manager, cleanup, err := openTestDimensionsManager(dbPath)
	if err != nil {
		t.Fatalf("Failed to open dimensions manager: %v", err)
	}
	defer cleanup()

	// 2. Initialize Standard Rules
	if err := manager.InitializeStandardRules(ctx, company); err != nil {
		t.Fatalf("Failed to initialize standard rules: %v", err)
	}

	// 3. Verify Hierarchy in Dimension Members
	dim, _ := manager.GetDimensionByCode(ctx, "CAS")
	res, _ := manager.ListMembers(ctx, dimensions.MemberQueryOptions{DimensionID: &dim.ID})
	members := res.Data

	var foundParent, foundChild bool
	var parentID int64
	for _, m := range members {
		if m.Code == "1601" {
			foundParent = true
			parentID = m.ID
		}
	}
	if !foundParent {
		t.Fatalf("Parent account 1601 not found")
	}

	for _, m := range members {
		if m.Code == "160101" {
			foundChild = true
			if m.ParentID == nil || *m.ParentID != parentID {
				t.Errorf("Child 160101 has wrong parent ID: %v, want %d", m.ParentID, parentID)
			}
		}
	}
	if !foundChild {
		t.Fatalf("Child account 160101 not found")
	}

	// 4. Verify Mapping Rule Priority
	ruleRes, _ := manager.ListMappingRules(ctx, dimensions.MappingRuleQueryOptions{Company: company})
	rules := ruleRes.Data
	var rule1601, rule160101 *dimensions.MappingRule
	for i := range rules {
		if rules[i].MemberCode == "1601" {
			rule1601 = &rules[i]
		}
		if rules[i].MemberCode == "160101" {
			rule160101 = &rules[i]
		}
	}

	if rule1601 == nil || rule160101 == nil {
		t.Fatalf("Mapping rules for 1601/160101 not found")
	}

	if rule160101.Priority <= rule1601.Priority {
		t.Errorf("Level 2 rule priority (%d) should be > Level 1 priority (%d)", rule160101.Priority, rule1601.Priority)
	}
}

func pointer(s string) *string {
	return &s
}

func openTestDimensionsManager(dbPath string) (*dimensions.Manager, func(), error) {
	sqlDB, err := db.Open(context.Background(), dbPath)
	if err != nil {
		return nil, nil, err
	}
	repo := dimensions.NewSQLiteRepository(sqlDB)
	return dimensions.NewManager(repo), func() { _ = sqlDB.Close() }, nil
}

func findProjectFile(name string) string {
	cwd, _ := os.Getwd()
	// Try up to 3 levels up to find the project root
	dir := cwd
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		dir = filepath.Dir(dir)
	}
	return ""
}
