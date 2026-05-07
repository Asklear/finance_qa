// Package mcp implements the Model Context Protocol server for financeqa.
// Allows OpenClaw/Cursor to call financeqa directly as an MCP tool without the Python bridge.
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/ingest"
	"financeqa/internal/query"
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

func (s *Server) handleFinanceQuery(ctx context.Context, id interface{}, args map[string]any) error {
	queryStr, _ := args["query"].(string)
	if queryStr == "" {
		return s.sendErrorResponse(id, -32602, "Missing required argument", "query")
	}

	engine, err := query.NewEngine(s.dbPath, s.company)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to create query engine", err.Error())
	}
	defer engine.Close()

	result := engine.Query(queryStr)

	// Return result as tool response
	return s.sendToolResponse(id, "finance-query", "query", result)
}

func (s *Server) handleFinanceHostData(ctx context.Context, id interface{}, args map[string]any) error {
	queryStr, _ := args["query"].(string)
	from, _ := args["from"].(string)
	to, _ := args["to"].(string)

	if queryStr == "" {
		queryStr = "输出全量财报原始数据给宿主LLM"
	}

	engine, err := query.NewEngine(s.dbPath, s.company)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to create query engine", err.Error())
	}
	defer engine.Close()

	result := engine.HostLLMPayload(from, to, queryStr)
	return s.sendToolResponse(id, "finance-host-data", "host-data", result)
}

func (s *Server) handleFinanceUpload(ctx context.Context, id interface{}, args map[string]any) error {
	filePath, _ := args["file"].(string)
	if filePath == "" {
		return s.sendErrorResponse(id, -32602, "Missing required argument", "file")
	}

	importer, err := s.newImporter(ctx)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to open database", err.Error())
	}
	defer importer.close()

	summary, err := importer.ingest.ImportFileWithOptions(ctx, s.dbPath, filePath, importOptionsFromArgs(args))
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Import failed", err.Error())
	}

	return s.sendToolResponse(id, "finance-upload", "upload", summary)
}

func (s *Server) handleFinanceSync(ctx context.Context, id interface{}, args map[string]any) error {
	dirPath, _ := args["directory"].(string)
	if dirPath == "" {
		return s.sendErrorResponse(id, -32602, "Missing required argument", "directory")
	}

	importer, err := s.newImporter(ctx)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to open database", err.Error())
	}
	defer importer.close()

	summary, err := importer.ingest.SyncDirectoryWithOptions(ctx, s.dbPath, dirPath, importOptionsFromArgs(args))
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Sync failed", err.Error())

	}

	return s.sendToolResponse(id, "finance-sync", "sync", summary)
}

type mcpImporter struct {
	ingest *ingest.Importer
	db     *sql.DB
}

func (s *Server) newImporter(ctx context.Context) (*mcpImporter, error) {
	dbConn, err := db.Open(ctx, s.dbPath)
	if err != nil {
		return nil, err
	}
	manager := dimensions.NewManager(dimensions.NewSQLiteRepository(dbConn))
	return &mcpImporter{
		ingest: ingest.NewImporter(manager),
		db:     dbConn,
	}, nil
}

func (i *mcpImporter) close() {
	if i != nil && i.db != nil {
		i.db.Close()
	}
}

func importOptionsFromArgs(args map[string]any) ingest.ImportOptions {
	company, _ := args["company"].(string)
	incremental, _ := args["incremental"].(bool)
	return ingest.ImportOptions{
		Incremental:     incremental,
		CompanyOverride: company,
	}
}

func (s *Server) handleFinanceDimensions(ctx context.Context, id interface{}, args map[string]any) error {
	action, _ := args["action"].(string)

	dbConn, err := db.Open(ctx, s.dbPath)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to open database", err.Error())
	}
	defer dbConn.Close()

	manager := dimensions.NewManager(dimensions.NewSQLiteRepository(dbConn))

	switch action {
	case "list":
		opts := dimensions.DimensionQueryOptions{
			Limit: 100,
		}
		result, err := manager.ListDimensions(ctx, opts)
		if err != nil {
			return s.sendErrorResponse(id, -32603, "Failed to list dimensions", err.Error())
		}
		return s.sendToolResponse(id, "finance-dimensions", "dimensions:list", result)

	default:
		return s.sendErrorResponse(id, -32602, "Unknown dimensions action", action)
	}
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
	return "2.0.0"
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
