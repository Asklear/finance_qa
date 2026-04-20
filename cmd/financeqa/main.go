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
	"financeqa/internal/query"
	"financeqa/internal/support"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	_ = support.LoadDotEnv(".env")
	_ = support.LoadDotEnv("/root/finance_qa/.env")

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

	fmt.Fprintf(stdout, "database initialized at %s\n", *dbPath)
	return 0
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
	company := fs.String("company", "模拟财务", "company name to query")
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
	company := fs.String("company", "模拟财务", "company name to query")
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

func runDimensions(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "dimensions requires a subcommand")
		return 2
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("dimensions list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		manager, cleanup, err := openDimensionsManager(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions manager failed: %v\n", err)
			return 1
		}
		defer cleanup()
		result, err := manager.ListDimensions(context.Background(), dimensions.DimensionQueryOptions{})
		if err != nil {
			fmt.Fprintf(stderr, "list dimensions failed: %v\n", err)
			return 1
		}
		return writeJSON(stdout, stderr, result)
	case "add-dimension":
		fs := flag.NewFlagSet("dimensions add-dimension", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		code := fs.String("code", "", "dimension code")
		name := fs.String("name", "", "dimension name")
		typ := fs.String("type", "custom", "dimension type")
		hierarchical := fs.Bool("hierarchical", false, "whether the dimension is hierarchical")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		manager, cleanup, err := openDimensionsManager(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions manager failed: %v\n", err)
			return 1
		}
		defer cleanup()
		created, err := manager.CreateDimension(context.Background(), dimensions.CreateDimensionInput{
			Code:           *code,
			Name:           *name,
			Type:           dimensions.DimensionType(*typ),
			IsHierarchical: *hierarchical,
		})
		if err != nil {
			fmt.Fprintf(stderr, "add dimension failed: %v\n", err)
			return 1
		}
		return writeJSON(stdout, stderr, created)
	case "add-member":
		fs := flag.NewFlagSet("dimensions add-member", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		dimensionCode := fs.String("dimension", "", "dimension code")
		code := fs.String("code", "", "member code")
		name := fs.String("name", "", "member name")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		manager, cleanup, err := openDimensionsManager(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions manager failed: %v\n", err)
			return 1
		}
		defer cleanup()
		dim, err := manager.GetDimensionByCode(context.Background(), *dimensionCode)
		if err != nil {
			fmt.Fprintf(stderr, "get dimension failed: %v\n", err)
			return 1
		}
		created, err := manager.AddMember(context.Background(), dimensions.AddMemberInput{
			DimensionID: dim.ID,
			Code:        *code,
			Name:        *name,
		})
		if err != nil {
			fmt.Fprintf(stderr, "add member failed: %v\n", err)
			return 1
		}
		return writeJSON(stdout, stderr, created)
	case "mapping-stats":
		fs := flag.NewFlagSet("dimensions mapping-stats", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		company := fs.String("company", "", "company")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		manager, cleanup, err := openDimensionsManager(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions manager failed: %v\n", err)
			return 1
		}
		defer cleanup()
		rules, err := manager.ListMappingRules(context.Background(), dimensions.MappingRuleQueryOptions{Company: *company})
		if err != nil {
			fmt.Fprintf(stderr, "list mapping rules failed: %v\n", err)
			return 1
		}
		return writeJSON(stdout, stderr, map[string]any{
			"company":   *company,
			"ruleCount": rules.Total,
			"rules":     rules.Data,
		})
	case "seed-standard":
		fs := flag.NewFlagSet("dimensions seed-standard", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		company := fs.String("company", "", "company to initialize standard rules for")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *company == "" {
			fmt.Fprintln(stderr, "seed-standard requires --company")
			return 2
		}

		manager, cleanup, err := openDimensionsManager(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions manager failed: %v\n", err)
			return 1
		}
		defer cleanup()

		// 1. Initialize dimension members and rules
		if err := manager.InitializeStandardRules(context.Background(), *company); err != nil {
			fmt.Fprintf(stderr, "seed-standard failed: %v\n", err)
			return 1
		}

		fmt.Fprintf(stdout, "successfully seeded standard CAS rules for %s\n", *company)
		return 0
	case "export-package":
		fs := flag.NewFlagSet("dimensions export-package", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		outputPath := fs.String("output", "", "output file path")
		format := fs.String("format", "json", "export format")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *outputPath == "" {
			fmt.Fprintln(stderr, "export-package requires --output")
			return 2
		}
		if *format != "json" {
			fmt.Fprintln(stderr, "only json format is currently supported")
			return 2
		}
		exchange, cleanup, err := openDimensionsExchange(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions exchange failed: %v\n", err)
			return 1
		}
		defer cleanup()
		pkg, err := exchange.ExportFullPackage(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "export package failed: %v\n", err)
			return 1
		}
		if err := writeJSONFile(*outputPath, pkg); err != nil {
			fmt.Fprintf(stderr, "write export package failed: %v\n", err)
			return 1
		}
		return writeJSON(stdout, stderr, map[string]any{"output": *outputPath, "dimensionCount": len(pkg.Dimensions), "mappingRuleCount": len(pkg.MappingRules)})
	case "import-dimensions":
		fs := flag.NewFlagSet("dimensions import-dimensions", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		filePath := fs.String("file", "", "input json file")
		validateOnly := fs.Bool("validate-only", false, "validate without persisting")
		skipExisting := fs.Bool("skip-existing", false, "skip existing dimensions")
		updateExisting := fs.Bool("update-existing", false, "update existing dimensions")
		format := fs.String("format", "json", "import format")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *filePath == "" {
			fmt.Fprintln(stderr, "import-dimensions requires --file")
			return 2
		}
		if *format != "json" {
			fmt.Fprintln(stderr, "only json format is currently supported")
			return 2
		}
		var dims []dimensions.DimensionExport
		if err := readJSONFile(*filePath, &dims); err != nil {
			fmt.Fprintf(stderr, "read dimensions import file failed: %v\n", err)
			return 1
		}
		exchange, cleanup, err := openDimensionsExchange(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions exchange failed: %v\n", err)
			return 1
		}
		defer cleanup()
		report := exchange.ImportDimensions(context.Background(), dims, dimensions.ImportOptions{
			ValidateOnly:   *validateOnly,
			SkipExisting:   *skipExisting,
			UpdateExisting: *updateExisting,
		})
		return writeJSON(stdout, stderr, report)
	case "import-members":
		fs := flag.NewFlagSet("dimensions import-members", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		dimensionCode := fs.String("dimension", "", "dimension code")
		filePath := fs.String("file", "", "input json file")
		validateOnly := fs.Bool("validate-only", false, "validate without persisting")
		skipExisting := fs.Bool("skip-existing", false, "skip existing members")
		updateExisting := fs.Bool("update-existing", false, "update existing members")
		format := fs.String("format", "json", "import format")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *dimensionCode == "" || *filePath == "" {
			fmt.Fprintln(stderr, "import-members requires --dimension and --file")
			return 2
		}
		if *format != "json" {
			fmt.Fprintln(stderr, "only json format is currently supported")
			return 2
		}
		var members []dimensions.MemberExport
		if err := readJSONFile(*filePath, &members); err != nil {
			fmt.Fprintf(stderr, "read members import file failed: %v\n", err)
			return 1
		}
		exchange, cleanup, err := openDimensionsExchange(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions exchange failed: %v\n", err)
			return 1
		}
		defer cleanup()
		report := exchange.ImportMembers(context.Background(), *dimensionCode, members, dimensions.ImportOptions{
			ValidateOnly:   *validateOnly,
			SkipExisting:   *skipExisting,
			UpdateExisting: *updateExisting,
		})
		return writeJSON(stdout, stderr, report)
	case "import-rules":
		fs := flag.NewFlagSet("dimensions import-rules", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		filePath := fs.String("file", "", "input json file")
		company := fs.String("company", "", "override company")
		validateOnly := fs.Bool("validate-only", false, "validate without persisting")
		skipExisting := fs.Bool("skip-existing", false, "skip existing rules")
		updateExisting := fs.Bool("update-existing", false, "update existing rules")
		format := fs.String("format", "json", "import format")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *filePath == "" {
			fmt.Fprintln(stderr, "import-rules requires --file")
			return 2
		}
		if *format != "json" {
			fmt.Fprintln(stderr, "only json format is currently supported")
			return 2
		}
		var rules []dimensions.MappingRuleExport
		if err := readJSONFile(*filePath, &rules); err != nil {
			fmt.Fprintf(stderr, "read rules import file failed: %v\n", err)
			return 1
		}
		exchange, cleanup, err := openDimensionsExchange(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions exchange failed: %v\n", err)
			return 1
		}
		defer cleanup()
		report := exchange.ImportMappingRules(context.Background(), rules, dimensions.ImportOptions{
			ValidateOnly:   *validateOnly,
			SkipExisting:   *skipExisting,
			UpdateExisting: *updateExisting,
			Company:        *company,
		})
		return writeJSON(stdout, stderr, report)
	case "preview-import":
		fs := flag.NewFlagSet("dimensions preview-import", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
		previewType := fs.String("type", "", "preview type: dimensions or members")
		dimensionCode := fs.String("dimension", "", "dimension code for member preview")
		filePath := fs.String("file", "", "input json file")
		format := fs.String("format", "json", "import format")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *previewType == "" || *filePath == "" {
			fmt.Fprintln(stderr, "preview-import requires --type and --file")
			return 2
		}
		if *format != "json" {
			fmt.Fprintln(stderr, "only json format is currently supported")
			return 2
		}
		exchange, cleanup, err := openDimensionsExchange(*dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "open dimensions exchange failed: %v\n", err)
			return 1
		}
		defer cleanup()
		switch *previewType {
		case "dimensions":
			var dims []dimensions.DimensionExport
			if err := readJSONFile(*filePath, &dims); err != nil {
				fmt.Fprintf(stderr, "read dimensions preview file failed: %v\n", err)
				return 1
			}
			return writeJSON(stdout, stderr, exchange.PreviewDimensionsImport(context.Background(), dims))
		case "members":
			if *dimensionCode == "" {
				fmt.Fprintln(stderr, "preview-import with type=members requires --dimension")
				return 2
			}
			var members []dimensions.MemberExport
			if err := readJSONFile(*filePath, &members); err != nil {
				fmt.Fprintf(stderr, "read members preview file failed: %v\n", err)
				return 1
			}
			return writeJSON(stdout, stderr, exchange.PreviewMembersImport(context.Background(), *dimensionCode, members))
		default:
			fmt.Fprintf(stderr, "unsupported preview type: %s\n", *previewType)
			return 2
		}
	default:
		fmt.Fprintf(stderr, "unknown dimensions subcommand: %s\n", args[0])
		return 2
	}
}

func openDimensionsManager(dbPath string) (*dimensions.Manager, func(), error) {
	sqlDB, err := db.Open(context.Background(), dbPath)
	if err != nil {
		return nil, nil, err
	}
	repo := dimensions.NewSQLiteRepository(sqlDB)
	return dimensions.NewManager(repo), func() { _ = sqlDB.Close() }, nil
}

func openDimensionsExchange(dbPath string) (*dimensions.DataExchange, func(), error) {
	manager, cleanup, err := openDimensionsManager(dbPath)
	if err != nil {
		return nil, nil, err
	}
	return dimensions.NewDataExchange(manager), cleanup, nil
}

func writeJSON(stdout, stderr io.Writer, v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "marshal json failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
}
