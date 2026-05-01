package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"financeqa/internal/db"
	"financeqa/internal/feishusync"
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
		fmt.Fprintln(stderr, "feishu scan is not implemented yet")
		return 2
	case "sync-once":
		fmt.Fprintln(stderr, "feishu sync-once is not implemented yet")
		return 2
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
