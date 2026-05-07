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

	dbPath         string
	company        string
	skillPath      string
	appendixPath   string
	financeqaBin   string
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

// WithFinanceQABin sets the financeqa binary path (for self-reference).
func WithFinanceQABin(path string) ServerOption {
	return func(s *Server) { s.financeqaBin = path }
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
			Tools:   &ToolsCapability{},
			Resources: &ResourcesCapability{},
		},
	}
	return s.sendResponse(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) error {
	result := ToolsListResult{
		Tools: []Tool{
			{
				Name:        "finance-query",
				Description: "Query financial data using natural language. Supports revenue, cost, profit, AR/AP, receipts, payments, and contract dimension queries.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Natural language query in Chinese, e.g., '2026年3月应收账款有多少' or '金程科技的收入是多少'",
						},
					},
					"required": []string{"query"},
				},
			},
			{
				Name:        "finance-host-data",
				Description: "Provide full financial data payload to host LLM for complex reasoning when direct query fails or ambiguous.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Context question or data request",
						},
						"from": map[string]any{
							"type":        "string",
							"description": "Period start in YYYY-MM format",
						},
						"to": map[string]any{
							"type":        "string",
							"description": "Period end in YYYY-MM format",
						},
					},
					"required": []string{},
				},
			},
			{
				Name:        "finance-upload",
				Description: "Import a single financial Excel file (income statement, balance sheet, journal, etc.)",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file": map[string]any{
							"type":        "string",
							"description": "Absolute path to the Excel file to import",
						},
						"company": map[string]any{
							"type":        "string",
							"description": "Override company name for this file",
						},
						"incremental": map[string]any{
							"type":        "boolean",
							"description": "Incremental import (don't clear existing data)",
						},
					},
					"required": []string{"file"},
				},
			},
			{
				Name:        "finance-sync",
				Description: "Synchronize a directory of financial Excel files",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"directory": map[string]any{
							"type":        "string",
							"description": "Directory path containing Excel files",
						},
						"company": map[string]any{
							"type":        "string",
							"description": "Override company name for all files",
						},
						"incremental": map[string]any{
							"type":        "boolean",
							"description": "Incremental sync",
						},
					},
					"required": []string{"directory"},
				},
			},
			{
				Name:        "finance-dimensions",
				Description: "Manage dimension mappings: list dimensions, add members, import/export, or preview",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"enum":        []string{"list", "mapping-stats", "seed-standard", "export-package", "import-dimensions", "import-members", "import-rules", "preview-import"},
							"description": "Dimension management action",
						},
						"company": map[string]any{
							"type":        "string",
							"description": "Company name for seed-standard action",
						},
						"file": map[string]any{
							"type":        "string",
							"description": "File path for import/preview actions",
						},
						"type": map[string]any{
							"type":        "string",
							"description": "Type for preview-import: dimensions or members",
						},
						"dimension": map[string]any{
							"type":        "string",
							"description": "Dimension code for add-member action",
						},
					},
					"required": []string{"action"},
				},
			},
		},
	}
	return s.sendResponse(req.ID, result)
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) error {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.sendErrorResponse(req.ID, -32602, "Invalid params", err.Error())
	}

	switch params.Name {
	case "finance-query":
		return s.handleFinanceQuery(ctx, req.ID, params.Arguments)
	case "finance-host-data":
		return s.handleFinanceHostData(ctx, req.ID, params.Arguments)
	case "finance-upload":
		return s.handleFinanceUpload(ctx, req.ID, params.Arguments)
	case "finance-sync":
		return s.handleFinanceSync(ctx, req.ID, params.Arguments)
	case "finance-dimensions":
		return s.handleFinanceDimensions(ctx, req.ID, params.Arguments)
	default:
		return s.sendErrorResponse(req.ID, -32602, "Unknown tool", params.Name)
	}
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
	return s.sendToolResponse(id, result)
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
	return s.sendToolResponse(id, result)
}

func (s *Server) handleFinanceUpload(ctx context.Context, id interface{}, args map[string]any) error {
	filePath, _ := args["file"].(string)
	if filePath == "" {
		return s.sendErrorResponse(id, -32602, "Missing required argument", "file")
	}

	company, _ := args["company"].(string)
	incremental, _ := args["incremental"].(bool)

	dbConn, err := db.Open(ctx, s.dbPath)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to open database", err.Error())
	}
	defer dbConn.Close()

	manager := dimensions.NewManager(dimensions.NewSQLiteRepository(dbConn))
	importer := ingest.NewImporter(manager)

	summary, err := importer.ImportFileWithOptions(ctx, s.dbPath, filePath, ingest.ImportOptions{
		Incremental:     incremental,
		CompanyOverride: company,
	})
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Import failed", err.Error())
	}

	return s.sendToolResponse(id, summary)
}

