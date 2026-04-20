package db

import "testing"

func TestEnsureSearchPathDoesNotInjectHardcodedSchemaWhenUnset(t *testing.T) {
	t.Setenv("FINANCEQA_PG_SCHEMA", "")

	dsn := "host=pg.example.com port=5432 user=finance password=secret dbname=bossagent"
	got := ensureSearchPath(dsn)
	if got != dsn {
		t.Fatalf("ensureSearchPath() = %q, want unchanged dsn %q", got, dsn)
	}
}

func TestEnsureSearchPathAppendsConfiguredSchema(t *testing.T) {
	t.Setenv("FINANCEQA_PG_SCHEMA", "tenant_demo")

	dsn := "host=pg.example.com port=5432 user=finance password=secret dbname=bossagent"
	want := dsn + " search_path=tenant_demo,public"
	if got := ensureSearchPath(dsn); got != want {
		t.Fatalf("ensureSearchPath() = %q, want %q", got, want)
	}
}
