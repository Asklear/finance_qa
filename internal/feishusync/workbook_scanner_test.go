package feishusync_test

import (
	"context"
	"encoding/json"
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

func TestWorkbookScannerSkipsUnchangedHashWithoutUploadingDuplicate(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "workbook-v1")
	src := mustSeedWorkbookSourceWithMetadata(t, repo, hash, `{"oss_prefix":"tenant/uhub/finance/2026"}`)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "优集资金收入计算表-2026.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-1"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v1")},
	}
	store := &recordingObjectStore{
		uri:    "tenant/uhub/finance/2026/优集资金收入计算表-2026.xlsx",
		hashes: map[string]string{"tenant/uhub/finance/2026/优集资金收入计算表-2026.xlsx": hash},
	}
	importer := &recordingWorkbookImporter{}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司", store)

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Skipped != 1 || len(store.puts) != 0 || len(importer.calls) != 0 {
		t.Fatalf("result=%#v puts=%#v imports=%#v", result, store.puts, importer.calls)
	}
}

func TestWorkbookScannerBackfillsStorageKeyWhenUnchanged(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "workbook-v1")
	src := mustSeedWorkbookSourceWithMetadata(t, repo, hash, `{"oss_prefix":"tenant/uhub/finance/2026"}`)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "优集资金收入计算表-2026.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-1"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v1")},
	}
	store := &recordingObjectStore{
		hashes: map[string]string{"tenant/uhub/finance/2026/优集资金收入计算表-2026.xlsx": hash},
	}
	importer := &recordingWorkbookImporter{}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司", store)

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Skipped != 1 || len(store.puts) != 0 || len(importer.calls) != 0 {
		t.Fatalf("result=%#v puts=%#v imports=%#v", result, store.puts, importer.calls)
	}
	updated := mustSingleSource(t, repo)
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["storage_key"] != "tenant/uhub/finance/2026/优集资金收入计算表-2026.xlsx" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestWorkbookScannerUploadsSnapshotWhenUnchangedStorageKeyMissing(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "workbook-v1")
	src := mustSeedWorkbookSourceWithMetadata(t, repo, hash, `{"oss_prefix":"tenant/uhub/finance/2026"}`)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "优集资金收入计算表-2026.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-1"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v1")},
	}
	store := &recordingObjectStore{}
	importer := &recordingWorkbookImporter{}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司", store)

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Skipped != 1 || len(store.puts) != 1 || len(importer.calls) != 0 {
		t.Fatalf("result=%#v puts=%#v imports=%#v", result, store.puts, importer.calls)
	}
	updated := mustSingleSource(t, repo)
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["storage_key"] != "tenant/uhub/finance/2026/优集资金收入计算表-2026.xlsx" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestWorkbookScannerFindsExistingOSSObjectByHashBeforeUploading(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedWorkbookSourceWithMetadata(t, repo, "old-hash", `{"oss_prefix":"tenant/uhub/finance/2026"}`)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "优集收入、成本计算表 - 上传", MimeType: "application/octet-stream", Revision: "rev-2"}},
		exported:  map[string][]byte{src.SourceToken: []byte("workbook-v2")},
		downloads: map[string][]byte{},
	}
	hash := writeSnapshotAndHash(t, "workbook-v2")
	existingOSSKey := "tenant/uhub/finance/2026/优集资金收入计算表-2026.xlsx"
	store := &recordingObjectStore{
		hashKeys: map[string]string{hash: existingOSSKey},
	}
	importer := &recordingWorkbookImporter{
		summary: ingest.ImportSummary{ReportType: "contract_workbook", RecordCount: 12},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司", store)

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Created != 1 || len(store.puts) != 0 {
		t.Fatalf("result=%#v puts=%#v", result, store.puts)
	}
	updated := mustSingleSource(t, repo)
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["storage_key"] != existingOSSKey {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestWorkbookScannerAvoidsOverwritingOccupiedOSSPathWithDifferentHash(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedWorkbookSourceWithMetadata(t, repo, "old-hash", `{"oss_prefix":"tenant/uhub/finance/2026"}`)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "优集收入、成本计算表 - 上传.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-2"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v2")},
	}
	hash := writeSnapshotAndHash(t, "workbook-v2")
	targetKey := "tenant/uhub/finance/2026/优集收入、成本计算表 - 上传.xlsx"
	store := &recordingObjectStore{
		hashes: map[string]string{targetKey: "existing-different-hash"},
	}
	importer := &recordingWorkbookImporter{
		summary: ingest.ImportSummary{ReportType: "contract_workbook", RecordCount: 12},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司", store)

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	wantKey := "tenant/uhub/finance/2026/优集收入、成本计算表 - 上传.sha256-" + hash[:12] + ".xlsx"
	if result.Created != 1 || len(store.puts) != 1 || store.puts[0].key != wantKey {
		t.Fatalf("result=%#v puts=%#v want key %q", result, store.puts, wantKey)
	}
	updated := mustSingleSource(t, repo)
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["storage_key"] != wantKey {
		t.Fatalf("metadata = %#v", metadata)
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

func TestWorkbookScannerUsesFeishuTitleForImportedSnapshotPath(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedWorkbookSource(t, repo, "old-hash")
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "优集收入、成本计算表 - 上传.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-2"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v2")},
	}
	importer := &recordingWorkbookImporter{
		summary: ingest.ImportSummary{ReportType: "contract_workbook", RecordCount: 12},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Created != 1 || len(importer.calls) != 1 {
		t.Fatalf("result=%#v imports=%#v", result, importer.calls)
	}
	gotBase := filepath.Base(importer.calls[0].filePath)
	if gotBase != "优集收入、成本计算表 - 上传.xlsx" {
		t.Fatalf("imported snapshot basename = %q, want Feishu title instead of token", gotBase)
	}
}

func TestWorkbookScannerUploadsSnapshotToObjectStore(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedWorkbookSourceWithMetadata(t, repo, "old-hash", `{"oss_prefix":"tenant/uhub/finance/2026"}`)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "优集成本计算表-4.23-池", MimeType: "application/octet-stream", Revision: "rev-2"}},
		exported:  map[string][]byte{src.SourceToken: []byte("workbook-v2")},
		downloads: map[string][]byte{},
	}
	store := &recordingObjectStore{uri: "tenant/uhub/finance/2026/优集成本计算表-4.23-池.xlsx"}
	importer := &recordingWorkbookImporter{
		summary: ingest.ImportSummary{ReportType: "contract_workbook", RecordCount: 12},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司", store)

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Created != 1 || len(store.puts) != 1 {
		t.Fatalf("result=%#v puts=%#v", result, store.puts)
	}
	if store.puts[0].key != "tenant/uhub/finance/2026/优集成本计算表-4.23-池.xlsx" {
		t.Fatalf("object key should follow historical finance path: %#v", store.puts[0])
	}
	updated := mustSingleSource(t, repo)
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["storage_key"] != store.uri {
		t.Fatalf("metadata = %#v", metadata)
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
	return mustSeedWorkbookSourceWithMetadata(t, repo, lastHash, "")
}

func mustSeedWorkbookSourceWithMetadata(t *testing.T, repo *feishusync.Repository, lastHash, metadata string) feishusync.SyncSource {
	t.Helper()

	src := feishusync.SyncSource{
		SourceType:      feishusync.SourceTypeFinanceWorkbook,
		SourceToken:     "workbook-token",
		DisplayName:     "飞书财务表",
		SyncStatus:      feishusync.SyncStatusActive,
		LastContentHash: lastHash,
		MetadataJSON:    metadata,
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
