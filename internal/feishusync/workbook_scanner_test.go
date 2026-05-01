package feishusync_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/feishu"
	"financeqa/internal/feishusync"
	"financeqa/internal/ingest"
)

func TestWorkbookScannerSkipsUnchangedHash(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "workbook-v1")
	src := mustSeedWorkbookSource(t, repo, hash)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "财务表.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-1"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v1")},
	}
	importer := &recordingWorkbookImporter{}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Skipped != 1 || result.Scanned != 1 {
		t.Fatalf("result = %#v, want skipped=1 scanned=1", result)
	}
	if len(importer.calls) != 0 {
		t.Fatalf("unchanged workbook should not import: %#v", importer.calls)
	}
	updated := mustSingleSource(t, repo)
	if updated.LastContentHash != hash || updated.SyncStatus != feishusync.SyncStatusActive {
		t.Fatalf("source after skip = %#v", updated)
	}
}

func TestWorkbookScannerImportsChangedSnapshotNonIncrementally(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedWorkbookSource(t, repo, "old-hash")
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "财务表格", MimeType: "application/octet-stream", Revision: "rev-2"}},
		downloads: map[string][]byte{src.SourceToken: []byte("ignored-download")},
	}
	client.exported = map[string][]byte{src.SourceToken: []byte("workbook-v2")}
	importer := &recordingWorkbookImporter{
		summary: ingest.ImportSummary{ReportType: "contract_workbook", RecordCount: 12},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Created != 1 || result.Scanned != 1 || result.Skipped != 0 {
		t.Fatalf("result = %#v, want created=1 scanned=1", result)
	}
	if len(importer.calls) != 1 {
		t.Fatalf("import calls = %#v, want 1", importer.calls)
	}
	call := importer.calls[0]
	if call.dbPath != "db.sqlite" || call.opts.Incremental {
		t.Fatalf("import call = %#v", call)
	}
	if call.opts.CompanyOverride != "测试公司" {
		t.Fatalf("company override = %q", call.opts.CompanyOverride)
	}
	if _, err := os.Stat(call.filePath); err != nil {
		t.Fatalf("import file should exist: %v", err)
	}
	updated := mustSingleSource(t, repo)
	if updated.LastContentHash == "" || updated.LastContentHash == "old-hash" || updated.LastRevision != "rev-2" {
		t.Fatalf("source after import = %#v", updated)
	}
}

func TestWorkbookScannerImportFailureKeepsOldHashAndMarksError(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedWorkbookSource(t, repo, "old-hash")
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "财务表.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-3"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v3")},
	}
	importer := &recordingWorkbookImporter{err: errors.New("import failed")}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "")

	_, err := scanner.ScanWorkbook(context.Background(), src)
	if err == nil {
		t.Fatalf("scan workbook should fail")
	}
	updated := mustSingleSource(t, repo)
	if updated.LastContentHash != "old-hash" {
		t.Fatalf("last hash = %q, want old-hash", updated.LastContentHash)
	}
	if updated.SyncStatus != feishusync.SyncStatusError || updated.ErrorMessage != "import failed" {
		t.Fatalf("source after failure = %#v", updated)
	}
}

type recordingWorkbookImporter struct {
	calls   []workbookImportCall
	summary ingest.ImportSummary
	err     error
}

type workbookImportCall struct {
	dbPath   string
	filePath string
	opts     ingest.ImportOptions
}

func (i *recordingWorkbookImporter) ImportFileWithOptions(_ context.Context, dbPath, filePath string, opts ingest.ImportOptions) (ingest.ImportSummary, error) {
	i.calls = append(i.calls, workbookImportCall{dbPath: dbPath, filePath: filePath, opts: opts})
	return i.summary, i.err
}

func mustSeedWorkbookSource(t *testing.T, repo *feishusync.Repository, lastHash string) feishusync.SyncSource {
	t.Helper()

	src := feishusync.SyncSource{
		SourceType:      feishusync.SourceTypeFinanceWorkbook,
		SourceToken:     "workbook-token",
		DisplayName:     "飞书财务表",
		SyncStatus:      feishusync.SyncStatusActive,
		LastContentHash: lastHash,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: feishusync.SourceTypeFinanceWorkbook})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("source count = %d, want 1", len(sources))
	}
	return sources[0]
}

func workbookSnapshotPath(t *testing.T, dir, token string) string {
	t.Helper()
	return filepath.Join(dir, token+".xlsx")
}
