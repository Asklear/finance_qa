package testutil

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	dbpkg "financeqa/internal/db"
	"financeqa/internal/query"
	"financeqa/internal/support"
)

const DefaultBusinessCompany = "南京优集数据科技有限公司"

var liveDBGate = make(chan struct{}, liveDBParallelism())

func RunLiveDBCase(t testing.TB, fn func()) {
	t.Helper()

	liveDBGate <- struct{}{}
	t.Cleanup(func() {
		<-liveDBGate
	})
	fn()
}

func liveDBParallelism() int {
	raw := strings.TrimSpace(os.Getenv("FINANCEQA_LIVE_DB_PARALLELISM"))
	if raw == "" {
		return 2
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 2
	}
	if n > 8 {
		return 8
	}
	return n
}

var (
	liveEngineMu    sync.Mutex
	liveEngineCache = map[string]*query.Engine{}
)

func RequireRepoRoot(t testing.TB) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			t.Fatalf("repo root not found from working directory")
		}
		cwd = parent
	}
}

func RequireLiveDBConfig(t testing.TB) (string, string) {
	t.Helper()

	if os.Getenv("FINANCEQA_RUN_LIVE_DB_TESTS") != "1" {
		t.Skip("set FINANCEQA_RUN_LIVE_DB_TESTS=1 to run live database tests")
	}

	root := RequireRepoRoot(t)
	_ = support.LoadAppDotEnv(root)
	dbPath := support.DefaultDBPath(root)
	if dbPath == "" {
		t.Skip("database is not configured; skipping live database test")
	}
	return root, dbPath
}

func RequireLiveDBEngine(t testing.TB, company string) *query.Engine {
	t.Helper()

	_, dbPath := RequireLiveDBConfig(t)
	cacheKey := dbPath + "|" + company

	liveEngineMu.Lock()
	if engine := liveEngineCache[cacheKey]; engine != nil {
		liveEngineMu.Unlock()
		return engine
	}
	liveEngineMu.Unlock()

	engine, err := query.NewEngine(dbPath, company)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	liveEngineMu.Lock()
	if cached := liveEngineCache[cacheKey]; cached != nil {
		liveEngineMu.Unlock()
		engine.Close()
		return cached
	}
	liveEngineCache[cacheKey] = engine
	liveEngineMu.Unlock()

	return engine
}

func RequireLiveSQLDB(t testing.TB) *sql.DB {
	t.Helper()

	_, dbPath := RequireLiveDBConfig(t)
	db, err := dbpkg.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open configured database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
