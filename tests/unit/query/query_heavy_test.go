package query_test

import "testing"

func skipHeavyQueryTest(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping heavy black-box query test in short mode")
	}
}

func runParallelHeavyQueryTest(t *testing.T) {
	t.Helper()
	skipHeavyQueryTest(t)
	t.Parallel()
}
