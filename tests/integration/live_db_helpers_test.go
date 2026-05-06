package integration

import (
	"testing"

	"financeqa/internal/query"
	"financeqa/tests/testutil"
)

func requireLiveDBConfig(t *testing.T) (string, string) {
	t.Helper()
	return testutil.RequireLiveDBConfig(t)
}

func requireLiveDBEngine(t *testing.T, company string) *query.Engine {
	t.Helper()
	return testutil.RequireLiveDBEngine(t, company)
}
