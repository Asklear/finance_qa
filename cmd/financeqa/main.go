package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"financeqa/internal/config"
	"financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/ingest"
	"financeqa/internal/mcp"
	"financeqa/internal/query"
	"financeqa/internal/support"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	_ = support.LoadAppDotEnv(support.FindProjectRoot())

	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "init-db":
		return runInitDB(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "keywords":
		return runKeywords(args[1:], stdout, stderr)
	case "query":
		return runQuery(args[1:], stdout, stderr)
	case "import":
		return runImport(args[1:], stdout, stderr)
	case "sync":
		return runSync(args[1:], stdout, stderr)
	case "host-data":
		return runHostData(args[1:], stdout, stderr)
	case "dimensions":
		return runDimensions(args[1:], stdout, stderr)
	case "feishu":
		return runFeishu(args[1:], stdout, stderr)
	case "ocr":
		return runOCR(args[1:], stdout, stderr)
	case "audit-accuracy":
		return runAuditAccuracy(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	default:
		return runQuery(args, stdout, stderr)
	}
}

func runInitDB(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init-db", flag.ContinueOnError)
	fs.SetOutput(stderr)

	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}

	if err := db.Bootstrap(context.Background(), *dbPath); err != nil {
		fmt.Fprintf(stderr, "init-db failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "database initialized at %s\n", redactDBTargetForCLI(*dbPath))
	return 0
}

func redactDBTargetForCLI(target string) string {
	parts := strings.Fields(strings.TrimSpace(target))
	if len(parts) == 0 {
		return target
	}
	changed := false
	for i, part := range parts {
		if strings.HasPrefix(strings.ToLower(part), "password=") {
			parts[i] = "password=<redacted>"
			changed = true
		}
	}
	if changed {
		return strings.Join(parts, " ")
	}
	return target
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "config requires a subcommand: show")
		return 2
	}

	switch args[0] {
	case "show":
		fs := flag.NewFlagSet("config show", flag.ContinueOnError)
		fs.SetOutput(stderr)
		configPath := fs.String("config", support.DefaultUserConfigPath(""), "path to user config yaml")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}

		mgr, err := config.NewUserConfigManager(*configPath)
		if err != nil {
			fmt.Fprintf(stderr, "load config failed: %v\n", err)
			return 1
		}

		b, err := yaml.Marshal(mgr.GetAllConfig())
		if err != nil {
			fmt.Fprintf(stderr, "marshal config failed: %v\n", err)
			return 1
		}
		if _, err := stdout.Write(b); err != nil {
			fmt.Fprintf(stderr, "write output failed: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown config subcommand: %s\n", args[0])
		return 2
	}
}

func runKeywords(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "keywords requires a subcommand: intents")
		return 2
	}

	switch args[0] {
	case "intents":
		fs := flag.NewFlagSet("keywords intents", flag.ContinueOnError)
		fs.SetOutput(stderr)
		path := fs.String("keywords", support.DefaultKeywordsPath(""), "path to query keywords json")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}

		mgr := config.NewKeywordsManager(*path)
		for _, name := range mgr.GetIntentNames() {
			fmt.Fprintln(stdout, name)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown keywords subcommand: %s\n", args[0])
		return 2
	}
}

func isProductionMode() bool {
	// 1. Priority: Environment variable (standard for servers/CI-CD)
	if os.Getenv("APP_ENV") == "production" {
		return true
	}

	// 2. Fallback: Local skill.md file (useful for rapid local switching)
	content, err := os.ReadFile("skill.md")
	if err != nil {
		return false // Default to test mode if file missing
	}
	// Specifically look for the active selection, avoiding the hint in parentheses
	return strings.Contains(string(content), "当前运行模式：【正式版本】")
}

func runQuery(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	company := fs.String("company", support.DefaultCompanyName(), "company name to query")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	question := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if question == "" {
		fmt.Fprintln(stderr, "query requires a natural language question")
		return 2
	}

	engine, err := query.NewEngine(*dbPath, *company)
	if err != nil {
		fmt.Fprintf(stderr, "create query engine failed: %v\n", err)
		return 1
	}
	defer func() { _ = engine.Close() }()

	result := engine.Query(question)
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "marshal query result failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	if !result.Success {
		// 关键兼容：即便业务失败，也把完整JSON（含llm_payload/trace）写到stdout，
		// 便于桥接层做降级；同时保留非0退出码，兼容CLI语义与CI脚本。
		fmt.Fprintln(stderr, result.Message)
		return 1
	}
	return 0
}

