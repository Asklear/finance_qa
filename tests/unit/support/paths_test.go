package support_test

import (
	"testing"

	"financeqa/internal/support"
)

func TestDefaultDBPathReturnsEmptyWhenNoDatabaseConfigured(t *testing.T) {
	t.Setenv("FINANCEQA_DB", "")
	t.Setenv("FINANCEQA_PG_DSN", "")
	t.Setenv("PGHOST", "")
	t.Setenv("PGPORT", "")
	t.Setenv("PGUSER", "")
	t.Setenv("PGPASSWORD", "")
	t.Setenv("PGDATABASE", "")
	t.Setenv("FINANCEQA_PG_SCHEMA", "")

	if got := support.DefaultDBPath(""); got != "" {
		t.Fatalf("DefaultDBPath() = %q, want empty", got)
	}
}

func TestDefaultDBPathPrefersExplicitTarget(t *testing.T) {
	t.Setenv("FINANCEQA_DB", "/tmp/custom.sqlite")
	t.Setenv("FINANCEQA_PG_DSN", "host=ignored dbname=ignored")
	t.Setenv("PGHOST", "ignored")
	t.Setenv("PGUSER", "ignored")
	t.Setenv("PGDATABASE", "ignored")

	if got := support.DefaultDBPath(""); got != "/tmp/custom.sqlite" {
		t.Fatalf("DefaultDBPath() = %q, want explicit FINANCEQA_DB", got)
	}
}

func TestDefaultDBPathBuildsPostgresDSNFromEnv(t *testing.T) {
	t.Setenv("FINANCEQA_DB", "")
	t.Setenv("FINANCEQA_PG_DSN", "")
	t.Setenv("PGHOST", "pg.example.com")
	t.Setenv("PGPORT", "5433")
	t.Setenv("PGUSER", "finance")
	t.Setenv("PGPASSWORD", "secret")
	t.Setenv("PGDATABASE", "bossagent")
	t.Setenv("FINANCEQA_PG_SCHEMA", "tenant_demo")

	want := "host=pg.example.com port=5433 user=finance password=secret dbname=bossagent search_path=tenant_demo,public"
	if got := support.DefaultDBPath(""); got != want {
		t.Fatalf("DefaultDBPath() = %q, want %q", got, want)
	}
}
