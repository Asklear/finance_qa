package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"financeqa/internal/buildinfo"
)

// HTTPServerConfig configures the remote MCP HTTP transport.
type HTTPServerConfig struct {
	Addr       string
	ReadToken  string
	AdminToken string
	ToolRunner ToolRunner
	Service    *Service
	Version    string
}

// HTTPServer serves FinanceQA MCP over authenticated HTTP.
type HTTPServer struct {
	config HTTPServerConfig
	server *http.Server
}

// NewHTTPServer creates a remote MCP HTTP server.
func NewHTTPServer(config HTTPServerConfig) (*HTTPServer, error) {
	config.Addr = strings.TrimSpace(config.Addr)
	config.ReadToken = strings.TrimSpace(config.ReadToken)
	config.AdminToken = strings.TrimSpace(config.AdminToken)
	if config.Addr == "" {
		return nil, errors.New("listen address is required")
	}
	if config.ReadToken == "" && config.AdminToken == "" {
		return nil, errors.New("at least one MCP token is required")
	}
	if config.ToolRunner == nil {
		if config.Service == nil {
			config.Service = NewService(ServiceConfig{})
		}
		config.ToolRunner = config.Service
	}
	if config.Version == "" {
		config.Version = buildinfo.Version
	}

	s := &HTTPServer{config: config}
	s.server = &http.Server{
		Addr:    config.Addr,
		Handler: s.Handler(),
	}
	return s, nil
}

// Handler returns the HTTP handler for tests and embedding.
func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/mcp", s.handleMCP)
	return mux
}

// Start starts listening and serving in a background goroutine.
func (s *HTTPServer) Start() error {
	ln, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return fmt.Errorf("mcp listen: %w", err)
	}
	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			// Avoid logging headers or request bodies from this layer.
			fmt.Printf("[financeqa-mcp-http] server error: %v\n", err)
		}
	}()
	return nil
}

// ListenAndServe starts the HTTP server and blocks until it stops.
func (s *HTTPServer) ListenAndServe() error {
	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *HTTPServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scope, ok := s.scopeForRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONRPCError(w, nil, -32700, "Parse error", err.Error(), http.StatusBadRequest)
		return
	}

	switch req.Method {
	case "initialize":
		s.writeJSONRPCResponse(w, req.ID, InitializeResult{
			ProtocolVersion: "2025-03-26",
			ServerInfo: ServerInfo{
				Name:    "financeqa-mcp",
				Version: s.config.Version,
			},
			Capabilities: ServerCapabilities{
				Tools:     &ToolsCapability{},
				Resources: &ResourcesCapability{},
			},
		}, http.StatusOK)
	case "tools/list":
		s.writeJSONRPCResponse(w, req.ID, ToolsListResult{Tools: financeToolsForScope(scope)}, http.StatusOK)
	case "tools/call":
		s.handleHTTPToolsCall(w, r, scope, &req)
	default:
		s.writeJSONRPCError(w, req.ID, -32601, "Method not found", req.Method, http.StatusOK)
	}
}

func (s *HTTPServer) handleHTTPToolsCall(w http.ResponseWriter, r *http.Request, scope AuthScope, req *Request) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeJSONRPCError(w, req.ID, -32602, "Invalid params", err.Error(), http.StatusOK)
		return
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	if !ScopeAllowsTool(scope, params.Name, params.Arguments) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	result, err := s.config.ToolRunner.RunTool(r.Context(), params.Name, params.Arguments)
	if err != nil {
		if toolErr, ok := err.(*ToolError); ok {
			s.writeJSONRPCError(w, req.ID, toolErr.Code, toolErr.Message, toolErr.Data, http.StatusOK)
			return
		}
		s.writeJSONRPCError(w, req.ID, -32603, "Tool failed", err.Error(), http.StatusOK)
		return
	}
	operation := result.Operation
	if operation == "" {
		operation = params.Name
	}

	envelopeServer := NewServer()
	payload := envelopeServer.bridgeEnvelope(params.Name, operation, result.Payload)
	resultJSON, err := json.Marshal(payload)
	if err != nil {
		s.writeJSONRPCError(w, req.ID, -32603, "Failed to marshal result", err.Error(), http.StatusOK)
		return
	}

	s.writeJSONRPCResponse(w, req.ID, ToolResult{
		Content: []ContentItem{{Type: "text", Text: string(resultJSON)}},
	}, http.StatusOK)
}

func (s *HTTPServer) scopeForRequest(r *http.Request) (AuthScope, bool) {
	header := r.Header.Get("Authorization")
	if s.config.AdminToken != "" && ValidateBearerToken(header, s.config.AdminToken) {
		return ScopeAdmin, true
	}
	if s.config.ReadToken != "" && ValidateBearerToken(header, s.config.ReadToken) {
		return ScopeRead, true
	}
	return "", false
}

func (s *HTTPServer) writeJSONRPCResponse(w http.ResponseWriter, id interface{}, result any, status int) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeJSON(w, resp, status)
}

func (s *HTTPServer) writeJSONRPCError(w http.ResponseWriter, id interface{}, code int, message, data string, status int) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.writeJSON(w, resp, status)
}

func (s *HTTPServer) writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
