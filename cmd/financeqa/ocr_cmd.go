package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"financeqa/internal/db"
	"financeqa/internal/ocr"
	objectstorage "financeqa/internal/storage"
)

func runOCR(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ocr requires a subcommand: process-pending, process-file, retry-failed")
		return 2
	}

	switch args[0] {
	case "process-pending":
		return runOCRProcessPending(args[1:], stdout, stderr)
	case "process-file":
		return runOCRProcessFile(args[1:], stdout, stderr)
	case "retry-failed":
		return runOCRRetryFailed(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown ocr subcommand: %s\n", args[0])
		return 2
	}
}

func runOCRProcessPending(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ocr process-pending", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
	limit := fs.Int("limit", envInt("OCR_WORKER_LIMIT", 10), "maximum pending PDFs to process")
	concurrency := fs.Int("concurrency", defaultOCRWorkerConcurrency(), "maximum concurrent Gemini OCR calls")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	config, err := geminiConfigFromEnv()
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	resolver, err := objectstorage.NewResolverFromEnv()
	if err != nil {
		fmt.Fprintf(stderr, "load object storage failed: %v\n", err)
		return 1
	}
	config.FileResolver = resolver
	repo, closeFn, ok := openOCRRepository(context.Background(), resolveDBPath(*dbPath), stderr)
	if !ok {
		return 1
	}
	defer closeFn()

	worker := ocr.NewWorker(repo, ocr.NewGeminiClient(config))
	summary, err := worker.ProcessPending(context.Background(), ocr.ProcessOptions{Limit: *limit, Concurrency: *concurrency})
	if err != nil {
		fmt.Fprintf(stderr, "process pending ocr failed: %v\n", err)
		return 1
	}
	return writeJSON(stdout, stderr, map[string]any{"success": true, "data": summary})
}

func runOCRProcessFile(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ocr process-file", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
	file := fs.String("file", "", "PDF file to process")
	contractID := fs.Int64("contract-id", 0, "optional contract_main id to update")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*file) == "" {
		fmt.Fprintln(stderr, "--file is required")
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	config, err := geminiConfigFromEnv()
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	resolver, err := objectstorage.NewResolverFromEnv()
	if err != nil {
		fmt.Fprintf(stderr, "load object storage failed: %v\n", err)
		return 1
	}
	config.FileResolver = resolver
	client := ocr.NewGeminiClient(config)
	result, run, err := client.ExtractPDF(context.Background(), *file)
	if err != nil {
		fmt.Fprintf(stderr, "process file ocr failed: %v\n", err)
		return 1
	}
	quality := ocr.Validate(result)
	if *contractID > 0 {
		repo, closeFn, ok := openOCRRepository(context.Background(), resolveDBPath(*dbPath), stderr)
		if !ok {
			return 1
		}
		defer closeFn()
		doc := ocr.PendingDocument{ID: *contractID, FileName: *file, StorageKey: *file}
		if err := repo.SaveResult(context.Background(), doc, result, quality, run); err != nil {
			fmt.Fprintf(stderr, "save file ocr result failed: %v\n", err)
			return 1
		}
	}
	return writeJSON(stdout, stderr, map[string]any{"success": true, "result": result, "quality": quality, "run": run})
}

func runOCRRetryFailed(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ocr retry-failed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
	limit := fs.Int("limit", 10, "maximum failed rows to mark pending")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	repo, closeFn, ok := openOCRRepository(context.Background(), resolveDBPath(*dbPath), stderr)
	if !ok {
		return 1
	}
	defer closeFn()
	updated, err := repo.RetryFailed(context.Background(), *limit)
	if err != nil {
		fmt.Fprintf(stderr, "retry failed ocr rows failed: %v\n", err)
		return 1
	}
	return writeJSON(stdout, stderr, map[string]any{"success": true, "updated": updated})
}

func openOCRRepository(ctx context.Context, dbPath string, stderr io.Writer) (*ocr.Repository, func(), bool) {
	if err := db.Bootstrap(ctx, dbPath); err != nil {
		fmt.Fprintf(stderr, "bootstrap db failed: %v\n", err)
		return nil, nil, false
	}
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "open db failed: %v\n", err)
		return nil, nil, false
	}
	return ocr.NewRepository(sqlDB), func() { _ = sqlDB.Close() }, true
}

func geminiConfigFromEnv() (ocr.GeminiConfig, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return ocr.GeminiConfig{}, fmt.Errorf("GEMINI_API_KEY is required")
	}
	timeout := time.Duration(envInt("GEMINI_OCR_TIMEOUT_SECONDS", 240)) * time.Second
	maxFileBytes := int64(envInt("GEMINI_OCR_MAX_FILE_MB", 50)) * 1024 * 1024
	return ocr.GeminiConfig{
		APIKey:                  apiKey,
		BaseURL:                 geminiBaseURLFromEnv(),
		Model:                   strings.TrimSpace(os.Getenv("GEMINI_MODEL")),
		ProxyURL:                strings.TrimSpace(os.Getenv("GEMINI_PROXY")),
		Timeout:                 timeout,
		MaxFileBytes:            maxFileBytes,
		InputCostPerMillionUSD:  envFloat("OCR_COST_USD_PER_M_INPUT_TOKENS", 0),
		OutputCostPerMillionUSD: envFloat("OCR_COST_USD_PER_M_OUTPUT_TOKENS", 0),
	}, nil
}

func geminiBaseURLFromEnv() string {
	if baseURL := strings.TrimSpace(os.Getenv("GOOGLE_GEMINI_BASE_URL")); baseURL != "" {
		return normalizeGeminiBaseURL(baseURL)
	}
	return normalizeGeminiBaseURL(os.Getenv("GEMINI_BASE_URL"))
}

func normalizeGeminiBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	lower := strings.ToLower(baseURL)
	if strings.HasSuffix(lower, "/v1beta") || strings.HasSuffix(lower, "/v1") {
		return baseURL
	}
	return baseURL + "/v1beta"
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func defaultOCRWorkerConcurrency() int {
	return envInt("OCR_WORKER_CONCURRENCY", 1)
}

func envFloat(name string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
