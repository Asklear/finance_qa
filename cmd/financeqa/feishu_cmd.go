package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/feishu"
	"financeqa/internal/feishusync"
	"financeqa/internal/ingest"
	"financeqa/internal/support"
)

func runFeishu(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "feishu requires a subcommand: sources, seed-sources, scan, sync-once")
		return 2
	}

	switch args[0] {
	case "sources":
		return runFeishuSources(args[1:], stdout, stderr)
	case "seed-sources":
		return runFeishuSeedSources(args[1:], stdout, stderr)
	case "scan":
		return runFeishuScan(args[1:], stdout, stderr)
	case "sync-once":
		return runFeishuSyncOnce(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown feishu subcommand: %s\n", args[0])
		return 2
	}
}

func runFeishuSources(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu sources", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	sourceType := fs.String("source-type", "", "optional source type filter")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}

	repo, closeFn, ok := openFeishuRepository(context.Background(), *dbPath, stderr)
	if !ok {
		return 1
	}
	defer closeFn()

	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: *sourceType, IncludeDisabled: true})
	if err != nil {
		fmt.Fprintf(stderr, "list feishu sources failed: %v\n", err)
		return 1
	}
	return writeJSON(stdout, stderr, map[string]any{
		"success": true,
		"data":    sources,
	})
}

func runFeishuSeedSources(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu seed-sources", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}

	repo, closeFn, ok := openFeishuRepository(context.Background(), *dbPath, stderr)
	if !ok {
		return 1
	}
	defer closeFn()

	defaults := feishusync.DefaultSources()
	for _, src := range defaults {
		if err := repo.UpsertSource(context.Background(), src); err != nil {
			fmt.Fprintf(stderr, "seed feishu source %s failed: %v\n", src.SourceToken, err)
			return 1
		}
	}
	return writeJSON(stdout, stderr, map[string]any{
		"success": true,
		"seeded":  len(defaults),
		"data":    defaults,
	})
}

func runFeishuScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	company := fs.String("company", support.DefaultCompanyName(), "company override for workbook import")
	snapshotDir := fs.String("snapshot-dir", defaultFeishuSnapshotDir(), "directory for downloaded Feishu snapshots")
	sourceType := fs.String("source-type", "all", "source type: all, pdf_folder, finance_workbook")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}

	runner, closeFn, ok := openFeishuRunner(context.Background(), *dbPath, *snapshotDir, *company, stderr)
	if !ok {
		return 1
	}
	defer closeFn()

	filterType, err := normalizeFeishuSourceType(*sourceType)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}
	sources, err := runner.repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: filterType})
	if err != nil {
		fmt.Fprintf(stderr, "list feishu sources failed: %v\n", err)
		return 1
	}

	results := make([]feishusync.ScanResult, 0, len(sources))
	for _, src := range sources {
		result, err := runner.scanSource(context.Background(), src)
		if err != nil {
			fmt.Fprintf(stderr, "scan feishu source %s failed: %v\n", src.SourceToken, err)
			return 1
		}
		results = append(results, result)
	}
	return writeJSON(stdout, stderr, map[string]any{
		"success": true,
		"count":   len(results),
		"data":    results,
	})
}

