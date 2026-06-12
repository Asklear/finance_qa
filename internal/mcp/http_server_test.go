package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPMCPHealthAllowsUnauthenticated(t *testing.T) {
	t.Parallel()

	server := newTestHTTPServer(t, &recordingToolRunner{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/health status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTPMCPRejectsMissingOrBadBearerToken(t *testing.T) {
	t.Parallel()

	server := newTestHTTPServer(t, &recordingToolRunner{})

	for _, tt := range []struct {
		name  string
		token string
	}{
		{name: "missing token", token: ""},
		{name: "bad token", token: "bad-token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := postMCP(t, server.Handler(), tt.token, initializeRequestJSON(1))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHTTPMCPInitializeWithReadToken(t *testing.T) {
	t.Parallel()

	server := newTestHTTPServer(t, &recordingToolRunner{})

	rec := postMCP(t, server.Handler(), "read-token", initializeRequestJSON(1))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	response := decodeHTTPMCPResponse(t, rec.Body.Bytes())
	result := response["result"].(map[string]any)
	serverInfo := result["serverInfo"].(map[string]any)
	if serverInfo["name"] != "financeqa-mcp" {
		t.Fatalf("serverInfo.name = %v, want financeqa-mcp", serverInfo["name"])
	}
}

func TestHTTPMCPToolsListFiltersReadScope(t *testing.T) {
	t.Parallel()

	server := newTestHTTPServer(t, &recordingToolRunner{})

	rec := postMCP(t, server.Handler(), "read-token", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	response := decodeHTTPMCPResponse(t, rec.Body.Bytes())
	result := response["result"].(map[string]any)
	tools := result["tools"].([]any)
	names := map[string]bool{}
	for _, item := range tools {
		tool := item.(map[string]any)
		names[tool["name"].(string)] = true
	}
	for _, allowed := range []string{"finance-query", "finance-host-data", "finance-dimensions"} {
		if !names[allowed] {
			t.Fatalf("read tools missing %s: %#v", allowed, names)
		}
	}
	for _, denied := range []string{"finance-upload", "finance-sync"} {
		if names[denied] {
			t.Fatalf("read tools should not include %s: %#v", denied, names)
		}
	}
}

func TestHTTPMCPReadScopeRejectsWriteToolBeforeExecution(t *testing.T) {
	t.Parallel()

	runner := &recordingToolRunner{}
	server := newTestHTTPServer(t, runner)

	rec := postMCP(t, server.Handler(), "read-token", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"finance-sync","arguments":{"directory":"/tmp"}}}`)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if runner.name != "" {
		t.Fatalf("runner should not be called for forbidden tool, got %s", runner.name)
	}
}

func TestHTTPMCPAdminScopeListsAllTools(t *testing.T) {
	t.Parallel()

	server := newTestHTTPServer(t, &recordingToolRunner{})

	rec := postMCP(t, server.Handler(), "admin-token", `{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{}}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	response := decodeHTTPMCPResponse(t, rec.Body.Bytes())
	result := response["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 5 {
		t.Fatalf("admin tools length = %d, want 5", len(tools))
	}
}

func TestHTTPMCPToolCallPreservesBridgeEnvelope(t *testing.T) {
	t.Parallel()

	runner := &recordingToolRunner{
		result: ToolRunResult{
			Operation: "query",
			Payload: map[string]any{
				"success": true,
				"message": "2026-03 营收 123.45 元。",
				"data": map[string]any{
					"final_answer":       "2026-03 营收 123.45 元。",
					"source_note":        "来源：《优集收入、成本计算表 - 上传.xlsx》",
					"source_update_note": "来源更新时间：2026-05-05 20:46:23",
				},
			},
		},
	}
	server := newTestHTTPServer(t, runner)

	rec := postMCP(t, server.Handler(), "read-token", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"finance-query","arguments":{"query":"2026年3月营收"}}}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if runner.name != "finance-query" || runner.args["query"] != "2026年3月营收" {
		t.Fatalf("runner saw name=%q args=%#v", runner.name, runner.args)
	}
	response := decodeHTTPMCPResponse(t, rec.Body.Bytes())
	result := response["result"].(map[string]any)
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode payload: %v; text=%s", err, text)
	}
	finalAnswer, _ := payload["final_answer"].(string)
	for _, want := range []string{"2026-03 营收 123.45 元", "来源：《优集收入、成本计算表 - 上传.xlsx》", "来源更新时间：2026-05-05 20:46:23"} {
		if !strings.Contains(finalAnswer, want) {
			t.Fatalf("final_answer should contain %q, got %s", want, finalAnswer)
		}
	}
}

func TestNewHTTPServerRejectsEmptyTokenConfig(t *testing.T) {
	t.Parallel()

	_, err := NewHTTPServer(HTTPServerConfig{
		Addr:       "127.0.0.1:0",
		ToolRunner: &recordingToolRunner{},
	})
	if err == nil {
		t.Fatalf("NewHTTPServer should reject missing token config")
	}
}

func newTestHTTPServer(t *testing.T, runner ToolRunner) *HTTPServer {
	t.Helper()

	server, err := NewHTTPServer(HTTPServerConfig{
		Addr:       "127.0.0.1:0",
		ReadToken:  "read-token",
		AdminToken: "admin-token",
		ToolRunner: runner,
		Version:    "test-version",
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	return server
}

func postMCP(t *testing.T, handler http.Handler, token string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func initializeRequestJSON(id int) string {
	return `{"jsonrpc":"2.0","id":` + jsonNumber(id) + `,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}`
}

func jsonNumber(id int) string {
	raw, _ := json.Marshal(id)
	return string(raw)
}

func decodeHTTPMCPResponse(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	trimmed := bytes.TrimSpace(raw)
	if bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:")) {
		for _, line := range bytes.Split(trimmed, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if payload, ok := bytes.CutPrefix(line, []byte("data:")); ok {
				trimmed = bytes.TrimSpace(payload)
				break
			}
		}
	}

	var response map[string]any
	if err := json.Unmarshal(trimmed, &response); err != nil {
		t.Fatalf("decode HTTP MCP response: %v; body=%s", err, string(raw))
	}
	return response
}

var _ ToolRunner = (*recordingToolRunner)(nil)
