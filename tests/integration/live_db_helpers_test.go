package integration

import (
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/query"
	"financeqa/internal/support"
)

func requireLiveDBConfig(t *testing.T) (string, string) {
	t.Helper()
	if os.Getenv("FINANCEQA_RUN_LIVE_DB_TESTS") != "1" {
		t.Skip("set FINANCEQA_RUN_LIVE_DB_TESTS=1 to run live database suites")
	}

	cwd, _ := os.Getwd()
	root := filepath.Join(cwd, "..", "..")
	_ = support.LoadAppDotEnv(root)
	dbPath := support.DefaultDBPath(root)
	if dbPath == "" {
		t.Skip("database is not configured; skipping live database suite")
	}
	return root, dbPath
}

func requireLiveDBEngine(t *testing.T, company string) *query.Engine {
	t.Helper()
	_, dbPath := requireLiveDBConfig(t)
	engine, err := query.NewEngine(dbPath, company)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() {
		engine.Close()
	})
	return engine
}
