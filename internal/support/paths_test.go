package support

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathHelpersResolveDefaultsAndProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir nested: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	wantRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		wantRoot = resolved
	}
	if got := FindProjectRoot(); got != wantRoot {
		t.Fatalf("project root = %q, want %q", got, wantRoot)
	}
	if got := DefaultUserConfigPath(root); got != filepath.Join(root, "config", "user_preferences.yaml") {
		t.Fatalf("default user config path = %q", got)
	}

	keywordsPath := filepath.Join(root, "config", "query_keywords.json")
	if err := os.MkdirAll(filepath.Dir(keywordsPath), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(keywordsPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write keywords: %v", err)
	}
	if got := DefaultKeywordsPath(root); got != keywordsPath {
		t.Fatalf("default keywords path = %q, want %q", got, keywordsPath)
	}
}

func TestDefaultDBPathUsesConfiguredPriority(t *testing.T) {
	t.Setenv("FINANCEQA_DB", "/tmp/financeqa.sqlite")
	t.Setenv("FINANCEQA_PG_DSN", "postgres://ignored")
	if got := DefaultDBPath(""); got != "/tmp/financeqa.sqlite" {
		t.Fatalf("DefaultDBPath explicit = %q", got)
	}

	t.Setenv("FINANCEQA_DB", "")
	t.Setenv("FINANCEQA_PG_DSN", "postgres://dsn")
	if got := DefaultDBPath(""); got != "postgres://dsn" {
		t.Fatalf("DefaultDBPath dsn = %q", got)
	}

	t.Setenv("FINANCEQA_PG_DSN", "")
	t.Setenv("PGHOST", "pg.example.com")
	t.Setenv("PGPORT", "")
	t.Setenv("PGUSER", "finance")
	t.Setenv("PGPASSWORD", "secret")
	t.Setenv("PGDATABASE", "bossagent")
	t.Setenv("FINANCEQA_PG_SCHEMA", "tenant_uhub")
	want := "host=pg.example.com port=5432 user=finance password=secret dbname=bossagent search_path=tenant_uhub,public"
	if got := DefaultDBPath(""); got != want {
		t.Fatalf("DefaultDBPath pg env = %q, want %q", got, want)
	}
}
