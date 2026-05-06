.PHONY: test test-fast test-unit test-query-heavy test-integration test-business test-live test-full

GO_TEST ?= go test
PKGS_FAST := ./internal/... ./tests/unit/... ./tests/testutil

test: test-fast

test-fast:
	$(GO_TEST) -short -p 8 -parallel 8 $(PKGS_FAST) -count=1

test-unit:
	$(GO_TEST) -p 8 -parallel 8 ./tests/unit/... -count=1

test-query-heavy:
	$(GO_TEST) -p 8 -parallel 8 ./tests/unit/query -count=1

test-integration:
	$(GO_TEST) -p 4 -parallel 4 ./tests/integration -count=1

test-business:
	FINANCEQA_RUN_LIVE_DB_TESTS=1 $(GO_TEST) -tags accuracy -p 4 -parallel 8 ./tests/business -count=1

test-live:
	FINANCEQA_RUN_LIVE_DB_TESTS=1 $(GO_TEST) -p 4 -parallel 4 ./tests/integration -count=1

test-full:
	$(GO_TEST) -p 8 -parallel 8 ./... -count=1
