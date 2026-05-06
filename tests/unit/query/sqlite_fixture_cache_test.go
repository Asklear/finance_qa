package query_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var sqliteFixtureCache = struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	paths map[string]string
}{
	locks: map[string]*sync.Mutex{},
	paths: map[string]string{},
}

func cloneSQLiteFixture(t *testing.T, name string, build func(templatePath string)) string {
	t.Helper()

	templatePath := cachedSQLiteFixturePath(t, name, build)
	dst := filepath.Join(t.TempDir(), name+".db")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read cached sqlite fixture %s: %v", name, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("copy cached sqlite fixture %s: %v", name, err)
	}
	return dst
}

func cachedSQLiteFixturePath(t *testing.T, name string, build func(templatePath string)) string {
	t.Helper()

	sqliteFixtureCache.mu.Lock()
	if path := sqliteFixtureCache.paths[name]; path != "" {
		sqliteFixtureCache.mu.Unlock()
		return path
	}
	keyLock := sqliteFixtureCache.locks[name]
	if keyLock == nil {
		keyLock = &sync.Mutex{}
		sqliteFixtureCache.locks[name] = keyLock
	}
	sqliteFixtureCache.mu.Unlock()

	keyLock.Lock()
	defer keyLock.Unlock()

	sqliteFixtureCache.mu.Lock()
	if path := sqliteFixtureCache.paths[name]; path != "" {
		sqliteFixtureCache.mu.Unlock()
		return path
	}
	sqliteFixtureCache.mu.Unlock()

	dir, err := os.MkdirTemp("", "financeqa-test-fixture-"+name+"-")
	if err != nil {
		t.Fatalf("create sqlite fixture dir %s: %v", name, err)
	}
	templatePath := filepath.Join(dir, name+".db")
	build(templatePath)

	sqliteFixtureCache.mu.Lock()
	sqliteFixtureCache.paths[name] = templatePath
	sqliteFixtureCache.mu.Unlock()
	return templatePath
}