func (s *Server) handleFinanceSync(ctx context.Context, id interface{}, args map[string]any) error {
	dirPath, _ := args["directory"].(string)
	if dirPath == "" {
		return s.sendErrorResponse(id, -32602, "Missing required argument", "directory")
	}

	company, _ := args["company"].(string)
	incremental, _ := args["incremental"].(bool)

	dbConn, err := db.Open(ctx, s.dbPath)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to open database", err.Error())
	}
	defer dbConn.Close()

	manager := dimensions.NewManager(dimensions.NewSQLiteRepository(dbConn))
	importer := ingest.NewImporter(manager)

	summary, err := importer.SyncDirectoryWithOptions(ctx, s.dbPath, dirPath, ingest.ImportOptions{
		Incremental:     incremental,
		CompanyOverride: company,
	})
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Sync failed", err.Error())

	}

	return s.sendToolResponse(id, summary)
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
		return s.sendToolResponse(id, result)

	default:
		return s.sendErrorResponse(id, -32602, "Unknown dimensions action", action)
	}
}

func (s *Server) handleResourcesList(req *Request) error {
	resources := []Resource{}

	// Add SKILL.md and SKILL_APPENDIX as resources
	if s.skillPath != "" {
		resources = append(resources, Resource{
			URI:         "financeqa://skill",
			Name:        "SKILL.md",
			Description: "FinanceQA skill contract and usage guide",
			MimeType:    "text/markdown",
		})
	}
	if s.appendixPath != "" {
		resources = append(resources, Resource{
			URI:         "financeqa://appendix",
			Name:        "SKILL_APPENDIX.md",
			Description: "FinanceQA skill appendix with detailed rules",
			MimeType:    "text/markdown",
		})
	}

	return s.sendResponse(req.ID, ResourcesListResult{Resources: resources})
}

func (s *Server) handleResourcesRead(req *Request) error {
	var params ResourceReadParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.sendErrorResponse(req.ID, -32602, "Invalid params", err.Error())
	}

	var content []byte
	var mimeType string

	switch params.URI {
	case "financeqa://skill":
		if s.skillPath == "" {
			return s.sendErrorResponse(req.ID, -32602, "Resource not found", params.URI)
		}
		var err error
		content, err = os.ReadFile(s.skillPath)
		if err != nil {
			return s.sendErrorResponse(req.ID, -32603, "Failed to read skill", err.Error())
		}
		mimeType = "text/markdown"
	case "financeqa://appendix":
		if s.appendixPath == "" {
			return s.sendErrorResponse(req.ID, -32602, "Resource not found", params.URI)
		}
		var err error
		content, err = os.ReadFile(s.appendixPath)
		if err != nil {
			return s.sendErrorResponse(req.ID, -32603, "Failed to read appendix", err.Error())
		}
		mimeType = "text/markdown"
	default:
		// Try to read as file path
		var err error
		content, err = os.ReadFile(params.URI)
		if err != nil {
			return s.sendErrorResponse(req.ID, -32602, "Resource not found", params.URI)
		}
		mimeType = "application/octet-stream"
	}

	result := ResourceReadResult{
		Contents: []ResourceContent{
			{
				URI:      params.URI,
				MimeType: mimeType,
				Text:     string(content),
			},
		},
	}
	return s.sendResponse(req.ID, result)
}

// Response helpers

func (s *Server) sendResponse(id interface{}, result any) error {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	return s.writeResponse(resp)
}

func (s *Server) sendErrorResponse(id interface{}, code int, message, data string) error {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObject{
			Code:    code,
			Message: support.SanitizeDSN(message),
			Data:    support.SanitizeDSN(data),
		},
	}
	return s.writeResponse(resp)
}

func (s *Server) sendToolResponse(id interface{}, result any) error {
	// Convert result to JSON text for MCP content
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to marshal result", err.Error())
	}

	toolResult := ToolResult{
		Content: []ContentItem{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}
	return s.sendResponse(id, toolResult)
}

func (s *Server) sendNotification(method string, params any) error {
	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return s.writeResponse(notif)
}

func (s *Server) writeResponse(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(s.stdout, string(data)); err != nil {
		return err
	}
	return nil
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

// Helper types for internal use

type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ResourceReadParams struct {
	URI string `json:"uri"`
}

// Ensure Server implements the right interfaces
var _ = (*Server)(nil)
