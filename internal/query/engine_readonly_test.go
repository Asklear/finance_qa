package query

import (
	"context"
	"path/filepath"
	"testing"

	dbpkg "financeqa/internal/db"
)

func TestNewReadOnlyEngineOpensBootstrappedDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "readonly-engine.sqlite")
	if err := dbpkg.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	engine, err := NewReadOnlyEngine(dbPath, "DefaultCompany")
	if err != nil {
		t.Fatalf("NewReadOnlyEngine: %v", err)
	}
	defer engine.Close()

	if engine.db == nil {
		t.Fatal("NewReadOnlyEngine should open a database connection")
	}
}
