package query_test

import (
	"strings"
	"testing"

	"financeqa/internal/query"
)

type queryScenario struct {
	Name     string
	Question string
	DBPath   func(*testing.T) string
	Seed     func(*testing.T, string)
	Company  string
	Assert   func(*testing.T, query.Result)
}

func runParallelQueryScenarios(t *testing.T, scenarios []queryScenario) {
	t.Helper()

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			runParallelHeavyQueryTest(t)

			company := scenario.Company
			if company == "" {
				company = testCompany
			}
			dbPath := scenario.DBPath(t)
			if scenario.Seed != nil {
				scenario.Seed(t, dbPath)
			}
			engine, err := query.NewEngine(dbPath, company)
			if err != nil {
				t.Fatalf("new engine: %v", err)
			}
			defer engine.Close()

			res := engine.Query(scenario.Question)
			if !res.Success {
				t.Fatalf("query failed: %+v", res)
			}
			if scenario.Assert != nil {
				scenario.Assert(t, res)
			}
		})
	}
}

func assertCashBeforeFinancialView(t *testing.T, message string) {
	t.Helper()

	if !strings.Contains(message, "现金口径") || !strings.Contains(message, "财务口径") {
		t.Fatalf("message should mention cash and financial views, got: %s", message)
	}
	if strings.Index(message, "现金口径") > strings.Index(message, "财务口径") {
		t.Fatalf("contract answer should present cash view before financial view, got: %s", message)
	}
}

func assertViewAliases(t *testing.T, res query.Result) {
	t.Helper()

	if _, ok := res.Data["money_view"]; !ok {
		t.Fatalf("missing money_view alias: %+v", res.Data)
	}
	if _, ok := res.Data["account_view"]; !ok {
		t.Fatalf("missing account_view alias: %+v", res.Data)
	}
}
