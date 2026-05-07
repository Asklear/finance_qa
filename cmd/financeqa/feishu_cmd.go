package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/feishu"
	"financeqa/internal/feishusync"
	"financeqa/internal/ingest"
	objectstorage "financeqa/internal/storage"
	"financeqa/internal/support"
)

func runFeishu(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "feishu requires a subcommand: sources, seed-sources, scan, sync-once, oauth-url, exchange-code, oauth-login")
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
	case "oauth-url":
		return runFeishuOAuthURL(args[1:], stdout, stderr)
	case "exchange-code":
		return runFeishuExchangeCode(args[1:], stdout, stderr)
	case "oauth-login":
		return runFeishuOAuthLogin(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown feishu subcommand: %s\n", args[0])
		return 2
	}
}

func runFeishuOAuthURL(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu oauth-url", flag.ContinueOnError)
	fs.SetOutput(stderr)
	redirectURI := fs.String("redirect-uri", defaultFeishuRedirectURI(), "OAuth redirect URI configured in Feishu app")
	state := fs.String("state", "", "optional OAuth state")
	scope := fs.String("scope", defaultFeishuOAuthScope(), "OAuth scope string")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}
	client, ok := newFeishuOAuthClientFromEnv(stderr)
	if !ok {
		return 1
	}
	if strings.TrimSpace(*state) == "" {
		*state = randomState()
	}
	fmt.Fprintln(stdout, client.OAuthURL(*redirectURI, *state, *scope))
	return 0
}

func runFeishuExchangeCode(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu exchange-code", flag.ContinueOnError)
	fs.SetOutput(stderr)
	code := fs.String("code", "", "authorization code from Feishu redirect")
	tokenFile := fs.String("token-file", defaultFeishuUserTokenFile(), "path to save Feishu user token JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*code) == "" {
		fmt.Fprintln(stderr, "--code is required")
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}
	client, ok := newFeishuOAuthClientFromEnv(stderr)
	if !ok {
		return 1
	}
	token, err := client.ExchangeCode(context.Background(), *code)
	if err != nil {
		fmt.Fprintf(stderr, "exchange feishu oauth code failed: %v\n", err)
		return 1
	}
	if err := feishu.SaveUserToken(*tokenFile, token); err != nil {
		fmt.Fprintf(stderr, "save feishu user token failed: %v\n", err)
		return 1
	}
	return writeJSON(stdout, stderr, map[string]any{
		"success":    true,
		"token_file": *tokenFile,
		"expires_at": token.ExpiresAt,
	})
}

