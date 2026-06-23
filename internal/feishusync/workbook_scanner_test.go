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
	src := mustSeedWorkbookSourceWithMetadata(t, repo, hash, `{"file_mappings_content_hash":"`+hash+`"}`)
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

func TestWorkbookScannerImportsUnchangedHashWhenFileMappingsNotSynced(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "workbook-v1")
	src := mustSeedWorkbookSource(t, repo, hash)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "财务表.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-1"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v1")},
	}
	importer := &recordingWorkbookImporter{
		summary: ingest.ImportSummary{ReportType: "contract_mixed_finance", RecordCount: 12, PeriodStart: "2025-10", PeriodEnd: "2026-03"},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Created != 1 || result.Skipped != 0 || len(importer.calls) != 1 {
		t.Fatalf("result=%#v imports=%#v", result, importer.calls)
	}
	updated := mustSingleSource(t, repo)
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["file_mappings_content_hash"] != hash {
		t.Fatalf("metadata should mark file mappings synced: %#v", metadata)
	}
}

func TestWorkbookScannerSkipsUnchangedHashWithoutUploadingDuplicate(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "workbook-v1")
	src := mustSeedWorkbookSourceWithMetadata(t, repo, hash, `{"oss_prefix":"tenant/uhub/finance/2026","file_mappings_content_hash":"`+hash+`"}`)
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
	src := mustSeedWorkbookSourceWithMetadata(t, repo, hash, `{"oss_prefix":"tenant/uhub/finance/2026","file_mappings_content_hash":"`+hash+`"}`)
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

func TestWorkbookScannerPreservesImportMetadataWhenUnchanged(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "workbook-v1")
	src := mustSeedWorkbookSourceWithMetadata(t, repo, hash, `{"oss_prefix":"tenant/uhub/finance","storage_key":"tenant/uhub/finance/优集收入、成本计算表 - 上传.xlsx","report_type":"contract_mixed_finance","record_count":139,"period_start":"2025-10","period_end":"2026-05","file_mappings_content_hash":"`+hash+`"}`)
	client := &fakeFeishuClient{
		files:     []feishu.DriveFile{{Token: src.SourceToken, Name: "优集收入、成本计算表 - 上传.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Revision: "rev-1"}},
		downloads: map[string][]byte{src.SourceToken: []byte("workbook-v1")},
	}
	importer := &recordingWorkbookImporter{}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook: %v", err)
	}
	if result.Skipped != 1 || len(importer.calls) != 0 {
		t.Fatalf("result=%#v imports=%#v", result, importer.calls)
	}
	updated := mustSingleSource(t, repo)
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["report_type"] != "contract_mixed_finance" || metadata["period_start"] != "2025-10" || metadata["period_end"] != "2026-05" || metadata["record_count"].(float64) != 139 {
		t.Fatalf("metadata should preserve import summary on skip: %#v", metadata)
	}
}

func TestWorkbookScannerUploadsSnapshotWhenUnchangedStorageKeyMissing(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "workbook-v1")
	src := mustSeedWorkbookSourceWithMetadata(t, repo, hash, `{"oss_prefix":"tenant/uhub/finance/2026","file_mappings_content_hash":"`+hash+`"}`)
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

