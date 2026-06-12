// Package mcp implements the Model Context Protocol server for financeqa.
// Allows OpenClaw/Cursor to call financeqa directly as an MCP tool without the Python bridge.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"financeqa/internal/buildinfo"
	"financeqa/internal/support"
)

// Server implements an MCP server over stdio.
type Server struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	dbPath       string
	company      string
	skillPath    string
	appendixPath string
	toolRunner   ToolRunner
}

// ToolRunResult is the native payload produced by an MCP tool implementation.
type ToolRunResult struct {
	Operation string
	Payload   any
}

// ToolRunner allows tests and embedders to provide tool behavior while still
// using the production MCP framing and response envelope.
type ToolRunner interface {
	RunTool(ctx context.Context, name string, args map[string]any) (ToolRunResult, error)
}

// ToolError maps a tool failure to a JSON-RPC error response.
type ToolError struct {
	Code    int
	Message string
	Data    string
}

func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Data == "" {
		return e.Message
	}
	return e.Message + ": " + e.Data
}

// ServerOption configures the server.
type ServerOption func(*Server)

// WithDBPath sets the database path.
func WithDBPath(path string) ServerOption {
	return func(s *Server) { s.dbPath = path }
}

// WithCompany sets the default company name.
func WithCompany(name string) ServerOption {
	return func(s *Server) { s.company = name }
}

// WithSkillPath sets the skill markdown file path.
func WithSkillPath(path string) ServerOption {
	return func(s *Server) { s.skillPath = path }
}

// WithAppendixPath sets the skill appendix markdown file path.
func WithAppendixPath(path string) ServerOption {
	return func(s *Server) { s.appendixPath = path }
}

// WithToolRunner sets a custom tool runner.
func WithToolRunner(runner ToolRunner) ServerOption {
	return func(s *Server) { s.toolRunner = runner }
}

// WithIO sets the input/output streams.
func WithIO(stdin io.Reader, stdout, stderr io.Writer) ServerOption {
	return func(s *Server) {
		s.stdin = stdin
		s.stdout = stdout
		s.stderr = stderr
	}
}

// NewServer creates a new MCP server.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		dbPath: defaultDBTarget(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run starts the MCP server and processes requests.
func (s *Server) Run(ctx context.Context) error {
	// Send initialization notification
	if err := s.sendNotification("notifications/initialized", nil); err != nil {
		return fmt.Errorf("send init notification: %w", err)
	}

	decoder := json.NewDecoder(s.stdin)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var req Request
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode request: %w", err)
		}

		if err := s.handleRequest(ctx, &req); err != nil {
			s.logError("handle request %s: %v", req.ID, err)
			// Send error response but continue
			s.sendErrorResponse(req.ID, -32603, "Internal error", err.Error())
		}
	}
}

func (s *Server) handleRequest(ctx context.Context, req *Request) error {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	default:
		return s.sendErrorResponse(req.ID, -32601, "Method not found", req.Method)
	}
}

func (s *Server) handleInitialize(req *Request) error {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo: ServerInfo{
			Name:    "financeqa-mcp",
			Version: s.loadVersion(),
		},
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
		},
	}
	return s.sendResponse(req.ID, result)
}

func (s *Server) logError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(s.stderr, "[financeqa-mcp] %s\n", support.SanitizeDSN(msg))
}

func (s *Server) loadVersion() string {
	// Try to get from environment or fallback to default
	if v := os.Getenv("FINANCEQA_VERSION"); v != "" {
		return v
	}
	return buildinfo.Version
}

// Helpers

func defaultDBTarget() string {
	if dsn := os.Getenv("FINANCEQA_DB"); dsn != "" {
		return dsn
	}
	// Try to construct from PG environment variables
	host := os.Getenv("PGHOST")
	port := os.Getenv("PGPORT")
	user := os.Getenv("PGUSER")
	pass := os.Getenv("PGPASSWORD")
	dbname := os.Getenv("PGDATABASE")
	schema := os.Getenv("FINANCEQA_PG_SCHEMA")

	if host != "" && user != "" && dbname != "" {
		if port == "" {
			port = "5432"
		}
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s",
			host, port, user, pass, dbname)
		if schema != "" {
			dsn += fmt.Sprintf(" search_path=%s,public", schema)
		}
		return dsn
	}

	return ""
}

// Auto-detect paths
func AutoDetectPaths() (skillPath, appendixPath string) {
	// Try environment variables first
	skillPath = os.Getenv("FINANCEQA_SKILL_PATH")
	appendixPath = os.Getenv("FINANCEQA_APPENDIX_PATH")

	if skillPath != "" && appendixPath != "" {
		return
	}

	// Try to find relative to binary or working directory
	candidates := []string{
		"SKILL.md",
		"../SKILL.md",
		"../../SKILL.md",
		"/usr/share/financeqa/SKILL.md",
	}

	for _, c := range candidates {
		if skillPath == "" {
			if _, err := os.Stat(c); err == nil {
				skillPath = c
				if appendixPath == "" {
					// Try to find appendix relative to skill
					skillDir := filepath.Dir(c)
					appendixCandidates := []string{
						filepath.Join(skillDir, "docs", "SKILL_APPENDIX_FULL.md"),
						filepath.Join(skillDir, "SKILL_APPENDIX_FULL.md"),
					}
					for _, ac := range appendixCandidates {
						if _, err := os.Stat(ac); err == nil {
							appendixPath = ac
							break
						}
					}
				}
				break
			}
		}
	}

	return
}

// Ensure Server implements the right interfaces
var _ = (*Server)(nil)