func runFeishuOAuthLogin(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu oauth-login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	redirectURI := fs.String("redirect-uri", defaultFeishuRedirectURI(), "OAuth redirect URI configured in Feishu app")
	listenAddr := fs.String("listen", "127.0.0.1:8787", "local callback listen address")
	tokenFile := fs.String("token-file", defaultFeishuUserTokenFile(), "path to save Feishu user token JSON")
	scope := fs.String("scope", defaultFeishuOAuthScope(), "OAuth scope string")
	timeoutSeconds := fs.Int("timeout-seconds", 600, "seconds to wait for OAuth callback")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}
	client, ok := newFeishuOAuthClientFromEnv(stderr)
	if !ok {
		return 1
	}
	parsedRedirect, err := url.Parse(strings.TrimSpace(*redirectURI))
	if err != nil || parsedRedirect.Path == "" {
		fmt.Fprintf(stderr, "invalid --redirect-uri: %s\n", *redirectURI)
		return 2
	}
	state := randomState()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(parsedRedirect.Path, func(w http.ResponseWriter, r *http.Request) {
		if got := strings.TrimSpace(r.URL.Query().Get("state")); got != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		select {
		case codeCh <- code:
		default:
		}
		_, _ = io.WriteString(w, "Feishu authorization received. You can close this page.")
	})
	server := &http.Server{Addr: strings.TrimSpace(*listenAddr), Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	authURL := client.OAuthURL(*redirectURI, state, *scope)
	fmt.Fprintln(stdout, "Open this URL in your browser to authorize Feishu user access:")
	fmt.Fprintln(stdout, authURL)
	fmt.Fprintf(stdout, "Waiting for callback on %s ...\n", strings.TrimSpace(*listenAddr))

	timeout := time.Duration(*timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	select {
	case code := <-codeCh:
		token, err := client.ExchangeCode(context.Background(), code)
		if err != nil {
			fmt.Fprintf(stderr, "exchange feishu oauth code failed: %v\n", err)
			return 1
		}
		if err := feishu.SaveUserToken(*tokenFile, token); err != nil {
			fmt.Fprintf(stderr, "save feishu user token failed: %v\n", err)
			return 1
		}
		return writeJSON(stdout, stderr, map[string]any{
			"success":    true,
			"token_file": *tokenFile,
			"expires_at": token.ExpiresAt,
		})
	case err := <-errCh:
		fmt.Fprintf(stderr, "start oauth callback server failed: %v\n", err)
		return 1
	case <-time.After(timeout):
		fmt.Fprintln(stderr, "timed out waiting for Feishu OAuth callback")
		return 1
	}
}

func runFeishuSources(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu sources", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
	sourceType := fs.String("source-type", "", "optional source type filter")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}

	repo, closeFn, ok := openFeishuRepository(context.Background(), resolveDBPath(*dbPath), stderr)
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

func newFeishuOAuthClientFromEnv(stderr io.Writer) (*feishu.HTTPClient, bool) {
	appID := strings.TrimSpace(os.Getenv("FEISHU_APP_ID"))
	appSecret := strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET"))
	if appID == "" || appSecret == "" {
		fmt.Fprintln(stderr, "FEISHU_APP_ID and FEISHU_APP_SECRET are required")
		return nil, false
	}
	return feishu.NewHTTPClient(appID, appSecret), true
}

func defaultFeishuRedirectURI() string {
	if value := strings.TrimSpace(os.Getenv("FEISHU_OAUTH_REDIRECT_URI")); value != "" {
		return value
	}
	return "http://127.0.0.1:8787/feishu/oauth/callback"
}

func defaultFeishuUserTokenFile() string {
	if value := strings.TrimSpace(os.Getenv("FEISHU_USER_TOKEN_FILE")); value != "" {
		return value
	}
	return filepath.Join("secrets", "feishu_user_token.json")
}

func defaultFeishuOAuthScope() string {
	if value := strings.TrimSpace(os.Getenv("FEISHU_OAUTH_SCOPE")); value != "" {
		return value
	}
	return strings.Join([]string{
		"drive:drive:readonly",
		"drive:drive.metadata:readonly",
		"drive:file:readonly",
		"drive:file:download",
		"drive:export:readonly",
		"sheets:spreadsheet:readonly",
		"sheets:spreadsheet.meta:read",
		"offline_access",
	}, " ")
}

func randomState() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func runFeishuSeedSources(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("feishu seed-sources", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}

	repo, closeFn, ok := openFeishuRepository(context.Background(), resolveDBPath(*dbPath), stderr)
	if !ok {
		return 1
	}
	defer closeFn()

	defaults, err := feishusync.DefaultSources()
	if err != nil {
		fmt.Fprintf(stderr, "load feishu source config failed: %v\n", err)
		return 1
	}
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
	dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
	company := fs.String("company", support.DefaultCompanyName(), "company override for workbook import")
	snapshotDir := fs.String("snapshot-dir", defaultFeishuSnapshotDir(), "directory for downloaded Feishu snapshots")
	sourceType := fs.String("source-type", "all", "source type: all, pdf_folder, finance_workbook, finance_workbook_folder")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}

	runner, closeFn, ok := openFeishuRunner(context.Background(), resolveDBPath(*dbPath), *snapshotDir, *company, stderr)
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
	dbPath := fs.String("db", "", "postgres dsn (or FINANCEQA_PG_DSN env)")
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

	runner, closeFn, ok := openFeishuRunner(context.Background(), resolveDBPath(*dbPath), *snapshotDir, *company, stderr)
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
	store, err := objectstorage.NewOSSClientFromEnv()
	if err != nil {
		_ = sqlDB.Close()
		fmt.Fprintf(stderr, "load object storage failed: %v\n", err)
		return nil, nil, false
	}
	var objectStore feishusync.ObjectStore
	if store != nil {
		objectStore = store
	}
	runner := newFeishuRunner(repo, client, importer, dbPath, snapshotDir, company, objectStore)
	return runner, func() { _ = sqlDB.Close() }, true
}

func newFeishuRunner(repo *feishusync.Repository, client feishu.Client, importer feishusync.ContractWorkbookImporter, dbPath, snapshotDir, company string, store feishusync.ObjectStore) *feishuRunner {
	return &feishuRunner{
		repo:     repo,
		pdf:      feishusync.NewPDFScanner(client, repo, feishusync.NoopOCRDispatcher{}, snapshotDir, store),
		workbook: feishusync.NewWorkbookScanner(client, repo, importer, dbPath, snapshotDir, company, store),
	}
}

func (r *feishuRunner) scanSource(ctx context.Context, src feishusync.SyncSource) (feishusync.ScanResult, error) {
	switch src.SourceType {
	case feishusync.SourceTypePDFFolder:
		return r.pdf.ScanFolder(ctx, src)
	case feishusync.SourceTypeFinanceWorkbook, feishusync.SourceTypeFinanceWorkbookFolder:
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
	case feishusync.SourceTypePDFFolder, feishusync.SourceTypeFinanceWorkbook, feishusync.SourceTypeFinanceWorkbookFolder:
		return sourceType, nil
	default:
		return "", fmt.Errorf("unsupported --source-type: %s", sourceType)
	}
}

func defaultFeishuSnapshotDir() string {
	return filepath.Join("tmp", "feishu-snapshots")
}