func TestWorkbookScannerImportsLatestWorkbookFromFolderSource(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := feishusync.SyncSource{
		SourceType:      feishusync.SourceTypeFinanceWorkbookFolder,
		SourceToken:     "folder-token",
		DisplayName:     "飞书财务表文件夹",
		SyncStatus:      feishusync.SyncStatusActive,
		LastContentHash: "old-hash",
		MetadataJSON:    `{"oss_prefix":"tenant/uhub/finance"}`,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: feishusync.SourceTypeFinanceWorkbookFolder})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("source count = %d, want 1", len(sources))
	}
	src = sources[0]
	oldFile := feishu.DriveFile{Token: "old-workbook-token", Name: "优集收入、成本计算表 - 旧.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ModifiedTime: "2026-04-01T10:00:00+08:00", Revision: "rev-old"}
	latestFile := feishu.DriveFile{Token: "latest-workbook-token", Name: "优集收入、成本计算表 - 上传.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ModifiedTime: "2026-05-06T15:47:00+08:00", Revision: "rev-latest"}
	client := &fakeFeishuClient{
		files:         []feishu.DriveFile{oldFile, latestFile},
		filesByFolder: map[string][]feishu.DriveFile{"folder-token": {oldFile, latestFile}},
		downloads:     map[string][]byte{"latest-workbook-token": []byte("latest-workbook")},
	}
	importer := &recordingWorkbookImporter{
		summary: ingest.ImportSummary{ReportType: "contract_workbook", RecordCount: 12},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan workbook folder: %v", err)
	}
	if result.Created != 1 || result.Scanned != 1 || len(importer.calls) != 1 {
		t.Fatalf("result=%#v imports=%#v", result, importer.calls)
	}
	if len(client.downloadedTokens) != 1 || client.downloadedTokens[0] != "latest-workbook-token" {
		t.Fatalf("downloaded tokens = %#v", client.downloadedTokens)
	}
	updated := mustSingleSource(t, repo)
	if updated.SourceToken != "folder-token" || updated.LastRevision != "rev-latest" {
		t.Fatalf("source after folder scan = %#v", updated)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["file_token"] != "latest-workbook-token" || metadata["source_token"] != "folder-token" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestWorkbookScannerSkipsFolderWorkbookDownloadWhenMetadataUnchanged(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "latest-workbook")
	src := feishusync.SyncSource{
		SourceType:      feishusync.SourceTypeFinanceWorkbookFolder,
		SourceToken:     "folder-token",
		DisplayName:     "飞书财务表文件夹",
		SyncStatus:      feishusync.SyncStatusActive,
		LastContentHash: hash,
		MetadataJSON: `{
			"oss_prefix":"tenant/uhub/finance",
			"storage_key":"tenant/uhub/finance/优集收入、成本计算表 - 上传.xlsx",
			"file_token":"latest-workbook-token",
			"modified_time":"1714972020",
			"file_mappings_content_hash":"` + hash + `"
		}`,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: feishusync.SourceTypeFinanceWorkbookFolder})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	src = sources[0]
	latestFile := feishu.DriveFile{
		Token:        "latest-workbook-token",
		Name:         "优集收入、成本计算表 - 上传.xlsx",
		MimeType:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		ModifiedTime: "1714972020",
	}
	client := &fakeFeishuClient{
		files:         []feishu.DriveFile{latestFile},
		filesByFolder: map[string][]feishu.DriveFile{"folder-token": {latestFile}},
		downloads:     map[string][]byte{"latest-workbook-token": []byte("latest-workbook")},
		exported:      map[string][]byte{"latest-workbook-token": []byte("latest-workbook")},
	}
	importer := &recordingWorkbookImporter{}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan unchanged workbook folder: %v", err)
	}
	if result.Skipped != 1 || result.Scanned != 1 || result.Created != 0 {
		t.Fatalf("result=%#v, want skipped=1 scanned=1 created=0", result)
	}
	if len(client.downloadedTokens) != 0 || len(client.exportedTokens) != 0 {
		t.Fatalf("unchanged workbook should not download/export: downloads=%#v exports=%#v", client.downloadedTokens, client.exportedTokens)
	}
	if len(importer.calls) != 0 {
		t.Fatalf("unchanged workbook should not import: %#v", importer.calls)
	}
}

func TestWorkbookScannerForceDownloadsWhenMetadataUnchanged(t *testing.T) {
	t.Setenv("FINANCEQA_FEISHU_FORCE_DOWNLOAD", "1")

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "latest-workbook")
	src := feishusync.SyncSource{
		SourceType:      feishusync.SourceTypeFinanceWorkbookFolder,
		SourceToken:     "folder-token",
		DisplayName:     "飞书财务表文件夹",
		SyncStatus:      feishusync.SyncStatusActive,
		LastContentHash: hash,
		MetadataJSON: `{
			"oss_prefix":"tenant/uhub/finance",
			"storage_key":"tenant/uhub/finance/优集收入、成本计算表 - 上传.xlsx",
			"file_token":"latest-workbook-token",
			"modified_time":"1714972020",
			"file_mappings_content_hash":"` + hash + `"
		}`,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: feishusync.SourceTypeFinanceWorkbookFolder})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	src = sources[0]
	latestFile := feishu.DriveFile{
		Token:        "latest-workbook-token",
		Name:         "优集收入、成本计算表 - 上传.xlsx",
		MimeType:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		ModifiedTime: "1714972020",
	}
	client := &fakeFeishuClient{
		files:         []feishu.DriveFile{latestFile},
		filesByFolder: map[string][]feishu.DriveFile{"folder-token": {latestFile}},
		downloads:     map[string][]byte{"latest-workbook-token": []byte("latest-workbook")},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, &recordingWorkbookImporter{}, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan forced workbook folder: %v", err)
	}
	if result.Skipped != 1 || len(client.downloadedTokens) != 1 || client.downloadedTokens[0] != "latest-workbook-token" {
		t.Fatalf("result=%#v downloaded=%#v, want forced download", result, client.downloadedTokens)
	}
}

