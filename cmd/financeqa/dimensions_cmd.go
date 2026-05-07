package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"financeqa/internal/db"
	"financeqa/internal/dimensions"
)

func runDimensions(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "dimensions requires a subcommand")
		return 2
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("dimensions list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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

		if err := manager.InitializeStandardRules(context.Background(), *company); err != nil {
			fmt.Fprintf(stderr, "seed-standard failed: %v\n", err)
			return 1
		}

		fmt.Fprintf(stdout, "successfully seeded standard CAS rules for %s\n", *company)
		return 0
	case "export-package":
		fs := flag.NewFlagSet("dimensions export-package", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
		dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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
	sqlDB, err := db.Open(context.Background(), resolveDBPath(dbPath))
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
