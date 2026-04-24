package query

import "testing"

func TestExecuteCounterpartyAnswerPipelineUsesFirstMatchingHandler(t *testing.T) {
	ctx := counterpartyAuditContext{entity: "测试对象"}
	calls := make([]string, 0, 3)

	handlers := []counterpartyAnswerHandler{
		func(_ *Engine, _ counterpartyAuditContext) (Result, bool) {
			calls = append(calls, "first")
			return Result{}, false
		},
		func(_ *Engine, _ counterpartyAuditContext) (Result, bool) {
			calls = append(calls, "second")
			return Result{Success: true, Message: "matched"}, true
		},
		func(_ *Engine, _ counterpartyAuditContext) (Result, bool) {
			calls = append(calls, "third")
			return Result{Success: true, Message: "late"}, true
		},
	}

	got := executeCounterpartyAnswerPipeline(nil, ctx, handlers, func(counterpartyAuditContext) Result {
		t.Fatalf("fallback should not run when a handler matched")
		return Result{}
	})

	if got.Message != "matched" {
		t.Fatalf("message = %q, want %q", got.Message, "matched")
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want first two handlers only", calls)
	}
}

func TestExecuteCounterpartyAnswerPipelineUsesFallbackWhenNoHandlerMatches(t *testing.T) {
	ctx := counterpartyAuditContext{entity: "测试对象"}
	calls := make([]string, 0, 2)

	handlers := []counterpartyAnswerHandler{
		func(_ *Engine, _ counterpartyAuditContext) (Result, bool) {
			calls = append(calls, "first")
			return Result{}, false
		},
		func(_ *Engine, _ counterpartyAuditContext) (Result, bool) {
			calls = append(calls, "second")
			return Result{}, false
		},
	}

	got := executeCounterpartyAnswerPipeline(nil, ctx, handlers, func(counterpartyAuditContext) Result {
		return Result{Success: true, Message: "fallback"}
	})

	if got.Message != "fallback" {
		t.Fatalf("message = %q, want %q", got.Message, "fallback")
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want all handlers before fallback", calls)
	}
}