func TestWorkbookScannerMetadataShortcutDisabledDownloadsWhenMetadataUnchanged(t *testing.T) {
	t.Setenv("FINANCEQA_FEISHU_METADATA_SHORTCUT", "0")

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	hash := writeSnapshotAndHash(t, "latest-workbook")
	src := feishusync.SyncSource{
		SourceType:      feishusync.SourceTypeFinanceWorkbookFolder,
		SourceToken:     "folder-token",
		DisplayName:     "飞书财务表文件夹",
		SyncStatus:      feishusync.SyncStatusActive,
		LastContentHash: hash,
		MetadataJSON: `{
			"oss_prefix":"tenant/uhub/finance",
			"storage_key":"tenant/uhub/finance/优集收入、成本计算表 - 上传.xlsx",
			"file_token":"latest-workbook-token",
			"modified_time":"1714972020",
			"file_mappings_content_hash":"` + hash + `"
		}`,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: feishusync.SourceTypeFinanceWorkbookFolder})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	src = sources[0]
	latestFile := feishu.DriveFile{
		Token:        "latest-workbook-token",
		Name:         "优集收入、成本计算表 - 上传.xlsx",
		MimeType:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		ModifiedTime: "1714972020",
	}
	client := &fakeFeishuClient{
		files:         []feishu.DriveFile{latestFile},
		filesByFolder: map[string][]feishu.DriveFile{"folder-token": {latestFile}},
		downloads:     map[string][]byte{"latest-workbook-token": []byte("latest-workbook")},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, &recordingWorkbookImporter{}, "db.sqlite", t.TempDir(), "测试公司")

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan metadata-disabled workbook folder: %v", err)
	}
	if result.Skipped != 1 || len(client.downloadedTokens) != 1 || client.downloadedTokens[0] != "latest-workbook-token" {
		t.Fatalf("result=%#v downloaded=%#v, want shortcut disabled download", result, client.downloadedTokens)
	}
}

func TestWorkbookScannerFolderSourceUpdatesStorageKeyForReuploadedWorkbook(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	oldHash := writeSnapshotAndHash(t, "old-workbook")
	src := feishusync.SyncSource{
		SourceType:      feishusync.SourceTypeFinanceWorkbookFolder,
		SourceToken:     "folder-token",
		DisplayName:     "飞书财务表文件夹",
		SyncStatus:      feishusync.SyncStatusActive,
		LastContentHash: oldHash,
		MetadataJSON:    `{"oss_prefix":"tenant/uhub/finance/2026","storage_key":"tenant/uhub/finance/2026/旧表.xlsx"}`,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: feishusync.SourceTypeFinanceWorkbookFolder})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	src = sources[0]
	newFile := feishu.DriveFile{Token: "new-workbook-token", Name: "优集收入、成本计算表 - 上传.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ModifiedTime: "2026-05-06T15:47:00+08:00", Revision: "rev-new"}
	client := &fakeFeishuClient{
		files:         []feishu.DriveFile{newFile},
		filesByFolder: map[string][]feishu.DriveFile{"folder-token": {newFile}},
		downloads:     map[string][]byte{"new-workbook-token": []byte("new-workbook")},
	}
	store := &recordingObjectStore{}
	importer := &recordingWorkbookImporter{
		summary: ingest.ImportSummary{ReportType: "contract_workbook", RecordCount: 12},
	}
	scanner := feishusync.NewWorkbookScanner(client, repo, importer, "db.sqlite", t.TempDir(), "测试公司", store)

	result, err := scanner.ScanWorkbook(context.Background(), src)
	if err != nil {
		t.Fatalf("scan reuploaded workbook: %v", err)
	}
	if result.Created != 1 || len(store.puts) != 1 || len(importer.calls) != 1 {
		t.Fatalf("result=%#v puts=%#v imports=%#v", result, store.puts, importer.calls)
	}
	wantKey := "tenant/uhub/finance/2026/优集收入、成本计算表 - 上传.xlsx"
	if store.puts[0].key != wantKey {
		t.Fatalf("uploaded key = %q, want %q", store.puts[0].key, wantKey)
	}
	updated := mustSingleSource(t, repo)
	if updated.LastContentHash == "" || updated.LastContentHash == oldHash || updated.LastRevision != "rev-new" {
		t.Fatalf("source after reupload scan = %#v", updated)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["file_token"] != "new-workbook-token" || metadata["storage_key"] != wantKey {
		t.Fatalf("metadata = %#v", metadata)
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
	if len(importer.calls) != 1 {
		t.Fatalf("imports=%#v, want one import", importer.calls)
	}
	importOpts := importer.calls[0].opts
	if importOpts.SourceFileName != "优集成本计算表-4.23-池" || importOpts.SourceStorageKey != store.uri || importOpts.SourceFileSize != int64(len("workbook-v2")) {
		t.Fatalf("import source metadata = %#v", importOpts)
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