func runHostData(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("host-data", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	company := fs.String("company", support.DefaultCompanyName(), "company name to query")
	from := fs.String("from", "", "period start in YYYY-MM")
	to := fs.String("to", "", "period end in YYYY-MM")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	question := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if question == "" {
		question = "输出全量财报原始数据给宿主LLM"
	}

	engine, err := query.NewEngine(*dbPath, *company)
	if err != nil {
		fmt.Fprintf(stderr, "create query engine failed: %v\n", err)
		return 1
	}
	defer func() { _ = engine.Close() }()

	result := engine.HostLLMPayload(*from, *to, question)
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "marshal host-data result failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}

func runImport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	incremental := fs.Bool("incremental", false, "incremental import (don't clear existing data)")
	company := fs.String("company", "", "override company name for imported file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	filePath := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if filePath == "" {
		fmt.Fprintln(stderr, "import requires a file path")
		return 2
	}

	db, err := db.Open(context.Background(), *dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "open db failed: %v\n", err)
		return 1
	}
	defer db.Close()
	manager := dimensions.NewManager(dimensions.NewSQLiteRepository(db))

	importer := ingest.NewImporter(manager)
	summary, err := importer.ImportFileWithOptions(context.Background(), *dbPath, filePath, ingest.ImportOptions{
		Incremental:     *incremental,
		CompanyOverride: strings.TrimSpace(*company),
	})
	if err != nil {
		fmt.Fprintf(stderr, "import failed: %v\n", err)
		return 1
	}

	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "marshal import summary failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}

func runSync(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	incremental := fs.Bool("incremental", false, "incremental sync (don't clear existing data)")
	company := fs.String("company", "", "override company name for all imported files in this sync")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dirPath := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if dirPath == "" {
		fmt.Fprintln(stderr, "sync requires a directory path")
		return 2
	}

	db, err := db.Open(context.Background(), *dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "open db failed: %v\n", err)
		return 1
	}
	defer db.Close()
	manager := dimensions.NewManager(dimensions.NewSQLiteRepository(db))

	importer := ingest.NewImporter(manager)
	summary, err := importer.SyncDirectoryWithOptions(context.Background(), *dbPath, dirPath, ingest.ImportOptions{
		Incremental:     *incremental,
		CompanyOverride: strings.TrimSpace(*company),
	})
	if err != nil {
		fmt.Fprintf(stderr, "sync failed: %v\n", err)
		return 1
	}
	return writeJSON(stdout, stderr, summary)
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	company := fs.String("company", support.DefaultCompanyName(), "company name to query")
	skillPath := fs.String("skill", "", "path to SKILL.md (auto-detected if not set)")
	appendixPath := fs.String("appendix", "", "path to SKILL_APPENDIX_FULL.md (auto-detected if not set)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Auto-detect paths if not provided
	skill := *skillPath
	appendix := *appendixPath
	if skill == "" || appendix == "" {
		autoSkill, autoAppendix := mcp.AutoDetectPaths()
		if skill == "" {
			skill = autoSkill
		}
		if appendix == "" {
			appendix = autoAppendix
		}
	}

	server := mcp.NewServer(
		mcp.WithDBPath(*dbPath),
		mcp.WithCompany(*company),
		mcp.WithSkillPath(skill),
		mcp.WithAppendixPath(appendix),
		mcp.WithIO(os.Stdin, stdout, stderr),
	)

	if err := server.Run(context.Background()); err != nil {
		fmt.Fprintf(stderr, "server error: %v\n", err)
		return 1
	}
	return 0
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "financeqa - PostgreSQL CLI")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  financeqa init-db [--db <dsn>]")
	fmt.Fprintln(out, "  financeqa config show [--config <path>]")
	fmt.Fprintln(out, "  financeqa keywords intents [--keywords <path>]")
	fmt.Fprintln(out, "  financeqa query [--db <dsn>] [--company <name>] <question>")
	fmt.Fprintln(out, "  financeqa import [--db <dsn>] [--incremental] <file>")
	fmt.Fprintln(out, "  financeqa sync [--db <dsn>] [--incremental] <directory>")
	fmt.Fprintln(out, "  financeqa dimensions list [--db <dsn>]")
	fmt.Fprintln(out, "  financeqa dimensions add-dimension --db <dsn> --code <code> --name <name> --type <type>")
	fmt.Fprintln(out, "  financeqa dimensions add-member --db <dsn> --dimension <code> --code <code> --name <name>")
	fmt.Fprintln(out, "  financeqa dimensions mapping-stats [--db <dsn>] [--company <name>]")
	fmt.Fprintln(out, "  financeqa dimensions seed-standard [--db <dsn>] --company <name>")
	fmt.Fprintln(out, "  financeqa dimensions export-package --db <dsn> --output <file> [--format json]")
	fmt.Fprintln(out, "  financeqa dimensions import-dimensions --db <dsn> --file <file> [--validate-only] [--skip-existing] [--update-existing]")
	fmt.Fprintln(out, "  financeqa dimensions import-members --db <dsn> --dimension <code> --file <file> [--validate-only] [--skip-existing] [--update-existing]")
	fmt.Fprintln(out, "  financeqa dimensions import-rules --db <dsn> --file <file> [--company <name>] [--validate-only] [--skip-existing] [--update-existing]")
	fmt.Fprintln(out, "  financeqa dimensions preview-import --db <dsn> --type <dimensions|members> --file <file> [--dimension <code>]")
	fmt.Fprintln(out, "  financeqa feishu seed-sources [--db <dsn>]")
	fmt.Fprintln(out, "  financeqa feishu sources [--db <dsn>] [--source-type <type>]")
	fmt.Fprintln(out, "  financeqa feishu scan [--db <dsn>] [--company <name>]")
	fmt.Fprintln(out, "  financeqa feishu sync-once [--db <dsn>] --source-token <token>")
	fmt.Fprintln(out, "  financeqa ocr process-pending [--db <dsn>] [--limit <n>]")
	fmt.Fprintln(out, "  financeqa ocr process-file [--db <dsn>] --file <pdf> [--contract-id <id>]")
	fmt.Fprintln(out, "  financeqa ocr retry-failed [--db <dsn>] [--limit <n>]")
	fmt.Fprintln(out, "  financeqa audit-accuracy [--db <dsn>] [--workbook <xlsx>] [--out <json>]")
	fmt.Fprintln(out, "  financeqa serve [--db <dsn>] [--company <name>] [--skill <path>] [--appendix <path>]")
}