func runFeishuSyncOnce(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu sync-once", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", support.DefaultDBPath(""), "postgres dsn (or FINANCEQA_PG_DSN env)")
	company := fs.String("company", support.DefaultCompanyName(), "company override for workbook import")
	snapshotDir := fs.String("snapshot-dir", defaultFeishuSnapshotDir(), "directory for downloaded Feishu snapshots")
	sourceToken := fs.String("source-token", "", "Feishu source token to scan once")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}
	if strings.TrimSpace(*sourceToken) == "" {
		fmt.Fprintln(stderr, "--source-token is required")
		return 2
	}

	runner, closeFn, ok := openFeishuRunner(context.Background(), *dbPath, *snapshotDir, *company, stderr)
	if !ok {
		return 1
	}
	defer closeFn()

	sources, err := runner.repo.ListSources(context.Background(), feishusync.SourceFilter{})
	if err != nil {
		fmt.Fprintf(stderr, "list feishu sources failed: %v\n", err)
		return 1
	}
	var matched *feishusync.SyncSource
	for i := range sources {
		if sources[i].SourceToken == strings.TrimSpace(*sourceToken) {
			matched = &sources[i]
			break
		}
	}
	if matched == nil {
		fmt.Fprintf(stderr, "feishu source token not found: %s\n", strings.TrimSpace(*sourceToken))
		return 1
	}

	result, err := runner.scanSource(context.Background(), *matched)
	if err != nil {
		fmt.Fprintf(stderr, "scan feishu source %s failed: %v\n", matched.SourceToken, err)
		return 1
	}
	return writeJSON(stdout, stderr, map[string]any{
		"success": true,
		"data":    result,
	})
}

func openFeishuRepository(ctx context.Context, dbPath string, stderr io.Writer) (*feishusync.Repository, func(), bool) {
	if err := db.Bootstrap(ctx, dbPath); err != nil {
		fmt.Fprintf(stderr, "bootstrap db failed: %v\n", err)
		return nil, nil, false
	}
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "open db failed: %v\n", err)
		return nil, nil, false
	}
	return feishusync.NewRepository(sqlDB), func() { _ = sqlDB.Close() }, true
}

type feishuRunner struct {
	repo     *feishusync.Repository
	pdf      *feishusync.PDFScanner
	workbook *feishusync.WorkbookScanner
}

func openFeishuRunner(ctx context.Context, dbPath, snapshotDir, company string, stderr io.Writer) (*feishuRunner, func(), bool) {
	if err := db.Bootstrap(ctx, dbPath); err != nil {
		fmt.Fprintf(stderr, "bootstrap db failed: %v\n", err)
		return nil, nil, false
	}
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "open db failed: %v\n", err)
		return nil, nil, false
	}

	client, err := feishu.NewHTTPClientFromEnv()
	if err != nil {
		_ = sqlDB.Close()
		fmt.Fprintln(stderr, err.Error())
		return nil, nil, false
	}

	repo := feishusync.NewRepository(sqlDB)
	manager := dimensions.NewManager(dimensions.NewSQLiteRepository(sqlDB))
	importer := ingest.NewImporter(manager)
	runner := &feishuRunner{
		repo:     repo,
		pdf:      feishusync.NewPDFScanner(client, repo, feishusync.NoopOCRDispatcher{}, snapshotDir),
		workbook: feishusync.NewWorkbookScanner(client, repo, importer, dbPath, snapshotDir, company),
	}
	return runner, func() { _ = sqlDB.Close() }, true
}

func (r *feishuRunner) scanSource(ctx context.Context, src feishusync.SyncSource) (feishusync.ScanResult, error) {
	switch src.SourceType {
	case feishusync.SourceTypePDFFolder:
		return r.pdf.ScanFolder(ctx, src)
	case feishusync.SourceTypeFinanceWorkbook:
		return r.workbook.ScanWorkbook(ctx, src)
	default:
		return feishusync.ScanResult{SourceID: src.ID, Source: src.SourceToken}, fmt.Errorf("unsupported feishu source type: %s", src.SourceType)
	}
}

func normalizeFeishuSourceType(sourceType string) (string, error) {
	sourceType = strings.TrimSpace(sourceType)
	if sourceType == "" || sourceType == "all" {
		return "", nil
	}
	switch sourceType {
	case feishusync.SourceTypePDFFolder, feishusync.SourceTypeFinanceWorkbook:
		return sourceType, nil
	default:
		return "", fmt.Errorf("unsupported --source-type: %s", sourceType)
	}
}

func defaultFeishuSnapshotDir() string {
	return filepath.Join("tmp", "feishu-snapshots")
}
