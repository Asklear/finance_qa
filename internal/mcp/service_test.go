package mcp

import (
	"context"
	"errors"
	"testing"
)

func TestServiceRejectsFinanceQueryWithoutQuery(t *testing.T) {
	t.Parallel()

	svc := NewService(ServiceConfig{
		DBPath:  "unused",
		Company: "测试公司",
	})
	_, err := svc.RunTool(context.Background(), "finance-query", map[string]any{})
	var toolErr *ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != -32602 {
		t.Fatalf("RunTool error = %#v, want -32602 ToolError", err)
	}
}

func TestServiceRejectsUnknownTool(t *testing.T) {
	t.Parallel()

	svc := NewService(ServiceConfig{})
	_, err := svc.RunTool(context.Background(), "missing-tool", map[string]any{})
	var toolErr *ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != -32602 {
		t.Fatalf("RunTool error = %#v, want -32602 ToolError", err)
	}
}
